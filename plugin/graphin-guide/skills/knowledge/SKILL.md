---
name: knowledge
description: >-
  Build and use knowledge sets — curated lists of documentation sections that an
  agent can scan by one-line summaries and then load exactly, instead of reading
  whole documents. Use when an agent needs background it should not carry in its
  prompt (a release process, an instrumentation design, a subsystem's rules),
  when asked to make such a set for a topic, or when a task says to consult one.
  Requires the graphin MCP server, which turns every markdown heading into a
  section node.
---

# Knowledge sets

A knowledge set is **one markdown file listing sections of other documents**,
each with a one-line summary. An agent reads the summaries, decides what it
actually needs, and loads only that.

This exists because the alternative is bad in both directions: putting the
documentation in an agent's prompt costs tokens on every session and goes stale,
while telling the agent to "read the docs" costs a whole file to answer one
question.

**Why sections and not files:** graphin indexes every markdown heading as its
own node (`docs/spec.md#some-heading`), so a set can point at a paragraph of a
50KB document and load just that.

## The format

Sets live in `docs/knowledge/<name>.md`. **The filename is the set's name** —
there is no second place that declares it.

```markdown
# Release

What you need when cutting a release: picking the version digit, deciding
whether the notes need a prelude, and why the workflow looks the way it does.

## Picking the version

- [§13.3 The 0.x rule](../plugin-distribution.md#133-the-0x-rule) —
  In 0.x, minor means "the user has to fix something" and everything else is a
  patch. The size of the change is irrelevant.
- [§13.2 What actually breaks](../plugin-distribution.md#132-what-actually-breaks) —
  Only five surfaces break: tool names, option keys, CLI flags, the minimum
  Claude Code version, and the DB snapshot contract.

## Why the workflow is like that

- [§5.2 The release workflow](../plugin-distribution.md#52-the-release-workflow) —
  It is dispatch-driven, not tag-driven, because the manifest must carry hashes
  of assets that do not exist until the build runs.
```

Rules that matter:

- **One entry is one line**: `- [title](relative/path.md#anchor) — summary`.
- **The node id is the link target resolved against the set file's directory.**
  `docs/knowledge/release.md` + `../plugin-distribution.md#133-the-0x-rule`
  → `docs/plugin-distribution.md#133-the-0x-rule`. That resolved string is what
  you pass to `read_code`.
- **The anchor is a GitHub heading anchor**, which is also exactly graphin's node
  id. Lowercase; drop everything that is not a letter, digit, `_` or `-`; turn
  each remaining space into one hyphen. **Consecutive hyphens are not collapsed
  and the ends are not trimmed** — a dropped character leaves nothing behind,
  but the spaces around it each still become a hyphen.
- **Group with `##` headings**, and keep a group small enough that its entries
  fit one `read_code` call. Groups are not decoration: the set file is itself
  markdown, so each group is a section node and an agent can read one group
  instead of the whole set.

## Using a set

1. Read the set — or one group of it, which is its own node:
   `read_code("docs/knowledge/release.md#picking-the-version")`.
2. Choose from the summaries. Skipping is the point; loading everything defeats
   the set.
3. Resolve the chosen links to node ids and fetch them in one call:
   `read_code(node_ids=[…])`.
4. If the response lists `<omitted reason="budget">`, request those in a second
   call. It never cuts inside a section, so what you got is complete.

Do not re-read a document you already loaded a section of, and do not read the
whole file "for context" — the set exists to make that unnecessary.

## Building a set

You are the part of this that graphin cannot do: graphin splits and retrieves
deterministically, but it has no generative model, so **the summaries are yours
to write**.

1. Find the material. `search_hybrid(topic, target="docs")` — the filter keeps
   code out of a search whose answer is always a document — then `explore_graph`
   on any document node to see its sections (`contains` edges).
2. **Read every section you are about to list.** A summary written from a
   heading is worthless — the heading is already in the link text.
3. Write one line per entry that says **what the section claims, not what it is
   about**. "Versioning rules" tells a reader nothing; "in 0.x, minor means the
   user has to fix something" lets them decide in one glance. That decision is
   the only job the summary has.
4. Group related entries under `##`, and split a group that grew past one
   `read_code` call.
5. Leave out what a reader will not need. A set that lists everything is a table
   of contents, and they already have one.

Overlap between sets is fine and expected — the same section can matter to
several audiences. What must not happen is copying a section's *content* into a
set; then there are two versions of it and they drift.

## Keeping sets honest

Entries point at anchors, and **renaming a heading silently breaks them**. Check
them in CI with a text-only script (no graphin needed) that re-derives each
target file's anchors and compares. Without that guard, a set rots invisibly.

A summary can also go stale while its anchor still resolves — the section was
rewritten and the one-liner no longer matches. Nothing catches that
automatically, so re-read the sections a set names whenever you touch it.
