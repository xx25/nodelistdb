#!/usr/bin/env bash
# Phone + desktop layout audit of a running server, with an optional desktop
# screenshot diff against a baseline run.
#
#   scripts/mobile-audit/run.sh [-b http://127.0.0.1:8080] [-o out-dir] [-B baseline-dir]
#
# Exit status is non-zero when the phone audit finds a problem (document
# overflow, a clipped table, a dropdown outside the viewport, a non-200 page)
# or when a desktop screenshot differs from the baseline by more than
# DIFF_THRESHOLD percent of pixels (default 0.5). Pass -B to a directory
# produced by an earlier run of this script to get the diff.
set -euo pipefail

BASE=http://127.0.0.1:8080
OUT=./mobile-audit-out
BASELINE=
while getopts "b:o:B:" opt; do
  case $opt in
    b) BASE=$OPTARG ;;
    o) OUT=$OPTARG ;;
    B) BASELINE=$OPTARG ;;
    *) echo "usage: $0 [-b base-url] [-o out-dir] [-B baseline-dir]" >&2; exit 2 ;;
  esac
done
HERE=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT/phone" "$OUT/desktop"

echo "### phone (390x844, touch, iPhone UA) -> $OUT/phone"
set +e
BASE=$BASE OUT=$OUT/phone WIDTH=390 MOBILE=1 FAIL=1 node "$HERE/audit.mjs" | tee "$OUT/phone.log"
phone_rc=${PIPESTATUS[0]}
set -e

echo
echo "### desktop (1280 wide) -> $OUT/desktop"
BASE=$BASE OUT=$OUT/desktop WIDTH=1280 MOBILE=0 SEARCH=0 node "$HERE/audit.mjs" > "$OUT/desktop.log"
grep -c "^===" "$OUT/desktop.log" | sed 's/$/ desktop pages rendered/'

diff_rc=0
if [ -n "$BASELINE" ]; then
  echo
  echo "### desktop screenshot diff against $BASELINE/desktop (threshold ${DIFF_THRESHOLD:-0.5}%)"
  python3 -I - "$BASELINE/desktop" "$OUT/desktop" "${DIFF_THRESHOLD:-0.5}" <<'EOF' || diff_rc=$?
import sys, os
from PIL import Image, ImageChops
base, cur, thr = sys.argv[1], sys.argv[2], float(sys.argv[3])
bad = 0
for name in sorted(os.listdir(cur)):
    if not name.endswith('.png'): continue
    b = os.path.join(base, name)
    if not os.path.exists(b):
        print(f"  {name}: no baseline"); continue
    a = Image.open(b).convert('RGB'); c = Image.open(os.path.join(cur, name)).convert('RGB')
    if a.size != c.size:
        # Height differs: compare the overlapping top part and report the size change.
        h = min(a.height, c.height); w = min(a.width, c.width)
        a2, c2 = a.crop((0, 0, w, h)), c.crop((0, 0, w, h)); note = f" (size {a.size}->{c.size})"
    else:
        a2, c2, note = a, c, ""
    diff = ImageChops.difference(a2, c2).convert('L').point(lambda v: 255 if v > 16 else 0)
    pct = 100.0 * sum(diff.histogram()[255:]) / (diff.width * diff.height)
    flag = "  <-- CHANGED" if pct > thr or note else ""
    if flag: bad += 1
    print(f"  {name}: {pct:.2f}%{note}{flag}")
sys.exit(1 if bad else 0)
EOF
fi

echo
[ "$phone_rc" = 0 ] && echo "phone audit: OK" || echo "phone audit: PROBLEMS (see $OUT/phone.log)"
[ "$diff_rc" = 0 ] && echo "desktop diff: OK" || echo "desktop diff: CHANGED pages (see above)"
exit $(( phone_rc != 0 || diff_rc != 0 ))
