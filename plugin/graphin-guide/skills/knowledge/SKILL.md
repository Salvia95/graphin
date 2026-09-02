---
name: knowledge
description: >-
  The project's knowledge layer under docs/wiki — knowledge sets (curated lists
  of documentation sections, scanned by one-line summaries and loaded exactly)
  and a glossary, the project's own vocabulary of words it uses precisely. Use
  when an agent needs background it should not carry in its prompt (a release
  process, a subsystem's rules); when asked what this project calls something,
  to define a term, or to add one to the glossary; when building or consulting
  a set; and when a tool call was BLOCKED for missing knowledge — this says
  what to run to clear it. Covers wiki_preflight / wiki_resolve / wiki_propose,
  the `graphin wiki` commands, and the file formats under docs/wiki. Requires
  the graphin MCP server, which turns every markdown heading into a section
  node.
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
aliases: [versioning, gate tier]  # the subject's names in other vocabularies
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
- **Aliases are matched like the set's name** — one hit is enough, no
  threshold — so write the subject's names, not words that appear in it. A
  multi-word alias is a phrase: `gate tier` needs both words in the task, and
  a task that only says "gate" does not pull the set in (a hyphenated alias
  such as `re-score` is a phrase in the same way). They exist because
  preflight matches on labels only: a set whose labels are Korean is invisible
  to an English task unless an alias says what the subject is called there.
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
- `reviewed="false"` — an agent changed the set this section came from and no
  person has checked the change. The text is the document's own; what may be
  off is which sections the set chose and how it summarised them.

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

`graphin wiki queue` shows five things at once, because they are one decision:

- **awaiting review** — candidates already filed.
- **work the wiki had no answer for** — every `wiki_preflight` that matched
  nothing, newest first. This is the list to write from.
- **offered but never opened** — sets that keep appearing in catalogues and are
  never resolved. Demote or delete; they cost every delegation and return
  nothing.
- **served with a stale pin** — entries to re-read and `repin`.
- **agent changes awaiting review** — sets the maintainer agent repaired or
  re-summarised. Read the diff, then set `reviewed: true` in the frontmatter.

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

## Maintaining a set without editing markdown

Three kinds of decay make no new claim — a dangling entry, a drifted summary,
a set nobody opens — and `wiki_edit_set` handles them so nobody types anchors
by hand:

| `op` | When | What it writes |
|---|---|---|
| `repoint` | the entry's section was renamed or moved | the link target (and title, if given) |
| `summarize` | the section was rewritten and the summary no longer holds | the entry's one-line summary, then repins it |
| `confirm` | the section was rewritten but the summary still holds | nothing in the set; repins the entry |
| `describe` | the set is offered and never opened | the set's `description` |

Every write is judged first and refused on **anchor** (no such heading),
**structure** (the target is code — the index already answers it),
**duplicate** (the set already lists it), **summary** (empty) or **format**
(a title with `]`, a node id with a space — text the entry line cannot
carry). Confirming a dangling entry is refused too: there is nothing to have
re-read, so repoint it. Every write that lands marks the set `reviewed: false`
and repins what it touched. It is surgical: one entry's lines change, nothing
else is reformatted, and the review is an ordinary diff.

The `wiki-maintainer` subagent works this loop from `graphin wiki check` and
`graphin wiki queue`. It never writes a set from nothing, and it is exempt
from the knowledge gate because it works on the wiki rather than with it.

## Writing a set at the end of a task

A set is written by the agent that just did the work, at wrap-up, and only
when that work's `wiki_preflight` came back empty. That is the whole trigger.
"Extracting knowledge" from a task nobody asked about writes a set per task,
and a wiki grown that way is a sweep of the documents dressed as curation.

`wiki_write_set` takes the set whole — name, title, one-line description,
aliases, groups of entries — plus `task`: the sentence you gave
`wiki_preflight`, verbatim. It refuses before anything lands when:

- **unasked** — no preflight in this workspace missed for that task;
- **unreachable** — a preflight for that task would not select the set. Name
  it, or alias it, the way the task names the subject; the check runs with the
  set in place, so it is the real matcher's answer;
- **anchor / structure / duplicate / summary** — the same rules as an edit:
  every entry is a documentation heading that exists, listed once, with a
  title and a sentence; and a set whose every section an existing set already
  lists is that set under a new name;
- **cap** — the wiki holds 30 sets. Displacing one is a person's call;
- **format** — an alias or tag with a comma or bracket, a title on two lines.

It also refuses outright in a workspace with no `docs/wiki`: creating that
directory is how a project opts in, and a set is never the thing that does it.

What lands carries `origin: agent` and `reviewed: false`, is pinned, and is
served at once with the flag on every section. There are no roles: pushing a
set into every delegation of a role is a standing decision the wiki's authors
make, not something to decide at the end of one task.

What to put in it: **the sections you actually had to find and read**, and for
each, **what it claims** — "minor means the user has to fix something", not
"the versioning rules". The reader is deciding whether to open it. If the
honest set is one entry, it is not a set yet; say so in your report instead.

## Writing a glossary entry

Approving a candidate means moving its file from `docs/wiki/propose/` to
`docs/wiki/glossary/`. This is what you are moving, and what to write by hand:

```markdown
---
type: glossary
canonical: posting              # the one word the project should use
title: Posting                  # optional; defaults to the filename
description: A unit of published writing.
tags: [editorial]
aliases: [post, 포스트]          # see the substitution test below
# derives_from: post           # a compound inherits its root's definition
not_to_be_confused_with:
  - blog — a posting is a unit, a blog is a medium
scope: [all]                    # roles this reaches; see Push versus pull
evidence:                       # node ids, from at least two different files
  - docs/handbook.md#publishing
  - docs/api.md#post-endpoints
status: stable
reviewed:
  - human:ahormati — 2026-08-21
---

One paragraph. What it means, and why the project needed its own word for it.
```

**The filename is the term's identity.** `canonical` defaults to it, and other
pages link by it.

Three rules decide the hard parts:

- **The substitution test decides `aliases`.** A word is an alias only if it is
  interchangeable in *every* context in this project. Partial overlap is a
  separate entry with the relation stated — merging the two hides the exact
  difference that made someone look the word up.
- **Compounds are not defined twice.** Point `derives_from` at the root and let
  the definition be inherited. Two definitions of overlapping things drift, and
  the one nobody reads is the one that goes wrong.
- **`not_to_be_confused_with` earns its place.** Only write one where two words
  in this project are close enough that somebody already got them wrong. It is
  the field that does the most work per line, and the field that is worthless
  when filled in speculatively.

## Push versus pull

A set can be tagged `roles:`, and `graphin wiki skills` turns those tags into a
per-role block that is injected whole at the start of every session for that
role. Reserve it for what an agent **cannot notice it is missing** — layering
rules, forbidden patterns, project vocabulary. Everything an agent can tell it
needs stays in the catalogue, where it costs nothing until asked for.

The block is capped, because it is paid for on every session whether or not it
is read. Overflow is reported in the block itself rather than trimmed quietly.

### Which agents get which block

Roles reach agents through one file, `docs/wiki/agents.md`:

```markdown
---
type: agents
agents:
  - backend-dev — backend      # this agent's role
  - docs-writer — docs
  - lint-bot — exempt          # or leave the role blank
---
```

The left-hand name is the **subagent type** — what you pass as
`subagent_type` when delegating, which is the `name:` in the agent's own
definition. If those two disagree the row never matches anything.

One table answers two questions on purpose — which role to preflight for, and
whether this agent is subject to the gate at all. Split them into two files and
they start disagreeing about the same agent.

**An agent the table has never heard of is gated, with no role.** The reverse
default would silently exempt every new agent, which is the state the gate
exists to prevent. graphin's own read-only agents (`graphin-rag`,
`release`) are exempt in the binary, because they run in repositories where no
file of yours describes them.

