#!/bin/bash

# NodelistDB FTP Synchronization Script
#
# This script automates the process of:
# 1. Connecting to an FTP server to get available nodelist files
# 2. Checking which files are already imported in the database via API
# 3. Downloading missing files from FTP
# 4. Importing new files with the parser
# 5. Compressing and storing files in the proper directory structure
# 6. Pushing the archive to a secondary download server (rsync, optional)
#
# Runs on the host that holds the archive (oracle-vm1 / nodelist.fidonet.cc
# since 2026-10-27; it was oracle-main before). Everything else it needs is
# reached over the network: the FTP source, ClickHouse and the API.
#
# Uses ClickHouse backend via YAML configuration (config.yaml)
# Author: Generated for NodelistDB project
# Usage: ./sync_nodelists.sh [options]

set -euo pipefail  # Exit on error, undefined vars, pipe failures

#################################################################################
# CONFIGURATION SECTION
#################################################################################

# Script paths and directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/sync_config.conf"
LOG_FILE="${SCRIPT_DIR}/sync.log"

# Default configuration values (can be overridden in config file)
FTP_HOST=""                                          # FTP server hostname
FTP_USER=""                                          # FTP username
FTP_PASS=""                                          # FTP password
FTP_DIR="/"                                          # Remote FTP directory path
LOCAL_NODELIST_DIR="/home/dp/nodelists"              # Local storage for compressed files
CONFIG_PATH="${SCRIPT_DIR}/config.yaml"              # Configuration file path (YAML format)
API_BASE_URL="http://localhost:8080/api"             # NodelistDB API endpoint
TEMP_DIR="/tmp/nodelist_sync"                        # Temporary download directory
MAX_RETRIES=3                                        # Max download retry attempts
SERVICE_NAME="nodelistdb"                            # systemd service name
PARSER_PATH="${SCRIPT_DIR}/parser"               # Path to parser binary

# Remote server sync configuration
REMOTE_SYNC_HOST="root@nodelist.5001.ru"         # Remote server to sync nodelists to
REMOTE_NODELIST_DIR="/opt/nodelists"             # Remote nodelist directory
# rsync destination for the push; defaults to REMOTE_SYNC_HOST:REMOTE_NODELIST_DIR/.
# Set it explicitly when the remote key is restricted with rrsync, whose paths
# are relative to the directory it is pinned to (then "host:" is the root).
REMOTE_SYNC_DEST=""

# Held for the whole run, so a cron run and a hand run never share TEMP_DIR
# (cleanup() deletes it) or race the "already imported" check. deploy.sh takes
# the same lock before replacing the parser or this script.
SYNC_LOCK_FILE=""
SYNC_LOCK_HELD=false

# Push state: set once the run has started syncing, so cleanup() still pushes
# what was archived when a later step aborts the run; and so that a push that
# fails every night fails the run instead of hiding in the log.
SYNC_STARTED=false
PUSH_DONE=false
PUSH_FAILED=false

# Additional FTN networks to sync (fidonet is handled by the main flow).
# One entry per network: "name|source|url_or_dir|filename_regex"
#   name           lowercase network name (must exist in config.yaml networks:)
#   source         http or ftp
#   url_or_dir     http(s) directory URL, or FTP directory (uses FTP_* creds)
#   filename_regex extended regex the remote filenames must match
# Example:
#   EXTRA_NETWORKS=("fsxnet|http|https://fsxnet.nz/fsxnet/|fsxnet\.[0-9]{3}")
EXTRA_NETWORKS=()

# Pointlist series to sync (FTS-5002 weeklies).
# One entry per series: "name|source|url_or_dir|filename_regex|charset[|network]"
#   name           list_source label (z2, r24, r50, ...) passed to the parser
#   source         http or ftp
#   url_or_dir     http(s) directory URL, or FTP directory (uses FTP_* creds)
#   filename_regex extended regex the remote filenames must match
#   charset        cp437, cp850, cp866, latin1 or utf8
#   network        FTN network (optional, default fidonet)
# Idempotency comes from the parser's pointlist_files gate: already-imported
# issues are skipped automatically, so every matching remote file can be fed
# to the parser each run.
# The regex matches the COMPRESSED name; the day in a .Z## suffix is the
# day-of-year mod 100, so the true day comes from the extracted inner file.
# Example (a local FidoNet mailer's file-echo area, reached over its FTP):
#   EXTRA_POINTLISTS=("z2|ftp|/z2pnt|z2pnt\.z[0-9]{2}|cp437")
EXTRA_POINTLISTS=()

# Local storage for archived pointlist files:
# <LOCAL_POINTLIST_DIR>/<network>/<source>/<year>/NAME.DDD.gz
#
# Defaults to a "pointlists" directory inside the nodelist root, resolved
# after the config file is sourced so that overriding LOCAL_NODELIST_DIR moves
# it too. Living inside the nodelist root is deliberate, not incidental: the
# archive push (and whatever else mirrors nodelists to a download front end)
# then carries the pointlists with them, with no second transfer to configure
# and keep working.
LOCAL_POINTLIST_DIR=""

# Load configuration file if it exists
if [[ -f "$CONFIG_FILE" ]]; then
    source "$CONFIG_FILE"
fi

# Derive the pointlist root from the nodelist root unless set explicitly.
: "${LOCAL_POINTLIST_DIR:=${LOCAL_NODELIST_DIR}/pointlists}"

# The push mirrors the whole nodelist root, pointlists included.
if [[ -z "$REMOTE_SYNC_DEST" && -n "${REMOTE_SYNC_HOST:-}" ]]; then
    REMOTE_SYNC_DEST="${REMOTE_SYNC_HOST}:${REMOTE_NODELIST_DIR}/"
fi

: "${SYNC_LOCK_FILE:=${SCRIPT_DIR}/sync.lock}"

# FidoNet's own archive root. Its year directories used to sit directly in the
# nodelist root, which made FidoNet the one network with no name in the path —
# the archive listed forty year directories next to a lone fsxnet/. It now sits
# one level down like every other network, matching EXTRA_NETWORKS above and the
# pointlist tree. The server still serves the old network-less paths, so files
# already published under them keep resolving.
: "${LOCAL_FIDONET_DIR:=${LOCAL_NODELIST_DIR}/fidonet}"

# Shared with scripts/import_pointlists.sh — both must name the same file for
# the mutual exclusion to mean anything.
: "${POINTLIST_LOCK_FILE:=/tmp/nodelistdb-import-pointlists.lock}"

#################################################################################
# UTILITY FUNCTIONS
#################################################################################

# Logging function with timestamp
# Usage: log "message" [error]
log() {
    local message="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    
    # Always write to log file
    echo "$message" >> "$LOG_FILE"
    
    # Only print to console if it's an error OR we're not in quiet mode
    if [[ "${2:-}" == "error" ]] || [[ "${QUIET_MODE:-false}" != "true" ]]; then
        echo "$message"
    fi
}

