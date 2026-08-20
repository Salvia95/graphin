#!/bin/sh
# graphin PostToolUse handler (docs/usage-spec.md §2).
#
# Fires on EVERY tool call in EVERY project when installed at user scope, so
# the guard must be cheap and the script must never block the session: every
# path exits 0 with empty stdout — any stdout would be parsed by Claude Code
# as a hook decision.

# shellcheck source-path=SCRIPTDIR
. "$(dirname "$0")/lib.sh"

root="$(graphin_root)" || exit 0
bin="$(graphin_bin "$root")" || exit 0

# stdin (the PostToolUse JSON) passes straight through; ingest re-resolves the
# owning workspace from the event cwd and falls back to GRAPHIN_WS_ROOT.
GRAPHIN_WS_ROOT="$root" "$bin" usage ingest > /dev/null 2>&1
exit 0
