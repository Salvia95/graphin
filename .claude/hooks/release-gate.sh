#!/usr/bin/env bash
# Release gate at the Claude Code level (docs/rag-bench-spec.md §8): a release
# may not be dispatched — by the main session or by the release subagent —
# without a benchmark verdict on the commit being released. Tiers:
#   minor/major bump (0.X)  → full-set pass marker (mode "full")
#   patch bump (0.x.Y)      → smoke marker suffices ("smoke" or "full")
#   any tier                → a waiver minted by `eval-rag.py waive`, whose
#                             path rail already proved the diff cannot move
#                             what the benchmark measures.
# Markers are commit-bound: anything newly committed expires them, and a
# --worktree measurement never qualifies.
set -eu
root=${CLAUDE_PROJECT_DIR:-$PWD}
cmd=$(jq -r '.tool_input.command // empty')
[ -n "$cmd" ] || exit 0

case "$cmd" in
  *"gh workflow run "*release*|*"gh release create"*|*actions/workflows/release*|*api.github.com*dispatches*) ;;
  *) exit 0 ;;
esac

head=$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo "")
deny() {
  jq -n --arg why "$1" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $why}}'
  exit 0
}

# Which tier is being released? Read the version out of the command and
# compare X.Y against the last tag. Unparseable → assume the strict tier.
ver=$(printf '%s' "$cmd" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
prev=$(git -C "$root" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || true)
need="full"
if [ -n "$ver" ] && [ -n "$prev" ] && [ "${ver%.*}" = "${prev%.*}" ]; then
  need="patch"
fi

# A commit-bound waiver clears any tier: its path rail already established
# the diff cannot move the benchmark.
waiver="$root/.graphin/rag-gate-waiver.json"
if [ -f "$waiver" ]; then
  wcommit=$(jq -r '.commit // empty' "$waiver" 2>/dev/null || echo "")
  if [ -n "$wcommit" ] && [ "$wcommit" = "$head" ]; then
    exit 0
  fi
fi

steps="절차 — 풀셋: python3 scripts/eval-rag.py run --out <새 디렉터리> --runs 3 --detach · 스모크(patch 전용): run --out <새 디렉터리> --subset smoke --jobs 3 · 채점: score --out <디렉터리> --gate 0.80 · 성능 무관 변경이면: waive --reason \"…\" (경로 레일이 판단을 검증). --worktree 런은 인정되지 않습니다."

marker="$root/.graphin/rag-gate-pass.json"
[ -f "$marker" ] || deny "릴리스 게이트(§8): 이 커밋($head)에 벤치 통과 기록도 waive 기록도 없습니다. $steps"
mcommit=$(jq -r '.commit // empty' "$marker" 2>/dev/null || echo "")
mcorpus=$(jq -r '.corpus // empty' "$marker" 2>/dev/null || echo "")
mmode=$(jq -r '.mode // "full"' "$marker" 2>/dev/null || echo "")
[ -n "$mcommit" ] || deny "게이트 마커가 손상됐습니다. $steps"
[ "$mcorpus" != "worktree" ] || deny "게이트가 --worktree 코퍼스에서 측정됐습니다 — 릴리스가 배포할 트리가 아닙니다. $steps"
[ "$mcommit" = "$head" ] || deny "게이트 통과 커밋($mcommit)이 현재 HEAD($head)와 다릅니다 — 새 커밋에는 새 측정이 필요합니다. $steps"
case "$mmode" in
  full) exit 0 ;;
  smoke)
    [ "$need" = "patch" ] || deny "스모크 마커로는 patch(0.x.Y) 릴리스만 열립니다 — ${ver:-이} 릴리스는 minor 이상이라 풀셋(19태스크 × 3런) 통과가 필요합니다. $steps"
    exit 0 ;;
  *) deny "부분 측정 마커는 어떤 릴리스도 열지 않습니다. $steps" ;;
esac