# Error logging and exit
# Usage: error_exit "error message"
error_exit() {
    log "ERROR: $1" error
    cleanup
    exit 1
}

# Cleanup temporary files (server restart not needed for ClickHouse)
# TEMP_DIR is shared by every run, so only the run holding the sync lock may
# delete it; a run that found the lock taken would otherwise wipe the other
# run's downloads on its way out.
#
# A run aborted by error_exit (say, a nodelist the parser rejects) still
# pushes what it archived before the failure: otherwise one bad file would
# hold back the mirror on every run until it was dealt with. The exit status
# of the aborted run is kept.
cleanup() {
    if [[ "$SYNC_LOCK_HELD" == "true" && "$SYNC_STARTED" == "true" && "$PUSH_DONE" != "true" ]]; then
        push_archive
    fi

    if [[ "$SYNC_LOCK_HELD" == "true" && -d "$TEMP_DIR" ]]; then
        log "Cleaning up temporary directory: $TEMP_DIR"
        rm -rf "$TEMP_DIR"
    fi

    # Note: ClickHouse allows concurrent writes, so no server restart needed
}

#################################################################################
# DEPENDENCY AND CONFIGURATION VALIDATION
#################################################################################

# Check if required system dependencies are available
check_dependencies() {
    log "Checking system dependencies..."
    
    local deps=("curl" "ftp" "gzip" "cmp" "flock" "systemctl")
    if [[ -n "${REMOTE_SYNC_DEST:-}" ]]; then
        deps+=("rsync" "ssh")
    fi
    local missing_deps=()
    
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            missing_deps+=("$dep")
        fi
    done
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        error_exit "Missing required dependencies: ${missing_deps[*]}"
    fi
    
    log "All dependencies found"
}

# Validate configuration parameters
validate_config() {
    log "Validating configuration..."
    
    # Check FTP configuration
    if [[ -z "$FTP_HOST" || -z "$FTP_USER" || -z "$FTP_PASS" ]]; then
        error_exit "FTP configuration incomplete. Please set FTP_HOST, FTP_USER, and FTP_PASS in $CONFIG_FILE"
    fi
    
    # Check local directories exist
    if [[ ! -d "$(dirname "$LOCAL_NODELIST_DIR")" ]]; then
        error_exit "Parent directory of LOCAL_NODELIST_DIR does not exist: $(dirname "$LOCAL_NODELIST_DIR")"
    fi
    
    # Check configuration file exists (required for ClickHouse)
    if [[ ! -f "$CONFIG_PATH" ]]; then
        error_exit "Configuration file not found: $CONFIG_PATH"
    fi

    # Check parser binary exists
    if [[ ! -x "$PARSER_PATH" ]]; then
        error_exit "Parser binary not found or not executable: $PARSER_PATH"
    fi
    
    # Check if we can reach the API (server must be running)
    if ! wait_for_api; then
        error_exit "Cannot reach NodelistDB API at $API_BASE_URL. Is the server running?"
    fi

    log "Configuration validation passed"
}

# Wait for the API to answer. The sync now runs next to the public web server,
# so a deploy restarting it must delay a run rather than abort it.
# Returns: 0 once /health answers 200, 1 after API_WAIT_TRIES attempts
wait_for_api() {
    local tries="${API_WAIT_TRIES:-10}" i
    for ((i = 1; i <= tries; i++)); do
        if curl -sf --connect-timeout 5 --max-time 15 "$API_BASE_URL/health" > /dev/null; then
            return 0
        fi
        log "API at $API_BASE_URL not answering (attempt $i/$tries)"
        if ((i < tries)); then
            sleep "${API_WAIT_INTERVAL:-30}"
        fi
    done
    return 1
}

#################################################################################
# SERVER MANAGEMENT FUNCTIONS (ClickHouse Version)
#################################################################################

# Check if the NodelistDB systemd service is running
# Returns: 0 if running, 1 if stopped
is_server_running() {
    systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null
}

# Note: Server stop/start functions removed for ClickHouse version
# ClickHouse supports concurrent writes, so no server downtime is required
# during data imports. The server can continue serving API requests while
# new nodelist data is being imported in the background.

#################################################################################
# DATABASE STATE CHECKING (via API)
#################################################################################

# Get list of all dates that have nodelist data in the database
# Uses the /api/stats/dates endpoint to get available dates
# Returns: List of dates in YYYY-MM-DD format, one per line
get_database_dates() {
    log "Fetching existing dates from database via API..."
    
    # -f: an error page must not read as an empty database, which would send
    # every file on the FTP server back through the parser. /api/stats is in
    # the rate limiter's expensive class, so a 429 is retried (curl honours
    # Retry-After) rather than failing the run.
    local response
    if ! response=$(curl -sf --retry 4 --retry-max-time 180 --connect-timeout 10 --max-time 60 "$API_BASE_URL/stats/dates"); then
        error_exit "Failed to fetch dates from API"
    fi
    
    # Extract dates from JSON response using grep (simple JSON parsing)
    # Expected format: {"dates":["2023-01-01","2023-01-08",...]}
    local dates
    dates=$(echo "$response" | grep -o '"[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]"' | tr -d '"' | sort)
    
    if [[ -n "$dates" ]]; then
        local count=$(echo "$dates" | wc -l)
        log "Found $count dates in database"
        log "Latest date in database: $(echo "$dates" | tail -1)"
    else
        log "No dates found in database (empty database)"
    fi
    
    echo "$dates"
}

