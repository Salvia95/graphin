# shellcheck shell=sh
# Shared resolution for graphin's hook handlers.
#
# Sourced, never executed, so there is no shebang — a shebang here would claim
# otherwise, and the directive above is what tells shellcheck which dialect to
# check this as.
#
# Every handler has to answer the same two questions before it can do anything
# — which workspace is this, and where is the binary — and they have to answer
# them identically. Three copies of the binary-resolution order would drift,
# and the order is not arbitrary (see graphin_bin).
#
# Pure sh builtins, no forks: this runs on every tool call in every project.

# graphin_root echoes an indexed workspace root, or nothing.
#
# The marker is .graphin/merkle.json, NOT the .graphin directory: the directory
# appears the moment a server starts, the marker only after the initial scan
# persisted.
graphin_root() {
  _d="${GRAPHIN_USAGE_ROOT:-${CLAUDE_PROJECT_DIR:-}}"
  [ -n "$_d" ] || return 1

  # workspace_subdir points the server below the project root, and walking
  # upwards can never find it. Server and hooks share the plugin setting.
  if [ -n "${CLAUDE_PLUGIN_OPTION_WORKSPACE_SUBDIR:-}" ] &&
    [ -f "$_d/${CLAUDE_PLUGIN_OPTION_WORKSPACE_SUBDIR}/.graphin/merkle.json" ]; then
    _d="$_d/${CLAUDE_PLUGIN_OPTION_WORKSPACE_SUBDIR}"
  fi

  _i=0
  while [ "$_i" -le 8 ]; do
    if [ -f "$_d/.graphin/merkle.json" ]; then
      printf '%s' "$_d"
      return 0
    fi
    case "$_d" in
      */*) ;;
      *) return 1 ;;
    esac
    _nd="${_d%/*}"
    [ -n "$_nd" ] || _nd="/"
    [ "$_nd" = "$_d" ] && return 1
    _d="$_nd"
    _i=$((_i + 1))
  done
  return 1
}

# graphin_bin echoes an executable graphin, or nothing.
#
# The plugin's symlink comes BEFORE .graphin/binpath, and the order is not
# cosmetic. os.Executable() resolves symlinks on Linux, so a server started
# through $DATA/bin/graphin records the *versioned* file in binpath. The next
# upgrade prunes that file, binpath dangles, the handler dies silently.
# Preferring the symlink self-heals across upgrades.
graphin_bin() {
  _root="${1:-}"
  _bin="${GRAPHIN_BIN:-}"
  if [ -z "$_bin" ]; then
    _bin="${CLAUDE_PLUGIN_OPTION_BINARY_PATH:-}"
  fi
  if [ -z "$_bin" ]; then
    _data="${CLAUDE_PLUGIN_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/graphin}"
    [ -x "$_data/bin/graphin" ] && _bin="$_data/bin/graphin"
  fi
  if [ -z "$_bin" ] && [ -n "$_root" ] && [ -f "$_root/.graphin/binpath" ]; then
    # Legacy: written by the server itself, and the only route for a manually
    # registered binary that this plugin never installed.
    _bin="$(cat "$_root/.graphin/binpath" 2> /dev/null)"
  fi
  if [ -z "$_bin" ]; then
    _bin="$(command -v graphin 2> /dev/null)"
  fi
  [ -n "$_bin" ] && [ -x "$_bin" ] || return 1
  printf '%s' "$_bin"
}
