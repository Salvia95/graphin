// Decoy: prose-heavy lifecycle notes. Every query token except the symbol.

import { Worktree } from "./worktree";

/**
 * The runtime lifecycle: the runtime starts, the session starts, the worktree
 * is attached, the session runs, the session stops, the runtime stops.
 */
export function runtimeLifecycle(worktree: Worktree): string[] {
  return [
    "runtime start",
    "session start",
    "worktree attach",
    "session stop",
    "runtime stop",
  ];
}

/** Restart the runtime session on the same worktree. */
export function restartRuntimeSession(worktree: Worktree): void {
  // runtime restart session restart worktree runtime session start
}