# Convert a nodelist filename to a date
# Input: filename like "nodelist.276" or "z2daily.276"
# Output: date in YYYY-MM-DD format
# Logic: Day 276 of current year, adjusting for year boundaries
filename_to_date() {
    local filename="$1"
    
    # Extract day number from filename (e.g., nodelist.276, z2daily.276,
    # fsxnet.191). Any <name>.DDD file is accepted.
    if [[ "$filename" =~ ([A-Za-z0-9_-]+)\.([0-9]{3})$ ]]; then
        local day_num="${BASH_REMATCH[2]}"
        local current_year=$(date +%Y)
        local current_day_of_year=$(date +%j)
        
        # Remove leading zeros for arithmetic
        day_num=$((10#$day_num))
        current_day_of_year=$((10#$current_day_of_year))
        
        
        
        local target_year=$current_year
        # If the day number is more than 1 day ahead of today, it must be from last year
        # (we allow +1 day buffer for timezone differences and early uploads)
        if [[ $day_num -gt $((current_day_of_year + 1)) ]]; then
            target_year=$((current_year - 1))
        fi
        
        # Convert day of year to actual date
        local date_str
        if date_str=$(date -d "${target_year}-01-01 +$((day_num - 1)) days" +%Y-%m-%d 2>/dev/null); then
            echo "$date_str"
            return 0
        fi
    fi
    
    return 1
}

#################################################################################
# FTP OPERATIONS
#################################################################################

# Get list of nodelist files available on FTP server
# Returns: List of filenames matching nodelist.XXX pattern
get_ftp_listing() {
    local ftp_dir="$1"
    local temp_list="${TEMP_DIR}/ftp_listing.txt"
    local temp_clean="${TEMP_DIR}/ftp_clean.txt"
    
    # Redirect log output to stderr to avoid capturing it in return value
    log "Getting FTP directory listing from $FTP_HOST:$ftp_dir..." >&2
    
    # Use FTP to get directory listing, capture both stdout and stderr
    if ftp -n "$FTP_HOST" > "$temp_list" 2>&1 <<EOF
user $FTP_USER $FTP_PASS
cd $ftp_dir
ls
quit
EOF
    then
        # Clean the FTP output - remove FTP protocol messages and keep only file listings
        # FTP directory listings typically have lines that start with file permissions or dates
        # Filter out FTP commands, responses, and log-like messages
        grep -v -E "^(220|230|250|331|ftp>|200|150|226|221|331|530|550)" "$temp_list" | \
        grep -v -E "^\+" | \
        grep -v -E "^user|^cd|^ls|^quit" | \
        grep -v -E "^\[.*\]" | \
        grep -v -E "^(Getting|No nodelist|Found)" | \
        grep -E "^[-drwx].*|^[0-9].*|^[A-Za-z].*[0-9].*(nodelist|z2daily)" | \
        awk '{print $NF}' > "$temp_clean"
        
        # Extract nodelist files from the cleaned listing
        # Look for files like "nodelist.276", "NODELIST.276", "z2daily.276", or "Z2DAILY.276"
        local files
        files=$(grep -i -E "(nodelist|z2daily)\.[0-9]{3}$" "$temp_clean" | sort)
        
        if [[ -n "$files" ]]; then
            local count=$(echo "$files" | wc -l)
            log "Found $count nodelist files on FTP server" >&2
        else
            log "No nodelist files found on FTP server" >&2
        fi
        
        # Only output the files list, not log messages
        echo "$files"
        return 0
    else
        error_exit "Failed to connect to FTP server or list directory"
    fi
}

# Download a specific file from FTP server with retry logic
# Args: remote_filename local_filepath ftp_directory
download_ftp_file() {
    local remote_file="$1"
    local local_file="$2" 
    local ftp_dir="$3"
    
    log "Downloading $remote_file from FTP..."
    
    # Retry loop for network reliability
    local retry=0
    while [[ $retry -lt $MAX_RETRIES ]]; do
        
        # Attempt FTP download
        if ftp -n "$FTP_HOST" <<EOF 2>/dev/null
user $FTP_USER $FTP_PASS
binary
cd $ftp_dir
get $remote_file $local_file
quit
EOF
        then
            # Verify download was successful (file exists and has content)
            if [[ -f "$local_file" && -s "$local_file" ]]; then
                local size=$(stat -c%s "$local_file")
                log "Successfully downloaded $remote_file ($size bytes)"
                return 0
            else
                log "Download completed but file is empty or missing"
            fi
        else
            log "FTP download failed for $remote_file"
        fi
        
        # Increment retry counter and wait before next attempt
        retry=$((retry + 1))
        if [[ $retry -lt $MAX_RETRIES ]]; then
            log "Retry attempt $retry/$MAX_RETRIES for $remote_file in 5 seconds..."
            sleep 5
        fi
    done

    log "ERROR: Failed to download $remote_file after $MAX_RETRIES attempts" error
    return 1
}

#################################################################################
# FILE PROCESSING FUNCTIONS
#################################################################################

# Import a nodelist file into the database using the parser
# Args: filepath_to_nodelist_file
import_to_database() {
    local file_path="$1"
    local filename=$(basename "$file_path")

    log "Importing $filename into database..."

    # Change to script directory to ensure paths work properly
    cd "$SCRIPT_DIR"

    # Build parser command using modern config.yaml format
    # The parser now reads ClickHouse connection from config.yaml's clickhouse section
    local parser_cmd="\"$PARSER_PATH\" -path \"$file_path\" -config \"$CONFIG_PATH\" -verbose"

    # Execute parser command
    if eval "$parser_cmd" > "${TEMP_DIR}/parser.log" 2>&1; then
        log "Successfully imported $filename"

        # Log any important parser output
        if grep -q "ERROR\|WARN" "${TEMP_DIR}/parser.log"; then
            log "Parser warnings/errors for $filename:" error
            grep "ERROR\|WARN" "${TEMP_DIR}/parser.log" | head -5 | while read -r line; do
                log "  $line" error
            done
        fi

        return 0
    else
        log "Parser failed for $filename. Last few lines of output:" error
        tail -10 "${TEMP_DIR}/parser.log" | while read -r line; do
            log "  $line" error
        done
        error_exit "Failed to import $filename into database"
    fi
}

# Gzip a file into the archive.
# Args: source_file_path destination_gz_path
# Sets STORE_RESULT to "stored" or "unchanged"; returns 1 on failure.
#
# Written to a temporary name and renamed, so the archive never holds a
# truncated file. A destination that already holds the same content is left
# alone: rewriting it would only move its mtime, and every rsync mirror would
# then copy it again (the pointlist series re-archived ~76 unchanged files a
# night that way). The comparison is on the decompressed bytes, so files
# gzipped elsewhere or with other gzip settings still count as the same.
store_gzip() {
    local src="$1" dest="$2"
    STORE_RESULT=""

    mkdir -p "$(dirname "$dest")" || return 1

    if [[ -s "$dest" ]] && gzip -dc "$dest" 2>/dev/null | cmp -s - "$src"; then
        STORE_RESULT="unchanged"
        return 0
    fi

    local tmp="${dest}.tmp.$$"
    if gzip -n -c "$src" > "$tmp" && [[ -s "$tmp" ]] && mv -f "$tmp" "$dest"; then
        STORE_RESULT="stored"
        return 0
    fi
    rm -f "$tmp"
    return 1
}

# Archive name of a fidonet file: z2daily.DDD is published as nodelist.ddd
# Args: ftp_filename
fidonet_archive_name() {
    local name="${1//[zZ]2[dD][aA][iI][lL][yY]/nodelist}"
    echo "${name,,}"
}

# Archive path of a fidonet file
# Args: ftp_filename date(YYYY-MM-DD)
fidonet_archive_path() {
    echo "${LOCAL_FIDONET_DIR}/${2%%-*}/$(fidonet_archive_name "$1").gz"
}

# Compress and store a nodelist file in the proper directory structure
# Args: source_file_path
# Creates: <LOCAL_FIDONET_DIR>/YYYY/nodelist.XXX.gz
compress_and_store() {
    local file_path="$1"
    local filename=$(basename "$file_path")

    # Determine the year for this file based on its date
    local file_date
    if ! file_date=$(filename_to_date "$filename"); then
        error_exit "Could not determine date for file: $filename"
    fi

    local compressed_file
    compressed_file=$(fidonet_archive_path "$filename" "$file_date")

    log "Compressing and storing $filename -> $compressed_file"

    if ! store_gzip "$file_path" "$compressed_file"; then
        error_exit "Failed to store $filename as $compressed_file"
    fi

    if [[ "$STORE_RESULT" == "unchanged" ]]; then
        log "$compressed_file already holds this content - left untouched"
    else
        local original_size=$(stat -c%s "$file_path")
        local compressed_size=$(stat -c%s "$compressed_file")
        local ratio=$(( (original_size - compressed_size) * 100 / original_size ))
        log "Successfully compressed $filename (${ratio}% reduction: ${original_size} -> ${compressed_size} bytes)"
    fi
}

#################################################################################
#################################################################################
# REMOTE SERVER SYNCHRONIZATION
#################################################################################

# Push the whole archive to the remote download server.
#
# One rsync of the tree at the end of a run, rather than an scp per file as
# it was written: a file whose push failed used to be missing on the mirror
# for good, because the next run skips any date already in the database. Now
# the next run simply carries it over. Unchanged files cost a stat each.
#
# Never --delete: the archive is append-only, and a mistaken source path must
# not be able to empty the mirror. (The 5001 key is also pinned with
# `rrsync -no-del`, so the remote refuses it regardless.)
push_archive() {
    PUSH_DONE=true
    if [[ -z "${REMOTE_SYNC_DEST:-}" ]]; then
        return 0
    fi

    log "Pushing archive $LOCAL_NODELIST_DIR/ to $REMOTE_SYNC_DEST..."
    mkdir -p "$TEMP_DIR"
    local rc=0
    # ControlMaster off: a persisting master ssh would inherit the sync lock's
    # fd and keep holding it after this run exits.
    rsync -a --no-owner --no-group --timeout=120 --stats \
        --exclude='*.tmp.*' \
        -e "ssh -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=30 -o ControlMaster=no -o ControlPath=none" \
        "${LOCAL_NODELIST_DIR}/" "$REMOTE_SYNC_DEST" > "${TEMP_DIR}/rsync.log" 2>&1 || rc=$?

    if [[ $rc -eq 0 ]]; then
        log "Archive pushed ($(grep -E '^Number of regular files transferred' "${TEMP_DIR}/rsync.log" | tr -s ' '))"
    else
        PUSH_FAILED=true
        log "ERROR: archive push to $REMOTE_SYNC_DEST failed (rsync rc=$rc) - the next run retries" error
        tail -5 "${TEMP_DIR}/rsync.log" | while read -r line; do log "  $line" error; done
    fi
}


# EXTRA NETWORK SYNCHRONIZATION (fsxNet and friends)
#################################################################################

# Get the dates already imported for one network
# Args: network_name
get_database_dates_for_domain() {
    local domain="$1"
    local response
    if ! response=$(curl -sf --retry 4 --retry-max-time 180 --connect-timeout 10 --max-time 60 "$API_BASE_URL/stats/dates?domain=${domain}"); then
        error_exit "Failed to fetch dates for network $domain from API"
    fi
    echo "$response" | grep -o '"[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]"' | tr -d '"' | sort
}

# List remote files for an extra network
# Args: source url_or_dir filename_regex
list_extra_network_files() {
    local source="$1" location="$2" name_re="$3"

    case "$source" in
        http)
            # Parse hrefs out of the HTTP index page
            curl -s --connect-timeout 15 "$location" \
                | grep -o -i -E 'href="[^"]+"' \
                | sed -E 's/^[Hh][Rr][Ee][Ff]="//; s/"$//' \
                | awk -F/ '{print $NF}' \
                | grep -i -E "^${name_re}(\.(gz|zip|z[0-9]{2}))?$" \
                | sort -u
            ;;
        ftp)
            local temp_list="${TEMP_DIR}/extra_ftp_listing.txt"
            ftp -n "$FTP_HOST" > "$temp_list" 2>&1 <<FTPCMDS
user $FTP_USER $FTP_PASS
cd $location
ls
quit
FTPCMDS
            awk '{print $NF}' "$temp_list" | grep -i -E "^${name_re}(\.(gz|zip|z[0-9]{2}))?$" | sort -u
            ;;
        *)
            error_exit "Unknown source type for extra network: $source"
            ;;
    esac
}

# Download one remote file for an extra network
# Args: source url_or_dir remote_file local_file
download_extra_network_file() {
    local source="$1" location="$2" remote_file="$3" local_file="$4"

    case "$source" in
        http)
            # -f: an error page is not a nodelist. An archive-only fetch never
            # reaches the parser, so nothing else would stop it being archived.
            curl -sf --connect-timeout 15 --retry "$MAX_RETRIES" -o "$local_file" "${location%/}/${remote_file}"
            [[ -f "$local_file" && -s "$local_file" ]]
            ;;
        ftp)
            download_ftp_file "$remote_file" "$local_file" "$location"
            ;;
    esac
}

