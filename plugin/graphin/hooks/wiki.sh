#!/bin/sh
# graphin knowledge-gate handler. $1 is the verb: gate or mark.
#
# Unlike the usage sink, this one is allowed to block: `wiki gate` exits 2 to
# stop a tool call, and Claude Code shows that run's stderr to the model. So
# the exit code passes through instead of being swallowed.
#
# What is NOT allowed is blocking for our own reasons. Every failure to reach
# the binary exits 0, because this fires on every tool call in every project
# on the machine and a broken install must not be able to stop someone's work.

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
exit $?
