// Decoy: dense in every prose token of the query (runtime, start, session,
// worktree) and free of the symbol. This is what outranked the definition in
// the 2026-08-07 log.

import { Worktree } from "./worktree";

/** Start the runtime for a session, attaching the session worktree. */
export function startRuntimeSession(worktree: Worktree): void {
  startRuntime();
  startSession(worktree);
}

/** Start the runtime. The runtime must start before any session starts. */
export function startRuntime(): void {
  // runtime start: the runtime session worktree runtime start session
}

/** Start a session on a worktree. A session cannot start without a worktree. */
export function startSession(worktree: Worktree): void {
  // session start worktree runtime session start worktree session
}

/** Start a worktree session for the runtime, then start the runtime session. */
export function startWorktreeSession(worktree: Worktree): void {
  startSession(worktree);
  startRuntimeSession(worktree);
}