# Decompress archive variants (.gz / .zip / .zNN) in place.
# Args: file_path  — echoes the path of the decompressed nodelist file
decompress_nodelist_archive() {
    local file_path="$1"
    local lower="${file_path,,}"

    if [[ "$lower" == *.gz ]]; then
        gunzip -f "$file_path"
        echo "${file_path%.*}"
        return 0
    fi

    if [[ "$lower" == *.zip ]] || [[ "$lower" =~ \.z[0-9]{2}$ ]]; then
        # .zNN files are usually ZIP archives holding one nodelist file
        if file "$file_path" | grep -qi zip; then
            local extract_dir="${TEMP_DIR}/extract_$$"
            mkdir -p "$extract_dir"
            unzip -o -q "$file_path" -d "$extract_dir"
            local extracted
            extracted=$(find "$extract_dir" -type f | head -1)
            [[ -n "$extracted" ]] || return 1
            local target="${TEMP_DIR}/$(basename "${extracted,,}")"
            mv "$extracted" "$target"
            rm -rf "$extract_dir" "$file_path"
            echo "$target"
            return 0
        fi
    fi

    # .lNN / .lha / .lzh — LHA archives (some pointlist series ship these)
    if [[ "$lower" =~ \.l(ha|zh|[0-9]{2})$ ]]; then
        if command -v 7z &> /dev/null; then
            local extract_dir="${TEMP_DIR}/extract_$$"
            mkdir -p "$extract_dir"
            if 7z x -y -o"$extract_dir" "$file_path" >/dev/null 2>&1; then
                local extracted
                extracted=$(find "$extract_dir" -type f | head -1)
                [[ -n "$extracted" ]] || return 1
                local target="${TEMP_DIR}/$(basename "${extracted,,}")"
                mv "$extracted" "$target"
                rm -rf "$extract_dir" "$file_path"
                echo "$target"
                return 0
            fi
            rm -rf "$extract_dir"
        fi
        log "Cannot extract LHA archive $file_path (7z missing or failed)" error
        return 1
    fi

    echo "$file_path"
}

