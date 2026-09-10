#!/bin/bash
#
# Drive a mailer under test against the zone and judge it the way its sysop
# would: by what the remote mailer wrote in its own log.
#
# A protocol test that "succeeds" on our side can still have left the peer
# logging an error, and that is the failure this zone exists to catch, so the
# verdict here is the peer's log, not our exit status.
set -uo pipefail

ZONE_IP="${ZONE_IP:-172.28.0.10}"
CONTAINER="${CONTAINER:-testzone-mbse}"
DAEMON="${DAEMON:-../bin/testdaemon}"
CONFIG="${CONFIG:-}"
LOGDIR="${LOGDIR:-$(mktemp -d)}"

if [ -z "${CONFIG}" ]; then
    echo "usage: CONFIG=/path/to/testdaemon.yaml $0" >&2
    echo "  the config needs protocols.telnet.enabled: true to exercise ITN," >&2
    echo "  and a ClickHouse the daemon can reach (results are dry-run only)." >&2
    exit 2
fi
if ! docker ps --format '{{.Names}}' | grep -qx "${CONTAINER}"; then
    echo "test zone is not running: make up" >&2
    exit 2
fi

# Everything the mailer logs from here on belongs to this run.
mark_now() { docker exec "${CONTAINER}" bash -c 'wc -l < /opt/mbse/log/system.log' | tr -d "[:space:]"; }
MARK=$(mark_now)

fail=0
run() {
    local proto="$1" port="$2"
    printf '%-8s ' "${proto}"
    # Captured rather than piped: grep -q would close the pipe early, the
    # daemon would take a SIGPIPE, and pipefail would call a good run bad.
    timeout 180 "${DAEMON}" -config "${CONFIG}" \
            -test-node "${ZONE_IP}:${port}" -test-proto "${proto}" \
            -dry-run -log-file "${LOGDIR}/${proto}.log" \
            >"${LOGDIR}/${proto}.out" 2>&1
    if grep -q "Success: true" "${LOGDIR}/${proto}.out"; then
        printf 'caller=ok   '
    else
        printf 'caller=FAIL '
        fail=1
    fi

    sleep 3
    # mbcico marks trouble with '!' (error) and '?' (attention) in column 1.
    local bad
    bad=$(docker exec "${CONTAINER}" \
        bash -c "tail -n +$((MARK + 1)) /opt/mbse/log/system.log | grep -cE '^[!?]'" 2>/dev/null | tr -d "[:space:]")
    bad=${bad:-0}
    # "Unexpected remote password" and the missing node.files are what any
    # unknown caller provokes on a node with no nodelist; they are not ours.
    local real
    real=$(docker exec "${CONTAINER}" \
        bash -c "tail -n +$((MARK + 1)) /opt/mbse/log/system.log | grep -E '^[!?]' | grep -vcE 'Unexpected remote password|node.files'" 2>/dev/null | tr -d "[:space:]")
    real=${real:-0}
    if [ "${real}" -gt 0 ]; then
        printf 'mailer=FAIL (%s error line(s))\n' "${real}"
        docker exec "${CONTAINER}" bash -c \
            "tail -n +$((MARK + 1)) /opt/mbse/log/system.log | grep -E '^[!?]' | grep -vE 'Unexpected remote password|node.files' | sed 's/^/           /'"
        fail=1
    else
        printf 'mailer=ok (%s benign)\n' "${bad}"
    fi
    mark_now() { docker exec "${CONTAINER}" bash -c 'wc -l < /opt/mbse/log/system.log' | tr -d "[:space:]"; }
MARK=$(mark_now)
}

echo "zone ${ZONE_IP} -- caller logs in ${LOGDIR}"
run binkp  24554
run ifcico 60179
run telnet 23

if [ "${fail}" -eq 0 ]; then
    echo "PASS: every session completed and the mailer logged no errors"
else
    echo "FAIL: see above"
fi
exit "${fail}"
