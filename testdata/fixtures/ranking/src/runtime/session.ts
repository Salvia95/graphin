// Session bookkeeping. The symbol under test lives here and nowhere else.

import { Worktree } from "./worktree";

export interface SessionState {
  id: string;
  worktree: Worktree;
}

/** Resolve the git ref the worktree currently has checked out. */
export function headRef(worktree: Worktree): string {
  return worktree.head ?? "HEAD";
}

export function sessionState(id: string, worktree: Worktree): SessionState {
  return { id, worktree };
}