# Sync one extra network definition ("name|source|url_or_dir|filename_regex")
sync_extra_network() {
    local spec="$1"
    local name source location name_re
    IFS='|' read -r name source location name_re <<< "$spec"

    if [[ -z "$name" || -z "$source" || -z "$location" || -z "$name_re" ]]; then
        log "Skipping malformed EXTRA_NETWORKS entry: $spec" error
        return 1
    fi

    log "--- Syncing network '$name' from $source $location ---"

    local db_dates remote_files
    db_dates=$(get_database_dates_for_domain "$name")
    remote_files=$(list_extra_network_files "$source" "$location" "$name_re")

    if [[ -z "$remote_files" ]]; then
        log "No $name nodelist files found at $location"
        return 0
    fi

    while IFS= read -r remote_file; do
        [[ -z "$remote_file" ]] && continue

        local plain_name="${remote_file%.gz}"
        plain_name="${plain_name%.zip}"
        # Strip .zNN archive extensions (fsxnet.191.z01 -> fsxnet.191)
        if [[ "${plain_name,,}" =~ \.z[0-9]{2}$ ]]; then
            plain_name="${plain_name%.*}"
        fi
        local file_date
        if ! file_date=$(filename_to_date "$plain_name"); then
            log "Warning: could not parse date from $remote_file"
            continue
        fi

        # Done only when the date is imported AND its file is archived; an
        # imported date with no file (a failed or interrupted archive write)
        # is fetched again and archived without a second import.
        local year="${file_date%%-*}"
        local archive_only=false
        if grep -q "^$file_date$" <<< "$db_dates"; then
            if [[ -s "${LOCAL_NODELIST_DIR}/${name}/${year}/${plain_name,,}.gz" ]]; then
                log "Skipping $remote_file - date $file_date already imported and archived for $name"
                continue
            fi
            archive_only=true
        fi

        local temp_file="${TEMP_DIR}/${remote_file,,}"
        if ! download_extra_network_file "$source" "$location" "$remote_file" "$temp_file"; then
            log "Failed to download $remote_file for $name - will retry next run" error
            rm -f "$temp_file"
            continue
        fi

        local nodelist_file
        if ! nodelist_file=$(decompress_nodelist_archive "$temp_file"); then
            log "Failed to unpack $remote_file for $name" error
            rm -f "$temp_file"
            continue
        fi

        # Import with the network's parser flag
        if [[ "$archive_only" == "true" ]]; then
            log "$(basename "$nodelist_file") already imported for $name - archiving only"
        else
            log "Importing $(basename "$nodelist_file") into database (network: $name)..."
            if ! "$PARSER_PATH" -path "$nodelist_file" -config "$CONFIG_PATH" -network "$name" -verbose \
                    > "${TEMP_DIR}/parser.log" 2>&1; then
                log "Parser failed for $(basename "$nodelist_file") (network $name):" error
                tail -10 "${TEMP_DIR}/parser.log" | while read -r line; do log "  $line" error; done
                rm -f "$nodelist_file"
                continue
            fi
        fi

        # Store at <root>/<network>/<year>/<name>.DDD.gz; push_archive carries
        # it to the remote mirror at the end of the run.
        local compressed_file="${LOCAL_NODELIST_DIR}/${name}/${year}/$(basename "${nodelist_file,,}").gz"
        if store_gzip "$nodelist_file" "$compressed_file"; then
            log "Stored $compressed_file ($STORE_RESULT)"
        else
            log "Failed to store $compressed_file - will retry next run" error
        fi

        rm -f "$nodelist_file"
        log "Completed $remote_file for network $name"
    done <<< "$remote_files"

    log "--- Network '$name' sync finished ---"
}

