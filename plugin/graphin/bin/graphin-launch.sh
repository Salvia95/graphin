#!/bin/sh
# graphin MCP launcher.
#
# The plugin's .mcp.json carries no `args` — only env. This script is what
# turns that env into a command line, which is the whole point: an unset user
# option renders as an empty string, and a static JSON config can neither drop
# the flag that would carry it nor express a bare boolean like --offline.
#
# Two rules, both of which break the MCP protocol when violated:
#
#   1. exec into the server. The transport is raw stdio on os.Stdin/os.Stdout;
#      any wrapper left in the middle that buffers or reformats corrupts it.
#   2. never write to stdout. Diagnostics go to stderr, always.
#
# This script — not the SessionStart hook — is the authority on the binary
# existing. The order in which Claude Code spawns MCP servers and runs
# SessionStart hooks is not specified, so whichever wins, the server starts.

set -eu

ROOT="${CLAUDE_PLUGIN_ROOT:?graphin: CLAUDE_PLUGIN_ROOT is unset}"
DATA="${CLAUDE_PLUGIN_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/graphin}"

# An unset user_config option renders as an empty string. The '${' case cannot
# happen for a declared option, but costs one comparison as insurance against
# a key that is referenced in .mcp.json and missing from the schema.
val() {
  case "$1" in
    "" | \$\{*) return 1 ;;
  esac
  printf '%s' "$1"
}

# ── 1. Resolve the binary ────────────────────────────────────────────────────
BIN="$(val "${GRAPHIN_BINARY_PATH:-}" || true)"
if [ -n "$BIN" ]; then
  if [ ! -x "$BIN" ]; then
    echo "graphin: binary_path is not an executable file: $BIN" >&2
    echo "graphin:   fix it in /plugin → Manage → graphin → Configure, or clear it to install a release" >&2
    exit 1
  fi
else
  BIN="$DATA/bin/graphin"
  # Version identity is the manifest's file identity: byte-compare the
  # plugin's manifest against the installed copy. Two stats and a 1KB compare,
  # no fork — cheap enough to run on every server start.
  if [ ! -x "$BIN" ] || ! cmp -s "$ROOT/install/manifest.json" "$DATA/state/manifest.json"; then
    mkdir -p "$DATA/logs"
    if ! GRAPHIN_INSTALL_CALLER=launcher "$ROOT/install/install.sh" >>"$DATA/logs/install.log" 2>&1; then
      if [ -r "$DATA/state/last-error.txt" ]; then
        cat "$DATA/state/last-error.txt" >&2
      else
        echo "graphin: install failed; see $DATA/logs/install.log" >&2
      fi
      exit 1
    fi
  fi
fi

# ── 2. Resolve the workspace ─────────────────────────────────────────────────
WS="${GRAPHIN_PROJECT_DIR:-$PWD}"
sub="$(val "${GRAPHIN_WORKSPACE_SUBDIR:-}" || true)"
if [ -n "$sub" ]; then
  WS="$WS/$sub"
fi

# ── 3. Assemble argv ─────────────────────────────────────────────────────────
set -- --workspace "$WS"

v="$(val "${GRAPHIN_MODEL_TYPE:-}" || true)"
if [ -n "$v" ]; then
  set -- "$@" --model-type "$v"
fi

v="$(val "${GRAPHIN_MODEL_DIR:-}" || true)"
if [ -n "$v" ]; then
  set -- "$@" --model-dir "$v"
fi

v="$(val "${GRAPHIN_SEMANTIC_MAX_NODES:-}" || true)"
if [ -n "$v" ]; then
  set -- "$@" --semantic-max-nodes "$v"
fi

# A boolean option renders as the string "true"/"false"; anything truthy a
# human might type by hand is accepted too.
case "$(printf '%s' "${GRAPHIN_OFFLINE:-}" | tr '[:upper:]' '[:lower:]')" in
  1 | true | yes | on) set -- "$@" --offline ;;
esac

exec "$BIN" "$@"
