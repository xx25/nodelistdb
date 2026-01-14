#!/bin/bash

# NodelistDB FTP Synchronization Script
#
# This script automates the process of:
# 1. Connecting to an FTP server to get available nodelist files
# 2. Checking which files are already imported in the database via API
# 3. Downloading missing files from FTP
# 4. Importing new files with the parser
# 5. Compressing and storing files in the proper directory structure
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
PARSER_PATH="${SCRIPT_DIR}/parser"                   # Path to parser binary

# Load configuration file if it exists
if [[ -f "$CONFIG_FILE" ]]; then
    source "$CONFIG_FILE"
fi

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
cleanup() {
    if [[ -d "$TEMP_DIR" ]]; then
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

    local deps=("curl" "ftp" "gzip" "systemctl")
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
    if ! curl -s --connect-timeout 5 "$API_BASE_URL/health" > /dev/null; then
        error_exit "Cannot reach NodelistDB API at $API_BASE_URL. Is the server running?"
    fi

    log "Configuration validation passed"
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

    local response
    if ! response=$(curl -s --connect-timeout 10 "$API_BASE_URL/stats/dates"); then
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
# Logic: Day number of year, adjusting for year boundaries
filename_to_date() {
    local filename="$1"

    # Extract day number from filename (e.g., nodelist.276 -> 276, z2daily.276 -> 276)
    if [[ "$filename" =~ (nodelist|z2daily)\.([0-9]{3})$ ]]; then
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
        ((retry++))
        if [[ $retry -lt $MAX_RETRIES ]]; then
            log "Retry attempt $retry/$MAX_RETRIES for $remote_file in 5 seconds..."
            sleep 5
        fi
    done

    error_exit "Failed to download $remote_file after $MAX_RETRIES attempts"
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

# Compress and store a nodelist file in the proper directory structure
# Args: source_file_path
# Creates: $LOCAL_NODELIST_DIR/YYYY/nodelist.XXX.gz
compress_and_store() {
    local file_path="$1"
    local filename=$(basename "$file_path")

    # Determine the year for this file based on its date
    local file_date
    if ! file_date=$(filename_to_date "$filename"); then
        error_exit "Could not determine date for file: $filename"
    fi

    local year=$(echo "$file_date" | cut -d'-' -f1)
    local year_dir="${LOCAL_NODELIST_DIR}/${year}"

    # Create year directory if it doesn't exist
    mkdir -p "$year_dir"

    # Compress file with .gz extension
    local compressed_file="${year_dir}/${filename,,}.gz"  # Convert filename to lowercase

    log "Compressing and storing $filename -> $compressed_file"

    # Use gzip to compress the file
    if gzip -c "$file_path" > "$compressed_file"; then
        # Verify compressed file was created successfully
        if [[ -f "$compressed_file" && -s "$compressed_file" ]]; then
            local original_size=$(stat -c%s "$file_path")
            local compressed_size=$(stat -c%s "$compressed_file")
            local ratio=$(( (original_size - compressed_size) * 100 / original_size ))

            log "Successfully compressed $filename (${ratio}% reduction: ${original_size} -> ${compressed_size} bytes)"
            return 0
        else
            error_exit "Compressed file was not created properly: $compressed_file"
        fi
    else
        error_exit "Failed to compress file: $filename"
    fi
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

    # Check each FTP file against database
    while IFS= read -r ftp_file; do
        [[ -z "$ftp_file" ]] && continue

        # Convert filename to date
        local file_date
        if file_date=$(filename_to_date "$ftp_file"); then

            # Check if this date already exists in database
            if grep -q "^$file_date$" <<< "$db_dates"; then
                log "Skipping $ftp_file - date $file_date already in database"
                files_skipped=$((files_skipped + 1))
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
    local total_files=${#files_to_download[@]}

    for i in "${!files_to_download[@]}"; do
        local ftp_file="${files_to_download[$i]}"
        local progress="$((i + 1))/$total_files"

        log "Processing file $progress: $ftp_file"

        # Download file to temp directory
        local temp_file="${TEMP_DIR}/${ftp_file}"
        download_ftp_file "$ftp_file" "$temp_file" "$FTP_DIR"

        # If this is a z2daily file, rename it to nodelist format for processing
        local processed_file="$temp_file"
        if [[ "$ftp_file" =~ ^[zZ]2[dD][aA][iI][lL][yY]\. ]]; then
            local nodelist_name="${ftp_file//[zZ]2[dD][aA][iI][lL][yY]/nodelist}"
            processed_file="${TEMP_DIR}/${nodelist_name,,}"  # Convert to lowercase
            log "Renaming z2daily file: $ftp_file -> $(basename "$processed_file")"
            mv "$temp_file" "$processed_file"
        fi

        # Import file into database
        import_to_database "$processed_file"

        # Compress and store file in proper location
        compress_and_store "$processed_file"

        # Clean up temporary file
        rm -f "$processed_file"

        success_count=$((success_count + 1))
        log "Completed processing $ftp_file ($progress)"
    done

    # Final summary (no server restart needed)
    log "=== Synchronization completed successfully ==="
    log "Files processed: $success_count/$total_files"

    if [[ $success_count -gt 0 ]]; then
        log "Updated files:"
        for file in "${files_to_download[@]}"; do
            local file_date=$(filename_to_date "$file")
            if [[ "$file" =~ ^[zZ]2[dD][aA][iI][lL][yY]\. ]]; then
                local nodelist_name="${file//[zZ]2[dD][aA][iI][lL][yY]/nodelist}"
                log "  - $file -> ${nodelist_name,,} (date: $file_date)"
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

# Download Configuration
MAX_RETRIES=3                                 # Number of retry attempts for failed downloads
TEMP_DIR="/tmp/nodelist_sync"                 # Temporary directory for downloads

# System Service
SERVICE_NAME="nodelistdb"                     # systemd service name for the server

# Example FTP configurations for common FidoNet nodelist sources:
#
# Zone 1 (North America):
# FTP_HOST="ftp.fidonet.org"
# FTP_DIR="/nodelist"
#
# Zone 2 (Europe):
# FTP_HOST="ftp.fidonet.org"
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
            echo "  API is reachable"
        else
            echo "  API is not reachable (is server running?)"
        fi

        # Check database configuration
        echo "Database Configuration:"
        if [[ -f "$CONFIG_PATH" ]]; then
            echo "  Config file exists: $CONFIG_PATH"
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
                echo "  No ClickHouse configuration found in config"
            fi
        else
            echo "  Config file not found: $CONFIG_PATH"
        fi

        # Check parser binary
        echo
        echo "Parser Binary:"
        if [[ -x "$PARSER_PATH" ]]; then
            echo "  Parser exists and is executable: $PARSER_PATH"
        else
            echo "  Parser not found or not executable: $PARSER_PATH"
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
                if ! echo "$db_dates" | grep -q "^$file_date$"; then
                    log "  Would download: $ftp_file (date: $file_date)"
                    ((would_download++))
                fi
            fi
        done <<< "$ftp_files"

        log "=== Dry run complete - would download $would_download files ==="
        exit 0
    fi

    # Run the actual synchronization
    sync_nodelists

    log "=== NodelistDB FTP Sync Script Completed Successfully ==="
}

# Execute main function with all command line arguments
main "$@"
