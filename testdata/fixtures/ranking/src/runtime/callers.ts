// Call sites. In a real repo a symbol is used far more often than it is
// defined, so the definition competes against a crowd of its own callers —
// each of which also carries the query's prose tokens.

import { headRef } from "./session";
import { Worktree } from "./worktree";

export function logHeadRef(worktree: Worktree): void {
  console.log("runtime session worktree headRef", headRef(worktree));
}

export function compareHeadRef(a: Worktree, b: Worktree): boolean {
  return headRef(a) === headRef(b);
}

export function requireHeadRef(worktree: Worktree): string {
  const ref = headRef(worktree);
  if (!ref) throw new Error("runtime session worktree headRef missing");
  return ref;
}

export function startWithHeadRef(worktree: Worktree): string {
  // start the runtime session at the worktree headRef
  return headRef(worktree);
}

export function reportHeadRef(worktree: Worktree): string {
  return `runtime start session worktree headRef ${headRef(worktree)}`;
}
