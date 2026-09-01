#!/usr/bin/env bash
# rag-bench change control (docs/rag-bench-spec.md §8): the rubric and the
# golden set move only with the owner's explicit approval. This hook denies
# Edit/Write/NotebookEdit on the guarded paths unless the owner has placed
# .rag-bench-unlock at the repo root (to be removed again when the work is
# done). It deliberately does not guard Bash — content drift through any
# route is what rubric.lock catches in CI.
set -eu
root=${CLAUDE_PROJECT_DIR:-$PWD}
f=$(jq -r '.tool_input.file_path // .tool_input.notebook_path // empty')
[ -n "$f" ] || exit 0
case "$f" in
  /*) rel=${f#"$root"/} ;;
  *) rel=$f ;;
esac
case "$rel" in
  scripts/eval-rag.py|eval/rag/*)
    if [ -e "$root/.rag-bench-unlock" ]; then
      exit 0
    fi
    jq -n '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: "rag 벤치 루브릭/골든셋은 변경 통제 대상입니다(docs/rag-bench-spec.md §8) — 추가·수정·삭제는 소유자의 명시적 허가가 있을 때만 가능합니다. 허가를 받았다면 소유자가 저장소 루트에 .rag-bench-unlock 파일을 만들어야 열립니다. 작업 후 절차: scripts/eval-rag.py lock --approved-by <소유자>, 커밋에 Rag-Bench-Approved-By: 트레일러, unlock 파일 삭제."}}'
    ;;
esac
exit 0
