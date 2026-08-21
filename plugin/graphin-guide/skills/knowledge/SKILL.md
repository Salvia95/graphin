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

Sets live in `docs/wiki/sets/<name>.md`. **The filename is the set's name** —
there is no second place that declares it.

```markdown
---
roles: []                 # push targets; empty means pull-only
prerequisites: []         # other set names, pulled in ahead of this one
mode: live                # live | pinned
---

# Release

What you need when cutting a release: picking the version digit, deciding
whether the notes need a prelude, and why the workflow looks the way it does.

## Picking the version

- [§13.3 The 0.x rule](../../plugin-distribution.md#133-the-0x-rule) —
  In 0.x, minor means "the user has to fix something" and everything else is a
  patch. The size of the change is irrelevant.
- [§13.2 What actually breaks](../../plugin-distribution.md#132-what-actually-breaks) —
  Only five surfaces break: tool names, option keys, CLI flags, the minimum
  Claude Code version, and the DB snapshot contract.

## Why the workflow is like that

- [§5.2 The release workflow](../../plugin-distribution.md#52-the-release-workflow) —
  It is dispatch-driven, not tag-driven, because the manifest must carry hashes
  of assets that do not exist until the build runs.
```

Rules that matter:

- **One entry is one line**: `- [title](relative/path.md#anchor) — summary`.
- **The node id is the link target resolved against the set file's directory.**
  `docs/wiki/sets/release.md` + `../../plugin-distribution.md#133-the-0x-rule`
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

1. `wiki_preflight(task, role)` — the catalogue for this work: which sets
   apply, one line each, no bodies. It returns a **token**; include it in the
   delegation prompt if you are handing this work to a subagent.
2. Choose from the summaries. Skipping is the point; loading everything defeats
   the set.
3. `wiki_resolve(sets=[…])` — or `wiki_resolve(node_ids=[…])` for a few
   specific entries once you have read the catalogue.
4. If the response lists `<omitted reason="budget">`, request those in a second
   call. It never cuts inside a section, so what you got is complete.

Read the attributes on each `<section>`; they are not decoration:

- `drift="changed-since-registration"` — the section was rewritten after this
  entry was written, so the one-line summary may no longer describe it. Trust
  the text you were given, not the summary.
- `drift="unpinned"` — nothing was recorded to compare against, so "unchanged"
  is not being claimed.
- `redirected_from="…"` — the heading was renamed. The content is right and the
  set's link is stale; fix the link when you are next in that file.

Do not re-read a document you already loaded a section of, and do not read the
whole file "for context" — the set exists to make that unnecessary.

## Glossary terms arrive with the catalogue

`wiki_preflight` also returns `<term>` elements when the task's own wording
touched a project word. Definitions come **inline**, unlike sets: a definition
is one paragraph, and the failure a glossary prevents is using the wrong word
without noticing — precisely the state in which nobody goes looking for it.

Read `<not_to_be_confused_with>` when it is there. It exists because two words
in this project are close enough that someone already got them wrong.

## Growing it

The wiki is grown from work that wanted knowledge and did not get it. There is
no retroactive sweep, and writing entries nobody asked for is how a glossary
becomes a dictionary.

`graphin wiki queue` shows four things at once, because they are one decision:

- **awaiting review** — candidates already filed.
- **work the wiki had no answer for** — every `wiki_preflight` that matched
  nothing, newest first. This is the list to write from.
- **offered but never opened** — sets that keep appearing in catalogues and are
  never resolved. Demote or delete; they cost every delegation and return
  nothing.
- **served with a stale pin** — entries to re-read and `repin`.

To add a term, call `wiki_propose`. It **files a candidate and never publishes**
— approving is a person moving the file into `docs/wiki/glossary/`, which makes
the review an ordinary diff. Three rules reject before a human ever looks:

- **identifier** — the code index already resolves the word. That is structure;
  `search_hybrid` answers it, and a glossary entry would only drift from it.