### Wiring the generated block

`graphin wiki skills` writes `.claude/skills/<role>-conventions/SKILL.md`, one
per role. Declaring it is **reported, not applied** — agent definitions are
yours, and a generator that rewrites them makes "regenerate" unsafe to run
without reading the diff first. The command prints the mapping; add the line
yourself:

```yaml
# .claude/agents/backend-dev.md
skills:
  - backend-conventions
```

Generated files are owned by the generator: edit `docs/wiki/` and regenerate.
`graphin wiki skills --check` fails when they have drifted, which is worth a CI
step — a generated block that no longer matches its source still reads as
authoritative.

## Lifecycle and trust are separate fields

Frontmatter borrows field names from the Open Knowledge Format wherever the
field is flat and we wanted it anyway. What is **not** borrowed is OKF's notion
of identity: a concept there is a whole file, while a set here points at one
heading, and that is the thing this system is for. Fields are cheap to share;
identity is not.


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

`graphin wiki export --okf --out DIR` writes a **projection** of the wiki as an
OKF bundle — the source stays here. Each `sources[].resource` names the file,
because that is all an OKF consumer can resolve, and `graphin_node` beside it
carries the heading actually meant. Ignore it and you get the right documents;
use it and you get the right paragraphs. Pins ride along as extension keys,
because OKF has no content hash and a bundle without them cannot tell whether it
still matches its source.

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
