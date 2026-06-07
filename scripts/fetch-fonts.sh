#!/usr/bin/env bash
# Downloads the curated card-font roster (static TTFs) into assets/fonts so the Go
# renderer can draw real fonts that match the dashboard's font picker. Fonts are
# OFL-licensed (redistribution OK) but kept out of git — run `make fonts` once per
# checkout / in your deploy image. Idempotent: existing files are skipped.
#
# The family list MUST stay in sync with FONTS in web/src/lib/layout/fonts.ts and
# the registry in internal/imaging/fonts.go (same family display names).
set -euo pipefail

DEST="${1:-assets/fonts}"
BASE="https://github.com/google/fonts/raw/main"
mkdir -p "$DEST"

# "OutputBase|regularPath|boldPath(optional)"
ENTRIES='
Lato|ofl/lato/Lato-Regular.ttf|ofl/lato/Lato-Bold.ttf
Poppins|ofl/poppins/Poppins-Regular.ttf|ofl/poppins/Poppins-Bold.ttf
Kanit|ofl/kanit/Kanit-Regular.ttf|ofl/kanit/Kanit-Bold.ttf
Barlow|ofl/barlow/Barlow-Regular.ttf|ofl/barlow/Barlow-Bold.ttf
Rajdhani|ofl/rajdhani/Rajdhani-Regular.ttf|ofl/rajdhani/Rajdhani-Bold.ttf
Arvo|ofl/arvo/Arvo-Regular.ttf|ofl/arvo/Arvo-Bold.ttf
TitilliumWeb|ofl/titilliumweb/TitilliumWeb-Regular.ttf|ofl/titilliumweb/TitilliumWeb-Bold.ttf
Anton|ofl/anton/Anton-Regular.ttf|
BebasNeue|ofl/bebasneue/BebasNeue-Regular.ttf|
Lobster|ofl/lobster/Lobster-Regular.ttf|
Pacifico|ofl/pacifico/Pacifico-Regular.ttf|
'

dl() { # url dest
  [ -s "$2" ] && { echo "  skip $(basename "$2")"; return; }
  curl -fsSL -m 60 -o "$2" "$1" && echo "  ok   $(basename "$2")" || { echo "  FAIL $1"; rm -f "$2"; return 1; }
}

fail=0
for line in $ENTRIES; do
  base="${line%%|*}"; rest="${line#*|}"; reg="${rest%%|*}"; bold="${rest##*|}"
  echo "$base:"
  dl "$BASE/$reg" "$DEST/${base}-Regular.ttf" || fail=1
  if [ -n "$bold" ] && [ "$bold" != "$reg" ]; then
    dl "$BASE/$bold" "$DEST/${base}-Bold.ttf" || fail=1
  fi
done
exit $fail
