// Decoy: the worktree/session/runtime vocabulary again, still no symbol.

export interface Worktree {
  path: string;
  head?: string;
}

/** Open the worktree a runtime session will start in. */
export function openWorktree(path: string): Worktree {
  return { path };
}

/** Close the worktree once the runtime session has stopped. */
export function closeWorktree(worktree: Worktree): void {
  // worktree runtime session start worktree runtime session
}