# Sync every configured extra network
sync_extra_networks() {
    if [[ ${#EXTRA_NETWORKS[@]} -eq 0 ]]; then
        return 0
    fi

    log "=== Syncing ${#EXTRA_NETWORKS[@]} additional FTN network(s) ==="
    local spec
    for spec in "${EXTRA_NETWORKS[@]}"; do
        sync_extra_network "$spec" || true
    done
}

#################################################################################
# POINTLIST SYNCHRONIZATION (FTS-5002 weeklies)
#################################################################################

# Sync one pointlist series ("name|source|url_or_dir|filename_regex|charset[|network]")
sync_extra_pointlist() {
    local spec="$1"
    local name source location name_re charset network
    IFS='|' read -r name source location name_re charset network <<< "$spec"
    network="${network:-fidonet}"

    if [[ -z "$name" || -z "$source" || -z "$location" || -z "$name_re" || -z "$charset" ]]; then
        log "Skipping malformed EXTRA_POINTLISTS entry: $spec" error
        return 1
    fi

    log "--- Syncing pointlist series '$name' from $source $location ---"

    local remote_files
    remote_files=$(list_extra_network_files "$source" "$location" "$name_re")
    if [[ -z "$remote_files" ]]; then
        log "No $name pointlist files found at $location"
        return 0
    fi

    while IFS= read -r remote_file; do
        [[ -z "$remote_file" ]] && continue

        local temp_file="${TEMP_DIR}/${remote_file,,}"
        if ! download_extra_network_file "$source" "$location" "$remote_file" "$temp_file"; then
            log "Failed to download $remote_file for pointlist $name - will retry next run" error
            rm -f "$temp_file"
            continue
        fi

        local pointlist_file
        if ! pointlist_file=$(decompress_nodelist_archive "$temp_file"); then
            log "Failed to unpack $remote_file for pointlist $name" error
            rm -f "$temp_file"
            continue
        fi

        # Compressed weekly names truncate the day (Z2PNT.Z01 -> Z2PNT.201);
        # the extracted inner filename carries the true 3-digit day.
        local plain_name
        plain_name=$(basename "$pointlist_file")
        local file_date
        if ! file_date=$(filename_to_date "$plain_name"); then
            log "Warning: could not parse day number from $plain_name" error
            rm -f "$pointlist_file"
            continue
        fi
        local year=$(echo "$file_date" | cut -d'-' -f1)

        # The parser's pointlist_files gate makes re-imports no-ops
        log "Importing pointlist $(basename "$pointlist_file") (series $name, year $year)..."
        if ! "$PARSER_PATH" -config "$CONFIG_PATH" -pointlist -network "$network" \
                -list-source "$name" -year "$year" -charset "$charset" \
                -path "$pointlist_file" > "${TEMP_DIR}/parser.log" 2>&1; then
            log "Parser failed for $(basename "$pointlist_file") (pointlist $name):" error
            tail -10 "${TEMP_DIR}/parser.log" | while read -r line; do log "  $line" error; done
            rm -f "$pointlist_file"
            continue
        fi
        # An already-imported issue still gets archived: the gate speaks for the
        # points table, not for the download tree, and skipping here would leave
        # data in the database whose file cannot be downloaded. A successful
        # parser run means the file is a valid pointlist either way.
        if grep -q "Already imported" "${TEMP_DIR}/parser.log"; then
            log "Pointlist $(basename "$pointlist_file") already imported - archiving only"
        fi

        # Archive at <root>/<network>/<source>/<year>/NAME.DDD.gz. Every issue
        # still in the file echo comes through here each run, so an unchanged
        # one must stay untouched (see store_gzip). push_archive carries new
        # ones to the remote mirror at the end of the run.
        local compressed_file="${LOCAL_POINTLIST_DIR}/${network}/${name}/${year}/$(basename "${pointlist_file,,}").gz"
        if store_gzip "$pointlist_file" "$compressed_file"; then
            log "Stored $compressed_file ($STORE_RESULT)"
        else
            log "Failed to store $compressed_file - will retry next run" error
        fi

        rm -f "$pointlist_file"
        log "Completed $remote_file for pointlist series $name"
    done <<< "$remote_files"

    log "--- Pointlist series '$name' sync finished ---"
}

# Sync every configured pointlist series
sync_extra_pointlists() {
    if [[ ${#EXTRA_POINTLISTS[@]} -eq 0 ]]; then
        return 0
    fi

    # Same lock file as scripts/import_pointlists.sh: the points table is a
    # plain MergeTree (no dedup), so the weekly sync and an hours-long bulk
    # import must never race on the same (domain, source, date) key.
    #
    # The fd is opened READ-ONLY, which looks wrong for a lock and is not:
    # fs.protected_regular is 2 on the production host (a Debian/Ubuntu
    # default), so opening a regular file for WRITING in a sticky
    # world-writable directory such as /tmp is refused when another user owns
    # it — root included. The bulk importer runs as an ordinary user and this
    # script runs as root from cron, so the previous `exec 201>` died with
    # EACCES; and because a failed redirect on `exec` terminates a
    # non-interactive shell, that one line silently killed the whole sync
    # before any pointlist was fetched. flock(2) needs no write access.
    [[ -e "$POINTLIST_LOCK_FILE" ]] || : > "$POINTLIST_LOCK_FILE" 2>/dev/null || true

    # Redirecting a subshell (rather than exec) keeps a failure local, so an
    # unusable lock path degrades to a logged skip instead of a dead script.
    (
        if ! flock -n 201; then
            log "Pointlist sync skipped: a bulk pointlist import is running (lock held)" error
            exit 0
        fi

        log "=== Syncing ${#EXTRA_POINTLISTS[@]} pointlist series ==="
        local spec
        for spec in "${EXTRA_POINTLISTS[@]}"; do
            sync_extra_pointlist "$spec" || true
        done
    ) 201<"$POINTLIST_LOCK_FILE" \
        || log "Pointlist sync skipped: cannot lock $POINTLIST_LOCK_FILE" error
}

#################################################################################
# MAIN SYNCHRONIZATION LOGIC
#################################################################################

# Main function that orchestrates the entire sync process
sync_nodelists() {
    log "=== Starting NodelistDB synchronization process ==="
    
    # Create temporary directory for downloads
    mkdir -p "$TEMP_DIR"
    log "Using temporary directory: $TEMP_DIR"
    
    # Step 1: Get current state of database via API
    log "Step 1: Checking database state..."
    local db_dates
    db_dates=$(get_database_dates)
    
    # Step 2: Get available files from FTP server  
    log "Step 2: Checking FTP server..."
    local ftp_files
    ftp_files=$(get_ftp_listing "$FTP_DIR")
    
    if [[ -z "$ftp_files" ]]; then
        log "No nodelist files found on FTP server - nothing to sync"
        return 0
    fi
    
    # Step 3: Determine which files need to be downloaded
    log "Step 3: Determining files to download..."
    local files_to_download=()
    local files_skipped=0
    # Files whose date is imported but whose archive file is missing: fetched
    # again and archived, never imported twice. Without this, an archive write
    # that failed after a successful import (full disk, a kill in between) left
    # a date that no later run would ever archive.
    local -A archive_only=()

    # Check each FTP file against database
    while IFS= read -r ftp_file; do
        [[ -z "$ftp_file" ]] && continue

        # Convert filename to date
        local file_date
        if file_date=$(filename_to_date "$ftp_file"); then

            # Check if this date already exists in database
            if grep -q "^$file_date$" <<< "$db_dates"; then
                if [[ -s "$(fidonet_archive_path "$ftp_file" "$file_date")" ]]; then
                    log "Skipping $ftp_file - date $file_date already in database and archived"
                    files_skipped=$((files_skipped + 1))
                else
                    log "Need to archive: $ftp_file (date: $file_date in database, file missing)"
                    files_to_download+=("$ftp_file")
                    archive_only["$ftp_file"]=1
                fi
            else
                log "Need to download: $ftp_file (date: $file_date)"
                files_to_download+=("$ftp_file")
            fi
        else
            log "Warning: Could not parse date from filename: $ftp_file"
        fi
        
    done <<< "$ftp_files"
    
    # Report what we found
    log "Analysis complete:"
    log "  - Files on FTP: $(echo "$ftp_files" | wc -l)"
    log "  - Files to download: ${#files_to_download[@]}"
    log "  - Files skipped (already in DB): $files_skipped"
    
    # Exit early if nothing to download
    if [[ ${#files_to_download[@]} -eq 0 ]]; then
        log "=== All files are up to date - no synchronization needed ==="
        return 0
    fi
    
    # Step 4: Download and process each file (no server downtime needed with ClickHouse)
    log "Step 4: Processing ${#files_to_download[@]} files..."
    local success_count=0
    local failed_files=()
    local total_files=${#files_to_download[@]}

    for i in "${!files_to_download[@]}"; do
        local ftp_file="${files_to_download[$i]}"
        local progress="$((i + 1))/$total_files"

        log "Processing file $progress: $ftp_file"

        # Download file to temp directory
        local temp_file="${TEMP_DIR}/${ftp_file}"
        if ! download_ftp_file "$ftp_file" "$temp_file" "$FTP_DIR"; then
            log "Skipping $ftp_file - will retry on next sync run" error
            failed_files+=("$ftp_file")
            rm -f "$temp_file"
            continue
        fi

        # If this is a z2daily file, rename it to nodelist format for processing
        local processed_file="$temp_file"
        if [[ "$ftp_file" =~ ^[zZ]2[dD][aA][iI][lL][yY]\. ]]; then
            processed_file="${TEMP_DIR}/$(fidonet_archive_name "$ftp_file")"
            log "Renaming z2daily file: $ftp_file -> $(basename "$processed_file")"
            mv "$temp_file" "$processed_file"
        fi

        # Import file into database (unless only its archive copy is missing)
        if [[ -n "${archive_only[$ftp_file]:-}" ]]; then
            log "$(basename "$processed_file") already in database - archiving only"
        else
            import_to_database "$processed_file"
        fi

        # Compress and store file in proper location. The remote mirror gets
        # it from push_archive at the end of the run.
        compress_and_store "$processed_file"

        # Clean up temporary file
        rm -f "$processed_file"
        
        success_count=$((success_count + 1))
        log "Completed processing $ftp_file ($progress)"
    done
    
    # Final summary (no server restart needed)
    if [[ ${#failed_files[@]} -eq 0 ]]; then
        log "=== Synchronization completed successfully ==="
    else
        log "=== Synchronization completed with ${#failed_files[@]} failed download(s) ==="
        log "Failed (will retry next run): ${failed_files[*]}" error
    fi
    log "Files processed: $success_count/$total_files"

    if [[ $success_count -gt 0 ]]; then
        log "Updated files:"
        for file in "${files_to_download[@]}"; do
            [[ " ${failed_files[*]:-} " == *" $file "* ]] && continue
            local file_date=$(filename_to_date "$file")
            if [[ "$file" =~ ^[zZ]2[dD][aA][iI][lL][yY]\. ]]; then
                log "  - $file -> $(fidonet_archive_name "$file") (date: $file_date)"
            else
                log "  - $file (date: $file_date)"
            fi
        done
    fi
    
    log "Database and file storage are now synchronized with FTP server"
    log "Note: ClickHouse allows concurrent operations - server remained online during sync"
}

#################################################################################
# CONFIGURATION FILE MANAGEMENT
#################################################################################

# Create a sample configuration file if none exists
create_sample_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        log "Creating sample configuration file: $CONFIG_FILE"
        
        cat > "$CONFIG_FILE" <<'EOF'
# NodelistDB FTP Synchronization Configuration
# 
# Edit this file with your specific FTP server details and paths

# FTP Server Configuration (REQUIRED)
FTP_HOST="ftp.example.com"                    # FTP server hostname or IP
FTP_USER="username"                           # FTP username
FTP_PASS="password"                           # FTP password  
FTP_DIR="/pub/fidonet/nodelist"               # Remote directory containing nodelist files

# Local System Paths
LOCAL_NODELIST_DIR="/home/dp/nodelists"       # Where to store compressed nodelist files
CONFIG_PATH="./config.yaml"                   # Configuration file path for NodelistDB (YAML format)
API_BASE_URL="http://localhost:8080/api"      # NodelistDB API endpoint

# Parser Configuration
PARSER_PATH="./bin/parser"                    # Path to parser binary

# Remote server sync configuration: the whole archive is rsync'ed there at
# the end of each run (never with --delete). Empty REMOTE_SYNC_HOST = no push.
REMOTE_SYNC_HOST="root@nodelist.5001.ru"         # Remote server to sync nodelists to
REMOTE_NODELIST_DIR="/opt/nodelists"             # Remote nodelist directory
# With an rrsync-restricted key, name the destination relative to the pinned dir:
# REMOTE_SYNC_DEST="root@nodelist.5001.ru:"

# Pointlist series to sync (FTS-5002 weeklies). Empty means none.
# "name|source|url_or_dir|filename_regex|charset[|network]"
# EXTRA_POINTLISTS=("z2|ftp|/z2pnt|z2pnt\.z[0-9]{2}|cp437")
#
# The archive root defaults to <nodelist root>/pointlists, which is what lets
# the archive push and any nodelist mirror carry pointlists too. Override only
# if it must live somewhere else:
# LOCAL_POINTLIST_DIR="/opt/nodelists/pointlists"

# Download Configuration
MAX_RETRIES=3                                 # Number of retry attempts for failed downloads
TEMP_DIR="/tmp/nodelist_sync"                 # Temporary directory for downloads

# System Service
SERVICE_NAME="nodelistdb"                     # systemd service name for the server

# Example FTP configurations for common FidoNet nodelist sources:
#
# Zone 1 (North America):
# FTP_HOST="members.shaw.ca"
# FTP_DIR="/nnftp/f-prot"
#
# Zone 2 (Europe):  
# FTP_HOST="ftp.fidotel.com"
# FTP_DIR="/nodelist"
EOF
        
        log "Please edit $CONFIG_FILE with your FTP server details before running sync"
        log "The script will not proceed without proper FTP configuration"
        return 1
    fi
    return 0
}

#################################################################################
# COMMAND LINE INTERFACE
#################################################################################

# Display usage information
usage() {
    cat <<EOF
NodelistDB FTP Synchronization Script

DESCRIPTION:
    Automatically synchronizes FidoNet nodelist files from an FTP server to the
    local NodelistDB database and compressed file storage.

    Uses ClickHouse backend configured via YAML file (config.yaml).
    ClickHouse supports concurrent writes, eliminating server downtime needs.

USAGE:
    $0 [OPTIONS]

OPTIONS:
    -h, --help              Show this help message and exit
    -c, --config FILE       Use alternate configuration file (default: $CONFIG_FILE)
    -v, --verbose           Enable verbose logging and debug output
    -q, --quiet             Quiet mode - only show errors on console (for cron)
    --dry-run              Show what would be synchronized without making changes
    --check-config         Validate configuration and exit
    --server-status        Show current server status and exit

CONFIGURATION:
    Edit the configuration file to set your FTP server details:
    $CONFIG_FILE

    Required settings:
    - FTP_HOST: FTP server hostname
    - FTP_USER: FTP username
    - FTP_PASS: FTP password
    - FTP_DIR: Remote directory path
    - CONFIG_PATH: Path to NodelistDB YAML configuration file (config.yaml)
    - PARSER_PATH: Path to parser binary (default: ./bin/parser)

EXAMPLES:
    $0                      # Run normal synchronization
    $0 --check-config       # Validate configuration file
    $0 --dry-run           # Preview what would be downloaded
    $0 --verbose           # Run with detailed logging
    $0 --quiet             # Quiet mode for cron (only errors to console)
    $0 --server-status     # Check if server is running

FILES:
    Config: $CONFIG_FILE
    Log:    $LOG_FILE
    Temp:   $TEMP_DIR

For more information, see the NodelistDB documentation.
EOF
}

# Show current server status
show_server_status() {
    echo "NodelistDB Server Status:"
    echo "========================="
    echo "Service name: $SERVICE_NAME"
    
    if is_server_running; then
        echo "Status: RUNNING"
        echo "API Health: $(curl -s "$API_BASE_URL/health" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || echo "Not responding")"
    else
        echo "Status: STOPPED"
    fi
    
    echo "API URL: $API_BASE_URL"
    echo "Config: $CONFIG_PATH"
}

# Validate configuration and show status
check_config() {
    log "Checking configuration..."
    
    echo "Configuration Status:"
    echo "===================="
    echo "Config file: $CONFIG_FILE"
    echo "Exists: $(if [[ -f "$CONFIG_FILE" ]]; then echo "YES"; else echo "NO"; fi)"
    echo
    
    if [[ -f "$CONFIG_FILE" ]]; then
        echo "Settings:"
        echo "  FTP_HOST: ${FTP_HOST:-"(not set)"}"
        echo "  FTP_USER: ${FTP_USER:-"(not set)"}" 
        echo "  FTP_DIR: ${FTP_DIR:-"(not set)"}"
        echo "  LOCAL_NODELIST_DIR: $LOCAL_NODELIST_DIR"
        echo "  CONFIG_PATH: $CONFIG_PATH"
        echo "  PARSER_PATH: $PARSER_PATH"
        echo "  API_BASE_URL: $API_BASE_URL"
        echo
        
        # Test API connection
        echo "API Connection Test:"
        if curl -s --connect-timeout 5 "$API_BASE_URL/health" > /dev/null; then
            echo "  ✓ API is reachable"
        else
            echo "  ✗ API is not reachable (is server running?)"
        fi
        
        # Check database configuration
        echo "Database Configuration:"
        if [[ -f "$CONFIG_PATH" ]]; then
            echo "  ✓ Config file exists: $CONFIG_PATH"
            # Check for ClickHouse configuration section
            if grep -q "^clickhouse:" "$CONFIG_PATH"; then
                echo "  Database type: ClickHouse"
                # Extract host and port if available
                local ch_host=$(grep "^\s*host:" "$CONFIG_PATH" | head -1 | awk '{print $2}')
                local ch_port=$(grep "^\s*port:" "$CONFIG_PATH" | head -1 | awk '{print $2}')
                local ch_db=$(grep "^\s*database:" "$CONFIG_PATH" | head -1 | awk '{print $2}')
                [[ -n "$ch_host" ]] && echo "  ClickHouse host: $ch_host"
                [[ -n "$ch_port" ]] && echo "  ClickHouse port: $ch_port"
                [[ -n "$ch_db" ]] && echo "  Database name: $ch_db"
            else
                echo "  ✗ No ClickHouse configuration found in config"
            fi
        else
            echo "  ✗ Config file not found: $CONFIG_PATH"
        fi

        # Check parser binary
        echo
        echo "Parser Binary:"
        if [[ -x "$PARSER_PATH" ]]; then
            echo "  ✓ Parser exists and is executable: $PARSER_PATH"
        else
            echo "  ✗ Parser not found or not executable: $PARSER_PATH"
        fi
        
        echo
        check_dependencies
        
    else
        echo "Configuration file does not exist."
        echo "Run the script to create a sample configuration file."
    fi
}

#################################################################################
# MAIN SCRIPT EXECUTION
#################################################################################

# Main entry point - parse arguments and execute appropriate action
main() {
    local dry_run=false
    local verbose=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -c|--config)
                if [[ -z "${2:-}" ]]; then
                    error_exit "Option --config requires an argument"
                fi
                CONFIG_FILE="$2"
                shift 2
                ;;
            -v|--verbose)
                verbose=true
                shift
                ;;
            -q|--quiet)
                QUIET_MODE=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            --check-config)
                check_config
                exit 0
                ;;
            --server-status)
                show_server_status
                exit 0
                ;;
            *)
                error_exit "Unknown option: $1. Use --help for usage information."
                ;;
        esac
    done
    
    # Enable debug output if verbose mode requested
    if [[ "$verbose" == "true" ]]; then
        set -x
        log "Verbose mode enabled"
    fi
    
    # Start main process
    log "=== NodelistDB FTP Sync Script Starting ==="
    log "Configuration file: $CONFIG_FILE"
    log "Log file: $LOG_FILE"
    log "Process ID: $$"
    
    # Create sample configuration if needed
    if ! create_sample_config; then
        exit 1
    fi

    # One run at a time. Taken before the cleanup trap exists and without
    # error_exit, so a run that loses never touches the winner's TEMP_DIR.
    # Opened read-only for the same reason as the pointlist lock below:
    # fs.protected_regular refuses a write-open of another user's file.
    [[ -e "$SYNC_LOCK_FILE" ]] || : > "$SYNC_LOCK_FILE" 2>/dev/null || true
    if [[ ! -r "$SYNC_LOCK_FILE" ]]; then
        log "ERROR: cannot open lock file $SYNC_LOCK_FILE" error
        exit 1
    fi
    exec 200<"$SYNC_LOCK_FILE"
    if ! flock -n 200; then
        log "Another sync run holds $SYNC_LOCK_FILE - exiting" error
        exit 0
    fi
    SYNC_LOCK_HELD=true

    # Set up cleanup trap
    trap cleanup EXIT
    
    # Validate system and configuration
    check_dependencies
    validate_config
    
    # Handle dry-run mode
    if [[ "$dry_run" == "true" ]]; then
        log "=== DRY RUN MODE - No changes will be made ==="
        
        # Create temporary directory for dry-run operations
        mkdir -p "$TEMP_DIR"
        
        # Get database and FTP state without making changes
        local db_dates ftp_files
        db_dates=$(get_database_dates)
        ftp_files=$(get_ftp_listing "$FTP_DIR")
        
        log "Dry run analysis:"
        log "  Database contains $(echo "$db_dates" | wc -l) dates"
        log "  FTP server has $(echo "$ftp_files" | wc -l) files"
        
        # Show what would be downloaded
        local would_download=0
        while IFS= read -r ftp_file; do
            [[ -z "$ftp_file" ]] && continue
            local file_date
            if file_date=$(filename_to_date "$ftp_file"); then
                # A herestring, not `echo | grep -q`: under pipefail, grep -q
                # quitting at an early match SIGPIPEs echo and the pipeline
                # reads as "not found" (it flagged two imported dates).
                if ! grep -q "^$file_date$" <<< "$db_dates"; then
                    log "  Would download: $ftp_file (date: $file_date)"
                    would_download=$((would_download + 1))
                elif [[ ! -s "$(fidonet_archive_path "$ftp_file" "$file_date")" ]]; then
                    log "  Would archive only: $ftp_file (date: $file_date in database, file missing)"
                    would_download=$((would_download + 1))
                fi
            fi
        done <<< "$ftp_files"
        
        log "=== Dry run complete - would download $would_download files ==="
        exit 0
    fi
    
    # Run the actual synchronization (fidonet first, then extra networks,
    # then pointlist series)
    SYNC_STARTED=true
    sync_nodelists
    sync_extra_networks
    sync_extra_pointlists
    push_archive

    if [[ "$PUSH_FAILED" == "true" ]]; then
        log "=== NodelistDB FTP Sync Script Completed, but the archive push FAILED ===" error
        exit 1
    fi
    log "=== NodelistDB FTP Sync Script Completed Successfully ==="
}

# Execute main function with all command line arguments
main "$@"