- **evidence** — cited in fewer than two different files. One place is one
  author's usage, not a vocabulary the project speaks. Cite node ids.
- **cap** — the glossary is full (30). Displacing an entry is a judgement about
  which knowledge matters more, so it is left to a person.

The test for a set, not a term: **would this knowledge have made the session
shorter?** If the answer needs a paragraph of hedging, it is not a set yet.

## Push versus pull

A set can be tagged `roles:`, and `graphin wiki skills` turns those tags into a
per-role block that is injected whole at the start of every session for that
role. Reserve it for what an agent **cannot notice it is missing** — layering
rules, forbidden patterns, project vocabulary. Everything an agent can tell it
needs stays in the catalogue, where it costs nothing until asked for.

The block is capped, because it is paid for on every session whether or not it
is read. Overflow is reported in the block itself rather than trimmed quietly.

Generated files are owned by the generator: edit `docs/wiki/` and regenerate.
Which agents should declare which block is **reported, not applied** — agent
definitions are yours.

## Lifecycle and trust are separate fields

Frontmatter follows the Open Knowledge Format where the field is flat, so these
files read as an OKF bundle without translation:

- `status: draft | stable | deprecated` — **only** where it is in its life.
  Drafts are not served. Deprecated entries **are**: a reader who arrives with
  the old word needs to be told it is the old word.
- `stale_after: YYYY-MM-DD` — re-read after this date **whether or not anything
  changed**. This is a different question from drift, and both are needed: a
  decision record can be byte-for-byte what it was and describe a world that is
  gone. No hash will ever say so.
- `reviewed:` — a flat list of `actor — date`. Trust is **derived** from it, not
  declared: `human:<id>` gives human-reviewed, anything else machine-confirmed,
  nothing gives unverified. Nobody can assert their own tier.
- `title`, `description`, `tags` — the ordinary labels. `description` beats the
  opening paragraph in catalogues.

`graphin wiki export --okf --out DIR` writes the wiki as an OKF bundle. It
**exports rather than converts**: OKF identity is a file path, and this system
addresses a heading inside a document, which is what lets a set point at one
paragraph of a 50KB file. Pins ride along as extension keys, because OKF has no
content hash and a bundle without them cannot tell whether it still matches its
source.

## The gate

Where a project has a `docs/wiki`, graphin blocks two things: delegating without
a manifest token, and editing before knowledge has been loaded. Both blocks name
the command that clears them, so read the message rather than retrying.

An **empty catalogue is a normal answer** and still returns a token. A project
the wiki has nothing to say about is not a project you are stuck in.

The gate arms only where `docs/wiki` exists, so projects that never adopted this
are untouched. If it is blocking you wrongly, `GRAPHIN_WIKI_GATE=off` in the
environment disables it — that is for a bug in the gate, not for a knowledge set
you would rather not read.

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

Run `graphin wiki check`. It reports three things, and the difference between
them is what you act on:

- **dangling** — the anchor no longer resolves. A renamed heading does this and
  leaves no trace in the set file. Fix the link.
- **drift** — the section still exists but its text changed since the entry was
  admitted, so the summary may now describe something that is gone. Re-read the
  section, confirm or rewrite the sentence, then `graphin wiki repin`.
- **unpinned** — no hash was ever recorded, so drift cannot be detected for that
  entry at all. `graphin wiki repin`.

The hashes live in `docs/wiki/pins.lock`, which is generated and **committed**.
Committing it is not bookkeeping: the runtime data directory is gitignored, so a
lockfile kept there would vanish on clone and every entry would silently re-pin
to whatever the document says now — drift detection that can never fire.

Both verbs work with no index and no running server. A section's hash is BLAKE3
over its source slice, so re-parsing the document reproduces exactly what the
indexer recorded, which is why this runs in CI anywhere.

Never hand-edit `pins.lock`. A hash you typed asserts that you compared the
content, and nothing else in the system can tell that you did not.
