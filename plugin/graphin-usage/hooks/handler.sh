#!/bin/sh
# graphin-usage PostToolUse handler (docs/usage-spec.md §2).
#
# Fires on EVERY tool call in EVERY project when installed at user scope, so
# the guard below must be cheap (pure sh builtins, no forks) and the whole
# script must never block the session: every path exits 0 with empty stdout —
# any stdout would be parsed by Claude Code as a hook decision.

# --- 1. Guard: find a graphin-indexed root (spec §2.1) -----------------------
# Marker is .graphin/merkle.json, NOT the .graphin dir: the dir appears the
# moment the server starts; the marker only after the initial scan persisted.
d="${GRAPHIN_USAGE_ROOT:-${CLAUDE_PROJECT_DIR:-}}"
[ -n "$d" ] || exit 0
root=""
i=0
while [ "$i" -le 8 ]; do
  if [ -f "$d/.graphin/merkle.json" ]; then
    root="$d"
    break
  fi
  case "$d" in
    */*) ;;
    *) break ;;
  esac
  nd="${d%/*}"
  [ -n "$nd" ] || nd="/"
  [ "$nd" = "$d" ] && break
  d="$nd"
  i=$((i + 1))
done
[ -n "$root" ] || exit 0

# --- 2. Resolve the graphin binary (spec §2.2) -------------------------------
# MCP configs register graphin by absolute path, so PATH is unreliable; the
# server records its own location in .graphin/binpath at startup.
bin="${GRAPHIN_BIN:-}"
if [ -z "$bin" ] && [ -f "$root/.graphin/binpath" ]; then
  bin="$(cat "$root/.graphin/binpath" 2>/dev/null)"
fi
if [ -z "$bin" ]; then
  bin="$(command -v graphin 2>/dev/null)"
fi
[ -n "$bin" ] && [ -x "$bin" ] || exit 0

# --- 3. Ingest (spec §2.3) ---------------------------------------------------
# stdin (the PostToolUse JSON) passes straight through; ingest re-resolves the
# owning workspace from the event cwd and falls back to GRAPHIN_WS_ROOT.
GRAPHIN_WS_ROOT="$root" "$bin" usage ingest >/dev/null 2>&1
exit 0
