#!/bin/sh
# graphin SessionStart preheater.
#
# This hook is NOT the installation path — bin/graphin-launch.sh is, and it
# installs synchronously if it has to. This hook exists only to win the race
# most of the time, so a first-run download happens here instead of inside the
# MCP startup timeout.
#
# Therefore: it exits 0 on every path. A failure is reported as prose through
# additionalContext so Claude can tell the user, never as a blocked session.
#
# `compact` is deliberately absent from the matcher — compacting a transcript
# must not trigger a 25MB download.

set -u

ROOT="${CLAUDE_PLUGIN_ROOT:-}"
[ -n "$ROOT" ] || exit 0
DATA="${CLAUDE_PLUGIN_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/graphin}"

# The user supplies their own binary; nothing to install.
if [ -n "${CLAUDE_PLUGIN_OPTION_BINARY_PATH:-}" ] || [ -n "${GRAPHIN_BIN:-}" ]; then
  exit 0
fi

# Same fast path as the launcher: identical manifest means identical install.
if [ -x "$DATA/bin/graphin" ] && cmp -s "$ROOT/install/manifest.json" "$DATA/state/manifest.json"; then
  exit 0
fi

mkdir -p "$DATA/logs" 2> /dev/null || exit 0

if "$ROOT/install/install.sh" >> "$DATA/logs/install.log" 2>&1; then
  exit 0
fi

# Failed. Say why, in one line of valid JSON: escape backslashes, then quotes,
# then flatten newlines.
reason="graphin could not install its server binary."
if [ -r "$DATA/state/last-error.txt" ]; then
  reason="$(sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' "$DATA/state/last-error.txt" | tr '\n\r\t' '   ')"
fi

cat <<EOF
{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"The graphin plugin could not install its server binary, so its MCP tools will be unavailable this session. Reason: ${reason} Tell the user, and suggest running /graphin:doctor. The full log is at ${DATA}/logs/install.log."}}
EOF
exit 0
