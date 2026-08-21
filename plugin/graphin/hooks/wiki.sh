#!/bin/sh
# graphin knowledge-gate handler. $1 is the verb: gate or mark.
#
# Unlike the usage sink, this one is allowed to block: Claude Code stops a tool
# call when a PreToolUse hook exits 2, and shows that run's stderr to the model.
#
# What is NOT allowed is blocking for our own reasons. This fires on every tool
# call in every project on the machine, so a broken install must never be able
# to stop someone's work. That is why the binary answers a deliberate block
# with its own code (20) instead of 2: 2 is also what it returns for a usage
# error, so passing it through meant an older graphin — one that does not know
# the `wiki` subcommand at all — blocked everything with an instruction nobody
# could follow. Nothing that has not heard of the gate can answer 20.

# shellcheck source-path=SCRIPTDIR
. "$(dirname "$0")/lib.sh"

verb="${1:-}"
[ -n "$verb" ] || exit 0

# Escape hatch. The gate is already opt-in by construction — it arms only where
# a docs/wiki exists — but that protects against unwanted policy, not against a
# bug in the gate itself. Someone whose work is being blocked wrongly needs a
# way out that does not involve deleting their knowledge base.
case "${GRAPHIN_WIKI_GATE:-}" in
  off | 0 | false) exit 0 ;;
esac

root="$(graphin_root)" || exit 0
bin="$(graphin_bin "$root")" || exit 0

# stdout is suppressed on purpose: Claude Code parses a hook's stdout as a
# decision document, and these handlers speak through the exit code and stderr.
GRAPHIN_WS_ROOT="$root" "$bin" wiki "$verb" > /dev/null
rc=$?

# Only a deliberate block reaches the hook runner. Everything else — a usage
# error, a panic, a binary from another version — means we could not ask, and
# not asking allows.
case "$rc" in
  20) exit 2 ;;
  *) exit 0 ;;
esac
