#!/bin/bash
#
# First boot creates MBSE's databases and stamps in the test zone identity;
# every boot after that starts mbtask and the three per-connection listeners.
set -euo pipefail

: "${TESTZONE_AKA:=999:1/1}"
: "${TESTZONE_SYSTEM:=MBSE Test Node}"
: "${TESTZONE_SYSOP:=Test Sysop}"
: "${TESTZONE_LOCATION:=Test Zone}"
: "${TESTZONE_FLAGS:=CM,IBN,IFC,ITN,XA}"
: "${BINKP_PORT:=24554}"
: "${IFCICO_PORT:=60179}"
: "${TELNET_PORT:=23}"

export MBSE_ROOT=/opt/mbse

log() { echo "[testzone] $*"; }

# mbtask is the piece that writes config.data on a virgin tree and then
# launches "mbsetup init" to build the rest of the databases. It is also what
# mbcico registers with at the start of every session, so it stays running.
start_mbtask() {
    /opt/mbse/bin/mbtask -nd &
    MBTASK_PID=$!
}

if [ ! -f "${MBSE_ROOT}/etc/config.data" ]; then
    log "first boot: creating MBSE databases"
    start_mbtask
    for _ in $(seq 1 60); do
        [ -f "${MBSE_ROOT}/etc/config.data" ] && break
        sleep 1
    done
    if [ ! -f "${MBSE_ROOT}/etc/config.data" ]; then
        log "FATAL: mbtask never created config.data"
        exit 1
    fi
    # mbsetup init is launched by mbtask and builds the remaining databases;
    # give it room to finish before the config is rewritten underneath it.
    sleep 8
    kill "${MBTASK_PID}" 2>/dev/null || true
    wait "${MBTASK_PID}" 2>/dev/null || true

    log "stamping test zone identity"
    su mbse -c "MBSE_ROOT=${MBSE_ROOT} /usr/local/bin/patch-config \
        '${TESTZONE_AKA}' '${TESTZONE_SYSTEM}' '${TESTZONE_SYSOP}' \
        '${TESTZONE_LOCATION}' '${TESTZONE_FLAGS}'"
fi

log "starting mbtask"
start_mbtask
sleep 3

# A fresh MBSE is "closed", and mbcico answers a closed system with M_BSY
# (binkp.c WaitConn asks mbtask "SBBS:0;" before anything else). A test node
# that refuses every call is no use, so open it on every boot -- the flag
# lives in mbtask's status, not in config.data, so it does not survive a
# restart.
su mbse -c "MBSE_ROOT=${MBSE_ROOT} /opt/mbse/bin/mbstat open" >/dev/null 2>&1 \
    && log "system open for calls" \
    || log "WARNING: could not open the system; calls will get M_BSY"

# socat's nofork hands the accepted socket straight to mbcico as fd 0/1,
# which is the inetd contract mbcico is written against: it reads the session
# from stdin and calls getpeername(0) to log who connected. Without nofork
# socat would relay through a pipe and mbcico would see no peer at all.
listen() {
    local port="$1" mode="$2" name="$3"
    log "listening for ${name} on ${port} (mbcico -t ${mode})"
    socat "TCP-LISTEN:${port},fork,reuseaddr" \
          "EXEC:/opt/mbse/bin/mbcico -t ${mode},nofork,su=mbse" &
}

listen "${BINKP_PORT}"  ibn "binkp"
listen "${IFCICO_PORT}" ifc "ifcico/EMSI"
listen "${TELNET_PORT}" itn "EMSI over telnet"

# The mailer log is the whole reason this container exists: it is the view a
# sysop has of our sessions, and the only place an untidy ending shows up.
touch "${MBSE_ROOT}/log/mbcico.log" 2>/dev/null || true
log "ready -- mailer log follows"
exec tail -n 0 -F "${MBSE_ROOT}/log/mbcico.log" 2>/dev/null
