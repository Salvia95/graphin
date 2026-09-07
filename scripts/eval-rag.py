#!/usr/bin/env python3
"""Benchmark the graphin-rag agent loop against eval/rag — this repository is
the corpus, the agent is the real one.

Where eval-recall.py scores one deterministic search call, this harness runs
the actual graphin-rag subagent (plugin/graphin-guide/agents/graphin-rag.md,
with the graphin skill injected exactly as the plugin would) headlessly over a
throwaway snapshot of this tree, captures the full tool transcript, and grades
the run against eval/rag/{tasks,expected}.jsonl. The golden set must be built
WITHOUT graphin (see .claude/skills/rag-golden-set/SKILL.md); this script is
the measuring half of that split.

    scripts/eval-rag.py validate                 # schema + paths + evidence, no LLM
    scripts/eval-rag.py run --out out/rag-1      # all tiers, 1 run each
    scripts/eval-rag.py run --out out/rag-3 --runs 3 --detach
    scripts/eval-rag.py score --out out/rag-3    # refuses partial output

The rubric IS this file: docs/rag-bench-spec.md explains what the axes mean and
why, but every number and rule lives here, stamped into each report as
RUBRIC_VERSION. Reports from different rubric versions must not be compared —
score refuses to aggregate across them and the comparison belongs in a fresh
docs/eval/ entry.

An LLM drives the runs, so scores move between invocations. Judge tasks at the
instance level across --runs repeats; a single aggregate mean has flipped a
conclusion in this repository before (docs/eval/2026-07-25-h1-reverify).
"""

import argparse
import hashlib
import json
import os
import re
import shutil
import statistics
import subprocess
import sys
import tempfile
import threading
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

RUBRIC_VERSION = "1.4.0"

# Runs recorded under these versions were produced by a runner whose behavior
# is identical to the current one, so their transcripts may be re-scored.
# A change that alters what the RUNNER does (prompt content, snapshot corpus,
# tool roster) resets this to just the new version; a scoring-only change
# appends. Reports always carry the scorer's own RUBRIC_VERSION.
# (1.1.0 added --subset/--jobs and gate tiers; 1.2.0 added the db-nav tier —
# a default full sequential run over the same task list is byte-identical
# across all of these, so they stay compatible. A full marker from an older
# taskset correctly degrades to "partial" against the enlarged set.)
# Reset at 1.3.0: the runner now denies the delegation and skill tools, so
# earlier transcripts were produced under a different tool roster and cannot
# be re-scored as if they were comparable.
# 1.3.1 (2026-09-05, docs/keyword-plan.md M2): observation-only — what the
# agent does right after a search_keyword call, how many keyword responses
# came back empty or without a node id, and which retriever it reached for
# first. No verdict changes, so 1.3.0 transcripts re-score as-is.
# 1.3.2 (2026-09-06): the grep control arm (`run --arm grep`). The default
# arm's command line, prompt and roster are byte-identical to 1.3.1, so its
# transcripts stay comparable; grep-arm runs are a different population and
# carry arm="grep" in meta so a report never mixes the two by accident.
# 1.3.3 (2026-09-06, owner-approved verdict change, docs/handoff-2026-09-06.md
# §7-1): two notation rules in grade(). (1) An elided path — one carrying
# "..." or "…" as a segment, like docs/eval/.../scores.json — is the agent
# shortening a directory it saw, not a claim that the path exists; it no
# longer counts as a fake citation and is reported as elided_citations.
# (2) A forbidden literal whose preceding 60 characters on the same line hold
# a negation ("no Redis client library (e.g. `go-redis`)") stands inside the
# denial even though the sentence splitter cut it off at the period in "e.g.".
# Scoring-only: the runner is byte-identical, so 1.3.0–1.3.2 transcripts
# re-score. In 1.3.2 these two shapes failed five B-arm runs and one grep-smoke
# run that were correct refusals (docs/eval/2026-09-06-rescore-1.3.3).
# 1.3.4 (2026-09-06, owner: "재검토할 부분을 다시 개선"): the --semantic arm passes
# --semantic-wait to each per-run server so search_hybrid waits for the model
# instead of answering lexically for the first 8–15s of every run — the D arm
# (docs/eval/2026-09-06-p1-hybrid) was hybrid for only 62% of its hybrid calls.
# The default arm's command line is byte-identical, so it appends; semantic-arm
# runs before this version are a different population (meta.semantic_wait).
# 1.3.5 (2026-09-06, owner-approved verdict change in the same re-examination
# round): TRUNCATION_STATED also recognises an overrun stated as "5 KB over the
# ~20,000-byte target", "overrun", "overage", "exceeded". Once read_code and
# explore_graph reported their cost (D′ arm, self-report median 0.99), agents
# stopped writing "budget" and started writing the number — three D′ runs that
# named their overrun to the byte were scored over_silent. Scoring-only.
# Reset at 1.4.0 (2026-09-07, owner: "스냅샷 절단 규칙을 집어넣어줘"): the snapshot
# now also drops docs/eval/ — the repository's own benchmark records. They
# name every task id, and a not-here task's id contains the very literal it
# probes (rag-nh-redis → "redis"), so the corpus documented the absence it
# was being asked about: in the D′ arm 103 of the not-here tier's search hits
# landed in docs/eval. The golden set cites nothing under docs/eval and the
# wiki sets carry none of those literals, so nothing the tasks need is lost.
# A different corpus is a different runner, so earlier transcripts do not
# re-score as comparable; the next full run is a new baseline.
RUN_COMPAT = ("1.4.0",)

# What a --semantic arm's servers wait for the embedding model. Generous: the
# model loads in 8–15s on this machine, and a call that hits the ceiling just
# proceeds lexically (the response says semantic_ready="false").
SEMANTIC_WAIT = "60s"

# The patch-release smoke subset (docs/rag-bench-spec.md §8): one pulse per
# tier from the tasks that passed 3/3 on the 2026-09-01 baseline, so a smoke
# failure means something moved, not that the die came up wrong. budget-
# pressure is deliberately absent — its tasks carry irreducible variance and
# the full set covers them at every minor release.
SMOKE_TASKS = ("rag-semantic-gate", "rag-lock-steal", "rag-usage-rotation",
               "rag-hop-stem-sides", "rag-nh-rankdef", "rag-oor-adoption")

# Paths whose changes can move what this benchmark measures. `waive` refuses
# to skip the gate when the release diff touches any of these; everything
# else (docs, console UI, usage reporting, wiki, CI, the graphin plugin's
# command docs) is judged skippable. The carve-outs are subsystems that live
# under internal/ but feed no part of the retrieval loop.
SENSITIVE_PREFIXES = ("internal/", "plugin/graphin-guide/", "eval/rag/",
                      "scripts/eval-rag.py", "go.mod", "go.sum")
WAIVE_EXEMPT = ("internal/console/", "internal/usage/", "internal/wiki/")

# Change control (docs/rag-bench-spec.md §8): the rubric and the golden set
# move only with the owner's explicit approval. rubric.lock records the
# approved hashes; `verify-lock` runs in CI and in the release gate, and a
# drifted file fails both until `lock --approved-by` re-records it.
GUARDED = ("scripts/eval-rag.py", "eval/rag/tasks.jsonl", "eval/rag/expected.jsonl")
LOCK_PATH = os.path.join(REPO, "eval/rag/rubric.lock")

TIERS = ("answered", "multi-hop", "not-here", "out-of-reach", "budget-pressure",
         "db-nav")
DEFAULT_BUDGET = 40000

AGENT_MD = os.path.join(REPO, "plugin/graphin-guide/agents/graphin-rag.md")
SKILL_MD = os.path.join(REPO, "plugin/graphin-guide/skills/graphin/SKILL.md")

# In print mode --allowedTools is a PERMISSION allowlist, not a roster: every
# other tool stays available and un-prompted. Measured on the 1.2.x baseline,
# 21 of 65 runs used something outside this list — eight delegated to another
# subagent (whose retrieval never enters this ledger, so the budget axis
# undercounts), three ran the INSTALLED graphin binary against the real
# checkout, which is the corpus this harness excises on purpose. So the roster
# is cut with --disallowedTools below, which does remove the tools.
ALLOWED_TOOLS = [
    "mcp__graphin__bootstrap_workspace",
    "mcp__graphin__search_hybrid",
    "mcp__graphin__search_keyword",
    "mcp__graphin__explore_graph",
    "mcp__graphin__read_code",
    "mcp__graphin__diagnose_index",
    "Read", "Grep", "Glob",
]

# Cut from the roster. Bash/Read/Grep/Glob stay: production graphin-rag only
# disallows the write tools, so withholding a grep fallback would measure a
# different agent than the one that ships. What must go is anything that
# leaves this measurement — delegation (spend and evidence off-ledger), skills
# (the plugin's own report command reads the real workspace), and the network.
DENIED_TOOLS = "Edit,Write,NotebookEdit,Agent,Task,Skill,ScheduleWakeup,WebFetch,WebSearch"

NAV_TOOLS = {"mcp__graphin__search_hybrid", "mcp__graphin__search_keyword",
             "mcp__graphin__explore_graph", "mcp__graphin__read_code"}
SEARCH_TOOLS = {"mcp__graphin__search_hybrid", "mcp__graphin__search_keyword"}

# ---------------------------------------------------------------- grep arm
#
# The control group (v2, docs/eval/2026-09-01-rag-baseline "다음에 볼 것"):
# the same tasks, the same scorer, the same contract — and no graphin. The
# agent gets the host's own Read/Grep/Glob/Bash, a prompt that states the
# three end states, the citation rule and the budget rule in the same words,
# and nothing about any retriever. What the scorer then measures is what
# graphin adds over a competent grep loop, on questions graphin's own golden
# set considers fair.
ARMS = ("graphin", "grep")
GREP_ARM_TOOLS = ["Read", "Grep", "Glob", "Bash"]
GREP_ARM_DENIED = DENIED_TOOLS  # the MCP tools never load: no --mcp-config

GREP_AGENT_PROMPT = """# Role

You answer a question about a codebase by **retrieving just enough evidence and
stopping**. Your caller does not see your tool output — only your final
message. That message is the deliverable: the answer, what it rests on, and
what you did not check.

You are not a search engine wrapper. A search engine returns what matched; you
return what is true, and you are accountable for the difference.

# Tools

You have `Grep`, `Glob`, `Read` and `Bash`. The current working directory is the
whole corpus: everything you may consult is under it, and nothing outside it
counts as evidence. Search with `Grep` (or `grep`/`rg` through `Bash`), narrow
with `Glob`, and open files with `Read`. Prefer a specific pattern — an
identifier, an exact string — over a broad one, and prefer reading the matched
region over reading the whole file.

# The budget is the job

Unless the caller sets one, work to roughly **40,000 bytes** of retrieved
content — the tool results you receive — and treat two thirds of that as the
point where you start closing rather than opening. Nothing keeps that total for
you; keep it yourself.

Spending is not the goal and neither is thrift. An answer that cost 3,000 bytes
and is wrong is worse than one that cost 30,000 and is right. What is
unacceptable is spending the budget and *not saying* the answer is thin.

# The loop

1. **Name the evidence before you search.** Write down, for yourself, what
   would settle the question — a function's body, a caller list, a constant's
   declaration, a schema column.
2. **Search by the shape of what you know.** Exact text you can quote: grep the
   text. A symbol name: grep the identifier. A sentence about behavior: grep the
   words the code itself would use, then read what matched.
3. **Read last, and only what the matches point at.** A whole file is rarely the
   evidence; the region around the match usually is.
4. **Stop** when the evidence you named in step 1 is in hand — not when the
   tools stop returning things. Leftover budget is not waste.

Rephrasing has sharply diminishing returns. If a second pattern returns the
same files, a third will too — change the kind of pattern, not its wording.

# Three states end the search early, and each has its own report

- *Answered.* You have the evidence. Stop and write it up.
- *Not here.* You searched for the thing by name and by the words around it and
  nothing in this corpus spells it. Say that plainly — it is a real answer, and
  a far more useful one than five plausible near-misses.
- *Out of reach.* The evidence needs something the corpus does not hold: runtime
  values, execution order, a live database, logs, another repository. Say what
  is missing and what you would need to answer it.

# Check these against your draft before you report

- **No match is not proof of absence.** Say what you searched for, in which
  form, before concluding that something is not here.
- **Don't report a test or a document as the implementation.** If the best
  match is a test whose name restates the question, or a design note about
  the code, say the implementation was not found rather than citing it as one.
- **DB answers are snapshot-scoped.** Schema files in the corpus describe the
  committed schema, not a live database. Say "as committed", never "in
  production".

# What you must not do

- **Do not answer from what you already know.** Recognition tells you where to
  look, and nothing more. Retrieve the lines that show it and cite them, or say
  you did not verify it. An answer with no citation is not this agent's output.
- **Do not leave the working directory.** Absolute paths and `~` are outside
  the corpus and are not evidence.
- **Do not silently truncate.** If you stopped because the budget ran out, the
  report says so.

# Report

Structure the final message as:

1. **The answer**, in prose, first.
2. **What it rests on** — repository-relative `path:line` per claim.
3. **What you did not verify** — the file you did not open, the pattern you did
   not try, the call the text cannot show. This section is not optional.
4. **Cost** — roughly how many bytes of tool output you received and how many
   calls it took.

A pile of file paths is not an answer. Neither is a summary with no citations.
"""

# The grep arm is confined to the snapshot by a PreToolUse hook, in addition
# to the post-hoc `escaped` verdict both arms get. The graphin arm left the
# snapshot 0 times in 81 runs, so a hook there would be inert; a grep loop
# with the host's shell did it 33 times in 36 runs when the scaling bench
# measured it, and one such run reads the answer out of the plugin cache's
# copy of this repository. Denied calls are counted by the scorer from the
# transcript (`contained`), so the pressure to leave is still visible.
CONTAINMENT_HOOK = r'''#!/usr/bin/env bash
set -eu
cwd=$(pwd)
p=$(jq -r '[.tool_input.command, .tool_input.file_path, .tool_input.path, .tool_input.pattern] | map(select(. != null)) | join(" ")')
[ -n "$p" ] || exit 0
bad=""
for tok in $p; do
  case "$tok" in
    /*|~/*)
      abs=${tok/#\~/$HOME}
      case "$abs" in "$cwd"/*|"$cwd") ;; *) bad="$tok"; break ;; esac ;;
  esac
done
[ -n "$bad" ] || exit 0
jq -n --arg p "$bad" '{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",permissionDecisionReason:("Path outside the workspace under test: " + $p + ". Everything you may consult is under the current working directory; search there with relative paths.")}}'
'''


def write_containment_hook(out):
    p = os.path.join(out, "contain.sh")
    with open(p, "w") as f:
        f.write(CONTAINMENT_HOOK)
    os.chmod(p, 0o755)
    return p

# Sentence-level negation vocabulary for the not-here absence check. The
# failure direction to avoid is a fabrication passing, so the list is loose on
# purpose and an answer that neither denies nor fabricates lands in
# "inconclusive", never in "pass".
NEGATIONS = ("no ", "not ", "n't", "never", "nothing", "none", "absent",
             "nowhere", "does not", "isn't", "aren't", "없", "zero", " 0 ")

# 1.3.3: the sentence rule above splits on periods, so "e.g." and "go.mod"
# cut a denial in two and strand the forbidden literal in a fragment with no
# negation. This looks back this many characters from the literal across
# those periods — but never across a line break: a list item or a paragraph
# is a real boundary, and the failure direction to avoid is still a
# fabrication passing on a negation two sentences up.
NEGATION_WINDOW = 60


def negated_before(low, pos):
    start = max(0, pos - NEGATION_WINDOW, low.rfind("\n", 0, pos) + 1)
    return any(n in low[start:pos] for n in NEGATIONS)

TRUNCATION_STATED = re.compile(
    r"budget|truncat|ran out|stopped early|cut off|예산"
    # 1.3.5: an overrun named by its number or its ceiling is stated, not silent.
    # "over the" alone is not enough ("over the call chain") — it must be
    # followed by a figure or a word for the ceiling.
    r"|exceed|overr[au]n|overage|ran over|over (?:the |its |my )?(?:~?[\d,.]+|target|limit|cap|ceiling)",
    re.I)

# Repo-relative path shapes for the fake-citation check. Only paths under the
# repository's own top-level dirs count — a hallucinated golang.org import is
# not a citation — and only real file extensions: `internal/bench.Terms` is a
# qualified Go symbol, not a path (1.0.0 counted it and failed a correct run).
PATH_RE = re.compile(
    r"\b((?:internal|cmd|docs|plugin|scripts|eval|schema|proto)/[A-Za-z0-9_./-]+"
    r"\.(?:go|md|py|sh|jsonl?|ya?ml|toml|jsx?|tsx?|kt|java|txt|lock|mod|sum|fb|proto))\b")

# A path shown as an illustration ("e.g. docs/foo.md#slug") is not a citation.
EXAMPLE_CUE = re.compile(r"(?:e\.g\.|for example|for instance|example|such as|say|가령|예를 들|예:)\s*[,:]?\s*[`\"'(]*$", re.I)

# The measurement apparatus is cut OUT of the snapshot, not merely reported:
# search_keyword reads files, not the index, so the .jsonl trick does not
# protect it — a not-here task's forbidden literal exists verbatim in
# expected.jsonl (and, one calibration run showed, in this very file's
# comments), which flips "not here" to false. eval-recall keeps its self-files
# and reports contamination; here the files change the answers themselves.
# docs/eval/ is not apparatus but self-reference: the records of earlier
# runs name the tasks, and the not-here tasks' names carry their forbidden
# literals (1.4.0). The product index keeps docs/eval — the wiki sets pin
# sections there — only the measurement corpus drops it.
SELF_PREFIXES = ("eval/rag/", "eval/golden/", ".claude/skills/", "scripts/eval-", "docs/eval/")


def load_jsonl(path):
    rows = []
    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            line = line.strip()
            if not line or line.startswith("//"):
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError as e:
                raise SystemExit(f"{path}:{n}: {e}")
    return rows


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        h.update(f.read())
    return h.hexdigest()


def load_set():
    tasks = load_jsonl(os.path.join(REPO, "eval/rag/tasks.jsonl"))
    expected = load_jsonl(os.path.join(REPO, "eval/rag/expected.jsonl"))
    tids = [t["id"] for t in tasks]
    eids = [e["id"] for e in expected]
    if tids != eids:
        raise SystemExit("tasks.jsonl and expected.jsonl must carry the same ids "
                         f"in the same order — tasks={tids} expected={eids}")
    if len(set(tids)) != len(tids):
        raise SystemExit("duplicate task ids")
    return tasks, {e["id"]: e for e in expected}


# ---------------------------------------------------------------- validate

def validate():
    tasks, expected = load_set()
    errs = []
    for t in tasks:
        e = expected[t["id"]]
        if t.get("tier") not in TIERS:
            errs.append(f"{t['id']}: unknown tier {t.get('tier')!r}")
        es = e.get("end_state")
        if t.get("tier") == "db-nav":
            # db-nav mixes end states by design: its axis is the corpus
            # (schema navigation, edge cases included), not the verdict shape.
            if es not in ("answered", "not-here", "out-of-reach"):
                errs.append(f"{t['id']}: db-nav end_state must be one of "
                            f"answered/not-here/out-of-reach, got {es!r}")
        else:
            want_state = {"not-here": "not-here", "out-of-reach": "out-of-reach"}.get(
                t.get("tier"), "answered")
            if es != want_state:
                errs.append(f"{t['id']}: tier {t.get('tier')} expects end_state "
                            f"{want_state}, got {es!r}")
        if es == "not-here" and not e.get("subject"):
            errs.append(f"{t['id']}: not-here needs a `subject` token for the absence check")
        if es == "answered" and not (e.get("evidence") or e.get("evidence_any")):
            errs.append(f"{t['id']}: an answerable task needs evidence or evidence_any")
        cited_blob = ""
        for rel in e.get("must_cite", []):
            alts = [a for a in rel.split("|")
                    if os.path.isfile(os.path.join(REPO, a))]
            if not alts:
                errs.append(f"{t['id']}: no must_cite alternative exists: {rel}")
            for a in alts:
                with open(os.path.join(REPO, a), encoding="utf-8", errors="replace") as f:
                    cited_blob += f.read()
        # AND-evidence must be a literal someone could only copy from the
        # answer — so it must exist inside the files the answer lives in.
        if e.get("must_cite"):
            for ev in e.get("evidence", []):
                if not any(alt in cited_blob for alt in ev.split("|")):
                    errs.append(f"{t['id']}: evidence {ev!r} not found in any must_cite file")
    if errs:
        print("\n".join(errs))
        return 1
    print(f"ok — {len(tasks)} tasks, ids aligned, paths and evidence exist")
    return 0


# ---------------------------------------------------------------- lock

def lock_cmd(args):
    data = {
        "policy": "Changes to these files require the repository owner's "
                  "explicit approval. Regenerate only via `scripts/eval-rag.py "
                  "lock --approved-by <owner>` after that approval, and carry "
                  "a `Rag-Bench-Approved-By:` trailer on the commit.",
        "approved_by": args.approved_by,
        "date": time.strftime("%Y-%m-%d"),
        "files": {p: sha256_file(os.path.join(REPO, p)) for p in GUARDED},
    }
    with open(LOCK_PATH, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
        f.write("\n")
    print(f"locked {len(GUARDED)} files → {os.path.relpath(LOCK_PATH, REPO)} "
          f"(approved by {args.approved_by})")
    return 0


def verify_lock_cmd():
    if not os.path.exists(LOCK_PATH):
        print(f"no {os.path.relpath(LOCK_PATH, REPO)} — the rubric is unlocked. "
              "Run `scripts/eval-rag.py lock --approved-by <owner>`.")
        return 1
    data = json.load(open(LOCK_PATH, encoding="utf-8"))
    drifted = [p for p in GUARDED
               if data.get("files", {}).get(p) != sha256_file(os.path.join(REPO, p))]
    unlisted = [p for p in GUARDED if p not in data.get("files", {})]
    if drifted or unlisted:
        for p in drifted:
            print(f"DRIFT: {p} does not match rubric.lock")
        for p in unlisted:
            print(f"UNLISTED: {p} is guarded but absent from rubric.lock")
        print("The rag-bench rubric and golden set are change-controlled: get the "
              "owner's explicit approval, then re-record with "
              "`scripts/eval-rag.py lock --approved-by <owner>`.")
        return 1
    print(f"ok — {len(GUARDED)} guarded files match rubric.lock "
          f"(approved by {data.get('approved_by', '?')} on {data.get('date', '?')})")
    return 0


# ---------------------------------------------------------------- run

class MCP:
    """Minimal stdio MCP client, same dual-era probe as eval-recall.py —
    duplicated because a hyphenated filename cannot be imported."""

    def __init__(self, argv):
        self.p = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                  stderr=subprocess.DEVNULL, text=True, bufsize=1)
        self.id = 0
        self.modern = False

    def _send(self, method, params):
        self.id += 1
        if self.modern:
            params = dict(params or {})
            params["_meta"] = {
                "io.modelcontextprotocol/protocolVersion": "2026-07-28",
                "io.modelcontextprotocol/clientCapabilities": {},
            }
        self.p.stdin.write(json.dumps(
            {"jsonrpc": "2.0", "id": self.id, "method": method, "params": params or {}}) + "\n")
        self.p.stdin.flush()
        return self.id

    def call(self, method, params=None, timeout=300):
        want = self._send(method, params)
        end = time.time() + timeout
        while time.time() < end:
            line = self.p.stdout.readline()
            if not line:
                raise SystemExit("graphin exited while we waited for a response")
            msg = json.loads(line)
            if msg.get("id") == want:
                return msg
        raise SystemExit(f"{method}: no response within {timeout}s")

    def handshake(self):
        res = self.call("server/discover")
        if "result" in res:
            self.modern = True
            return
        res = self.call("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}})
        if "error" in res:
            raise SystemExit(f"initialize failed: {res['error']}")

    def tool(self, name, args):
        res = self.call("tools/call", {"name": name, "arguments": args})
        if "error" in res:
            raise SystemExit(f"{name}: protocol error {res['error']}")
        return res["result"]["content"][0]["text"]

    def close(self):
        try:
            self.p.stdin.close()
            self.p.wait(timeout=15)
        except Exception:
            self.p.kill()


def materialize(dest, ref, worktree):
    if worktree:
        out = subprocess.run(["git", "-C", REPO, "ls-files", "-co", "--exclude-standard"],
                             capture_output=True, text=True, check=True).stdout
        rels = [r for r in out.splitlines()
                if not r.startswith(SELF_PREFIXES)
                and os.path.isfile(os.path.join(REPO, r))]
        for rel in rels:
            dst = os.path.join(dest, rel)
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            shutil.copy2(os.path.join(REPO, rel), dst)
        return "worktree", sorted(rels)
    tar = subprocess.run(["git", "-C", REPO, "archive", ref], capture_output=True, check=True)
    subprocess.run(["tar", "-x", "-C", dest], input=tar.stdout, check=True)
    for p in SELF_PREFIXES:
        if p.endswith("/"):
            shutil.rmtree(os.path.join(dest, p), ignore_errors=True)
        else:
            d = os.path.join(dest, os.path.dirname(p))
            if os.path.isdir(d):
                for f in os.listdir(d):
                    if f.startswith(os.path.basename(p)):
                        os.remove(os.path.join(d, f))
    sha = subprocess.run(["git", "-C", REPO, "rev-parse", "--short", ref],
                         capture_output=True, text=True, check=True).stdout.strip()
    out = subprocess.run(["git", "-C", REPO, "ls-tree", "-r", "--name-only", ref],
                         capture_output=True, text=True, check=True).stdout
    return sha, sorted(r for r in out.splitlines() if not r.startswith(SELF_PREFIXES))


def strip_frontmatter(text):
    if text.startswith("---"):
        end = text.find("\n---", 3)
        if end != -1:
            text = text[end + 4:]
    # The agent file opens with an HTML comment addressed to maintainers, not
    # to the model — production injection drops frontmatter, keeps the body.
    text = re.sub(r"\A\s*<!--.*?-->\s*", "", text, count=1, flags=re.S)
    return text.strip() + "\n"


def compose_prompt(arm="graphin"):
    if arm == "grep":
        return GREP_AGENT_PROMPT
    with open(AGENT_MD, encoding="utf-8") as f:
        agent = strip_frontmatter(f.read())
    with open(SKILL_MD, encoding="utf-8") as f:
        skill = strip_frontmatter(f.read())
    return agent + "\n\n" + skill


def _hit_limit(tr_path):
    try:
        with open(tr_path, "rb") as f:
            f.seek(max(0, os.path.getsize(tr_path) - 4096))
            tail = f.read().decode("utf-8", "replace")
    except OSError:
        return False
    return '"api_error_status":429' in tail or "usage limit" in tail \
        or "session limit" in tail


def preindex(binpath, snap, semantic, timeout):
    argv = [binpath, "--workspace", snap, "--offline"]
    if not semantic:
        argv += ["--ort-lib", "/nonexistent-ort"]
    mcp = MCP(argv)
    mcp.handshake()
    mcp.tool("bootstrap_workspace", {})
    end = time.time() + timeout
    while True:
        text = mcp.tool("search_hybrid", {"query": "___warmup___"})
        if semantic:
            if 'code="MODEL_UNAVAILABLE"' in text:
                raise SystemExit("semantic unavailable here — rerun without --semantic")
            if 'semantic_ready="true"' in text:
                diag = mcp.tool("diagnose_index", {})
                if 'drained="true"' in diag and 'pending="0"' in diag:
                    break
        elif 'lexical_ready="true"' in text:
            break
        if time.time() > end:
            raise SystemExit(f"index never became ready within {timeout}s")
        time.sleep(0.2)
    mcp.close()
    return argv


def run(args):
    if args.detach and not os.environ.get("RAG_RUN_CHILD"):
        os.makedirs(args.out, exist_ok=True)
        log = open(os.path.join(args.out, "run.log"), "a")
        argv = [sys.executable, os.path.abspath(__file__)] + \
            [a for a in sys.argv[1:] if a != "--detach"]
        env = dict(os.environ, RAG_RUN_CHILD="1")
        p = subprocess.Popen(["setsid", "nohup"] + argv, stdout=log, stderr=log,
                             env=env, start_new_session=True)
        with open(os.path.join(args.out, "run.pid"), "w") as f:
            f.write(str(p.pid))
        print(f"detached pid {p.pid} → {args.out}/run.log; "
              f"score refuses until {args.out}/summary.json exists")
        return 0

    tasks, expected = load_set()
    if args.subset == "smoke":
        by_id = {t["id"]: t for t in tasks}
        missing = [i for i in SMOKE_TASKS if i not in by_id]
        if missing:
            raise SystemExit(f"smoke subset names unknown tasks: {missing} — "
                             "SMOKE_TASKS and the golden set drifted apart")
        tasks = [by_id[i] for i in SMOKE_TASKS]
    if args.tier != "all":
        want = {t.strip() for t in args.tier.split(",")}
        bad = want - set(TIERS)
        if bad:
            raise SystemExit(f"unknown tier(s) {sorted(bad)} — pick from {TIERS}")
        tasks = [t for t in tasks if t["tier"] in want]
    if args.max_tasks:
        tasks = tasks[:args.max_tasks]
    if not tasks:
        raise SystemExit("no tasks selected")
    if args.arm == "graphin" and not os.access(args.bin, os.X_OK):
        raise SystemExit(f"no graphin binary at {args.bin} — run `make build` or pass --bin")

    out = args.out
    if os.path.exists(os.path.join(out, "summary.json")):
        raise SystemExit(f"{out} already holds a finished run — pick a fresh --out "
                         "(partial output must be deleted, finished output must not be reused)")
    os.makedirs(os.path.join(out, "transcripts"), exist_ok=True)

    prompt = compose_prompt(args.arm)
    prompt_path = os.path.join(out, "system-prompt.md")
    with open(prompt_path, "w", encoding="utf-8") as f:
        f.write(prompt)

    cli_ver = subprocess.run(["claude", "--version"], capture_output=True,
                             text=True).stdout.strip()
    commit = subprocess.run(["git", "-C", REPO, "rev-parse", "--short", "HEAD"],
                            capture_output=True, text=True).stdout.strip()
    dirty = subprocess.run(["git", "-C", REPO, "status", "--porcelain"],
                           capture_output=True, text=True).stdout.strip() != ""

    snap = tempfile.mkdtemp(prefix="graphin-rag-")
    snaps = [snap]
    meta = None
    try:
        origin, files = materialize(snap, args.ref, args.worktree)
        with open(os.path.join(out, "files.txt"), "w") as f:
            f.write("\n".join(files))

        # The grep arm never indexes: the corpus it searches is the bare tree,
        # with no .graphin/ directory for a shell loop to stumble into.
        if args.arm == "graphin":
            print(f"indexing snapshot ({origin}) …", flush=True)
            preindex(args.bin, snap, args.semantic, args.index_timeout)

        # Each worker gets its own copy of the indexed snapshot: two servers
        # on one workspace fight over the lock (and lose half the runs), and
        # copying the built index is far cheaper than re-indexing per worker.
        jobs = max(1, min(args.jobs, len(tasks) * args.runs))
        for i in range(1, jobs):
            s2 = f"{snap}-w{i}"
            shutil.copytree(snap, s2)
            snaps.append(s2)
        cfgs = []
        for i, s in enumerate(snaps):
            if args.arm != "graphin":
                cfgs.append(None)
                continue
            argv = [args.bin, "--workspace", s, "--offline"]
            if not args.semantic:
                argv += ["--ort-lib", "/nonexistent-ort"]
            else:
                argv += ["--semantic-wait", SEMANTIC_WAIT]
            p = os.path.join(out, f"mcp-config-{i}.json")
            with open(p, "w") as f:
                json.dump({"mcpServers": {"graphin": {
                    "type": "stdio", "command": argv[0], "args": argv[1:]}}}, f)
            cfgs.append(p)

        # Not --bare: bare never reads OAuth, and this machine authenticates
        # that way. Isolation is by subtraction instead — hooks (the wiki gate
        # would block the very tools under test) and plugins off, MCP strictly
        # ours, cwd in the snapshot so no CLAUDE.md or auto-memory resolves.
        #
        # The grep arm cannot disable all hooks: containment IS a hook. It
        # keeps plugins off and adds the one hook; the snapshot's own
        # .claude/settings.json hooks are inert for it (they guard edits and
        # release commands, neither of which the arm can issue).
        settings_path = os.path.join(out, "settings.json")
        with open(settings_path, "w") as f:
            if args.arm == "grep":
                hook = write_containment_hook(out)
                json.dump({"enabledPlugins": {}, "hooks": {"PreToolUse": [
                    {"matcher": "Bash|Read|Grep|Glob",
                     "hooks": [{"type": "command", "command": hook}]}]}}, f)
            else:
                json.dump({"disableAllHooks": True, "enabledPlugins": {}}, f)

        if args.arm == "grep":
            agent_sha = hashlib.sha256(GREP_AGENT_PROMPT.encode()).hexdigest()
            skill_sha = "-"
        else:
            agent_sha, skill_sha = sha256_file(AGENT_MD), sha256_file(SKILL_MD)
        meta = {
            "rubric_version": RUBRIC_VERSION,
            "arm": args.arm,
            "graphin_commit": commit, "worktree_dirty": dirty, "corpus": origin,
            "model": args.model, "cli_version": cli_ver,
            "semantic": args.semantic, "runs": args.runs,
            "semantic_wait": SEMANTIC_WAIT if args.semantic else None,
            "agent_sha": agent_sha, "skill_sha": skill_sha,
            "prompt_sha": hashlib.sha256(prompt.encode()).hexdigest(),
            "taskset_sha": hashlib.sha256(
                open(os.path.join(REPO, "eval/rag/tasks.jsonl"), "rb").read() +
                open(os.path.join(REPO, "eval/rag/expected.jsonl"), "rb").read()
            ).hexdigest(),
            "tasks": [t["id"] for t in tasks],
            "subset": args.subset, "jobs": jobs,
            "started": time.strftime("%Y-%m-%dT%H:%M:%S"),
        }
        with open(os.path.join(out, "meta.json"), "w") as f:
            json.dump(meta, f, indent=2)

        # The child must not inherit this session's Claude Code plumbing: a
        # nested run picking up the parent's socket or entrypoint is the kind
        # of contamination nobody sees in the numbers.
        env = {k: v for k, v in os.environ.items()
               if not k.startswith(("CLAUDE_CODE_", "CLAUDECODE"))}

        runs_path = os.path.join(out, "runs.jsonl")
        work = [(r, t) for r in range(args.runs) for t in tasks]
        total = len(work)
        lk = threading.Lock()
        abort = threading.Event()
        state = {"done": 0, "errors": 0}

        def worker(wi):
            while not abort.is_set():
                with lk:
                    if not work:
                        return
                    run_i, t = work.pop(0)
                tag = f"{t['id']}_r{run_i}"
                tr_path = os.path.join(out, "transcripts", tag + ".jsonl")
                # A non-default budget is the caller's to state — the agent
                # cannot honor a number it was never told (1.0.0 graded the
                # budget-pressure tier against an uncommunicated 20KB).
                question = t["question"]
                if t.get("budget_bytes"):
                    question += ("\n\nWork to a retrieved-content budget of "
                                 f"about {t['budget_bytes']} bytes.")
                cmd = ["claude", "-p", question,
                       "--settings", settings_path, "--strict-mcp-config"]
                if args.arm == "grep":
                    # --strict-mcp-config with no --mcp-config: no MCP server
                    # at all, so the graphin tools do not exist for this arm.
                    cmd += ["--system-prompt-file", prompt_path,
                            "--model", args.model,
                            "--output-format", "stream-json", "--verbose",
                            "--max-turns", str(args.max_turns),
                            "--allowedTools", ",".join(GREP_ARM_TOOLS),
                            "--disallowedTools", GREP_ARM_DENIED]
                else:
                    cmd += ["--mcp-config", cfgs[wi],
                            "--system-prompt-file", prompt_path,
                            "--model", args.model,
                            "--output-format", "stream-json", "--verbose",
                            "--max-turns", str(args.max_turns),
                            "--allowedTools", ",".join(ALLOWED_TOOLS),
                            "--disallowedTools", DENIED_TOOLS]
                t0 = time.time()
                err = None
                try:
                    with open(tr_path, "w") as tf:
                        r = subprocess.run(cmd, stdout=tf, stderr=subprocess.PIPE,
                                           text=True, cwd=snaps[wi], env=env,
                                           timeout=args.run_timeout)
                    if r.returncode != 0:
                        err = f"exit {r.returncode}: {r.stderr[-400:]}"
                except subprocess.TimeoutExpired:
                    err = f"timeout {args.run_timeout}s"
                wall = round(time.time() - t0, 1)
                row = {"task": t["id"], "run": run_i, "transcript": tag + ".jsonl",
                       "wall_s": wall, "error": err}
                with lk:
                    with open(runs_path, "a") as f:
                        f.write(json.dumps(row) + "\n")
                    state["done"] += 1
                    state["errors"] += bool(err)
                    print(f"[{state['done']}/{total}] {tag} {wall}s"
                          + (f" ERROR {err}" if err else ""), flush=True)
                # A 429 means every remaining run would fail the same way —
                # one exhausted-limit baseline burned 52 junk runs before this
                # check existed. Abort without summary.json so score refuses
                # the directory; delete it and rerun after the limit resets.
                if err and _hit_limit(tr_path):
                    abort.set()
                    with lk:
                        print("aborting: usage limit exhausted (HTTP 429). "
                              f"Delete {out} and rerun after the limit resets.",
                              flush=True)

        threads = [threading.Thread(target=worker, args=(i,)) for i in range(jobs)]
        for th in threads:
            th.start()
        for th in threads:
            th.join()
        if abort.is_set():
            return 1
    finally:
        for s in snaps:
            shutil.rmtree(s, ignore_errors=True)

    meta["finished"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    meta["errors"] = state["errors"]
    with open(os.path.join(out, "summary.json"), "w") as f:
        json.dump(meta, f, indent=2)
    print(f"done — {state['done']} runs, {state['errors']} errors → {out}")
    return 0


# ---------------------------------------------------------------- score

def parse_transcript(path):
    """One run → final text + the ordered tool ledger."""
    names = {}      # tool_use_id -> name
    inputs = {}     # tool_use_id -> input
    ledger = []     # [{name, input, result_text, bytes}]
    final_text, cost_usd, num_turns = None, None, None
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            if ev.get("type") == "assistant":
                for b in ev.get("message", {}).get("content", []):
                    if b.get("type") == "tool_use":
                        names[b["id"]] = b.get("name", "?")
                        inputs[b["id"]] = b.get("input", {})
            elif ev.get("type") == "user":
                for b in ev.get("message", {}).get("content", []):
                    if b.get("type") != "tool_result":
                        continue
                    c = b.get("content", "")
                    if isinstance(c, list):
                        c = "".join(x.get("text", "") for x in c
                                    if isinstance(x, dict) and x.get("type") == "text")
                    tid = b.get("tool_use_id", "")
                    ledger.append({"name": names.get(tid, "?"),
                                   "input": inputs.get(tid, {}),
                                   "result": c, "bytes": len(str(c).encode())})
            elif ev.get("type") == "result":
                final_text = ev.get("result") or ""
                cost_usd = ev.get("total_cost_usd")
                num_turns = ev.get("num_turns")
    return final_text, ledger, cost_usd, num_turns


# Bash stays in the roster (production has it), so the one escape it still
# affords has to be caught at scoring time: a command naming an absolute path
# outside the snapshot reaches the real checkout — the installed graphin
# binary, the user's .graphin logs, the golden set this harness excises.
ESCAPE_RE = re.compile(
    r"(/home/[^\s\"']*/\.claude/[^\s\"']*|"
    r"/home/[^\s\"']*/projects/graphin(?!-)[^\s\"']*|"
    r"~/\.claude/[^\s\"']*)")


def escapes_of(ledger):
    out = []
    for e in ledger:
        if e["name"] != "Bash":
            continue
        cmd = str(e["input"].get("command", ""))
        out += ESCAPE_RE.findall(cmd)
    return sorted(set(out))[:5]


def behavior_metrics(ledger):
    m = {"calls": len(ledger), "bytes_total": sum(e["bytes"] for e in ledger),
         "by_tool": {}, "nav_calls": 0, "invented_ids": 0,
         "hints_seen": 0, "hints_followed": 0, "read_batches": [],
         "search_runs_max": 0, "escaped": escapes_of(ledger),
         "contained": 0}  # calls the containment hook denied (grep arm)
    seen_text = ""
    consec_search = 0
    hint_pending = False
    for e in ledger:
        n = e["name"]
        bt = m["by_tool"].setdefault(n, {"calls": 0, "bytes": 0})
        bt["calls"] += 1
        bt["bytes"] += e["bytes"]
        if n in NAV_TOOLS:
            m["nav_calls"] += 1
        consec_search = consec_search + 1 if n in SEARCH_TOOLS else 0
        m["search_runs_max"] = max(m["search_runs_max"], consec_search)
        if hint_pending and n in NAV_TOOLS:
            if n == "mcp__graphin__search_keyword":
                m["hints_followed"] += 1
            hint_pending = False
        if n == "mcp__graphin__read_code":
            ids = e["input"].get("node_ids") or \
                ([e["input"]["node_id"]] if e["input"].get("node_id") else [])
            m["read_batches"].append(len(ids))
            for i in ids:
                if i and i not in seen_text:
                    m["invented_ids"] += 1
        if n == "mcp__graphin__explore_graph":
            i = e["input"].get("node_id", "")
            if i and i not in seen_text:
                m["invented_ids"] += 1
        seen_text += str(e["result"])
        if "<hint>" in str(e["result"]) and "search_keyword" in str(e["result"]):
            m["hints_seen"] += 1
            hint_pending = True
        if "Path outside the workspace under test" in str(e["result"]):
            m["contained"] += 1
    m.update(keyword_metrics(ledger))
    return m


# What the agent does right after a search_keyword call, read off the ledger
# (docs/keyword-plan.md §0). The keyword retriever's contract is that a hit
# carries the node id that owns it, so the next move should be the graph or a
# read of that node; a whole-file Read, a shell grep, or another keyword
# search is the agent buying what the response did not give it. Observation
# only — none of this gates.
KEYWORD_TOOL = "mcp__graphin__search_keyword"
KEYWORD_LOOKAHEAD = 3
KEYWORD_NEXT_KINDS = ("keyword", "hybrid", "read_code", "read_code_other",
                      "explore", "Read", "grep", "bash_read", "none")
_GREP_CMD = re.compile(r"\b(grep|rg)\b")
_BASH_READ_CMD = re.compile(r"\b(sed -n|cat|head|tail)\b")
_KW_FILES = re.compile(r'\bfiles="(\d+)"')
_KW_NODE_ID = re.compile(r'<node id="([^"]+)"')


def keyword_next_of(entry, following):
    """Classify the first consuming action in `following` (the ledger entries
    after one keyword call, already capped at KEYWORD_LOOKAHEAD). Tool-schema
    loads and other non-navigation calls are skipped rather than counted."""
    ids = set(_KW_NODE_ID.findall(str(entry["result"])))
    for x in following:
        n, inp = x["name"], x["input"]
        if n == KEYWORD_TOOL:
            return "keyword"
        if n == "mcp__graphin__search_hybrid":
            return "hybrid"
        if n == "mcp__graphin__read_code":
            asked = set(inp.get("node_ids") or
                        ([inp["node_id"]] if inp.get("node_id") else []))
            return "read_code" if asked & ids else "read_code_other"
        if n == "mcp__graphin__explore_graph":
            return "explore"
        if n == "Read":
            return "Read"
        if n == "Grep":
            return "grep"
        if n == "Bash":
            cmd = str(inp.get("command", ""))
            if _GREP_CMD.search(cmd):
                return "grep"
            if _BASH_READ_CMD.search(cmd):
                return "bash_read"
            continue
        # ToolSearch, diagnose, bootstrap, Glob …: not a consuming move.
    return "none"


def keyword_metrics(ledger):
    m = {"keyword_calls": 0, "keyword_empty": 0, "keyword_idless": 0,
         "keyword_next": {k: 0 for k in KEYWORD_NEXT_KINDS},
         "first_retriever": None}
    for i, e in enumerate(ledger):
        n = e["name"]
        if m["first_retriever"] is None and n in SEARCH_TOOLS:
            m["first_retriever"] = "keyword" if n == KEYWORD_TOOL else "hybrid"
        if n != KEYWORD_TOOL:
            continue
        m["keyword_calls"] += 1
        res = str(e["result"])
        files = _KW_FILES.search(res)
        if files and files.group(1) == "0":
            m["keyword_empty"] += 1
        elif not _KW_NODE_ID.search(res):
            # Hits, but none resolved to a node: a package-level constant, a
            # line outside any symbol. The contract has no next move here.
            m["keyword_idless"] += 1
        m["keyword_next"][keyword_next_of(e, ledger[i + 1:i + 1 + KEYWORD_LOOKAHEAD])] += 1
    return m


def grade(task, exp, final_text, metrics, files, seen=""):
    budget = task.get("budget_bytes", DEFAULT_BUDGET)
    g = {"end_state": exp["end_state"], "budget": budget,
         "bytes": metrics["bytes_total"]}

    cited, elided = set(), set()
    for m in PATH_RE.finditer(final_text):
        if EXAMPLE_CUE.search(final_text[max(0, m.start() - 60):m.start()]):
            continue
        # docs/eval/.../scores.json is a directory the agent saw and shortened,
        # not a path it claims exists — 1.3.2 failed five correct refusals as
        # fake citations on this shape. Counted (1.3.3), never graded.
        if "..." in m.group(1) or "\u2026" in m.group(1):
            elided.add(m.group(1))
            continue
        cited.add(m.group(1))
    # Fabrication means citing a path the agent never saw. A path that is not
    # in the snapshot but DID appear in a tool result is the agent quoting
    # retrieved text — a calibration run failed a correct refusal for
    # attributing a fixture-internal fictional path to the e2e prose it
    # actually lives in.
    fake = sorted(p for p in cited if p not in files and p not in seen)
    g["fake_citations"] = fake
    g["elided_citations"] = sorted(elided)
    self_hits = sorted(p for p in cited if p.startswith(SELF_PREFIXES))
    g["self_citations"] = self_hits  # reported, never filtered

    # must_cite entries may carry |-separated alternatives too: a corpus can
    # hold the same truth in more than one source (the RLS state lives in a
    # JSON sidecar AND in a dbssot schema.sql), and pinning one of them fails
    # an answer grounded in the other.
    g["must_cite_hit"] = [p for p in exp.get("must_cite", [])
                          if any(alt in final_text for alt in p.split("|"))]
    g["must_cite_miss"] = [p for p in exp.get("must_cite", [])
                           if not any(alt in final_text for alt in p.split("|"))]
    # An evidence entry may carry |-separated alternatives; matching is
    # case-insensitive AND markdown-blind (a report writing "**not** enabled"
    # still delivered "not enabled" — emphasis is not a fact). Each entry is
    # still AND with the others.
    lower_text = final_text.lower().replace("*", "").replace("`", "")
    g["evidence_miss"] = [e for e in exp.get("evidence", [])
                          if not any(alt.lower() in lower_text for alt in e.split("|"))]
    any_list = exp.get("evidence_any", [])
    g["evidence_any_hit"] = any(e.lower() in lower_text for e in any_list) \
        if any_list else True

    within = metrics["bytes_total"] <= budget
    stated = bool(TRUNCATION_STATED.search(final_text))
    g["budget_state"] = "within" if within else ("over_stated" if stated else "over_silent")

    # Leaving the snapshot invalidates the run whatever it answered: what it
    # measured is not the corpus under test.
    g["escaped"] = metrics.get("escaped", [])

    low = final_text.lower()
    if exp["end_state"] == "not-here":
        subject = exp["subject"].lower()
        sentences = re.split(r"[.\n]", low)
        denies = any(subject in s and any(n in s for n in NEGATIONS) for s in sentences)
        # A forbidden literal quoted inside a denial ("searched for it, zero
        # files contain it") is the correct answer citing its own search, not
        # a fabrication — it only counts when some occurrence stands in a
        # sentence with no negation (1.0.0 failed two exemplary refusals on
        # this). Do not write any task's forbidden literal into this file:
        # a calibration run found one in a comment here via search_keyword.
        # 1.3.3 adds one exemption on top of the sentence rule: an occurrence
        # with a negation in the NEGATION_WINDOW before it on the same line is
        # inside the denial even when the splitter cut the sentence at "e.g.".
        # The pieces iterated here are exactly `sentences`, with offsets.
        forb = []
        for w in exp.get("forbidden", []):
            wl = w.lower()
            for sm in re.finditer(r"[^.\n]+", low):
                s = sm.group(0)
                if wl not in s or any(n in s for n in NEGATIONS):
                    continue
                if any(not negated_before(low, sm.start() + k.start())
                       for k in re.finditer(re.escape(wl), s)):
                    forb.append(w)
                    break
        g["forbidden_hit"] = forb
        if forb or fake:
            g["verdict"] = "fail"
        elif denies:
            g["verdict"] = "pass"
        else:
            g["verdict"] = "inconclusive"
    elif exp["end_state"] == "out-of-reach":
        if fake:
            g["verdict"] = "fail"
        elif g["evidence_any_hit"]:
            g["verdict"] = "pass"
        else:
            g["verdict"] = "inconclusive"
    else:
        correct = (not g["must_cite_miss"] and not g["evidence_miss"]
                   and g["evidence_any_hit"])
        honest = not fake
        budget_ok = g["budget_state"] != "over_silent"
        g["verdict"] = "pass" if (correct and honest and budget_ok) else "fail"
    if g["escaped"]:
        g["verdict"] = "escaped"

    # The agent's own Cost section, checked against what it actually spent.
    m = re.search(r"([\d,]+)\s*(?:bytes|B\b)|(~?\d+(?:\.\d+)?)\s*KB", final_text)
    if m:
        rep = int(m.group(1).replace(",", "")) if m.group(1) else \
            int(float(m.group(2).lstrip("~")) * 1024)
        g["self_report_bytes"] = rep
        g["self_report_ratio"] = round(rep / metrics["bytes_total"], 2) \
            if metrics["bytes_total"] else None
    return g


def score(args):
    out = args.out
    if not os.path.exists(os.path.join(out, "summary.json")):
        raise SystemExit(f"{out} has no summary.json — the run did not finish. "
                         "Partial output is not scored; delete it and rerun.")
    meta = json.load(open(os.path.join(out, "meta.json")))
    if meta["rubric_version"] not in RUN_COMPAT:
        raise SystemExit(f"run was recorded under rubric {meta['rubric_version']}, "
                         f"whose runner behaved differently from {RUBRIC_VERSION} "
                         f"(compatible: {', '.join(RUN_COMPAT)}) — re-run instead of "
                         "comparing across runner eras")
    tasks, expected = load_set()
    tasks = {t["id"]: t for t in tasks}
    files = set(open(os.path.join(out, "files.txt")).read().splitlines())

    rows = []
    for r in load_jsonl(os.path.join(out, "runs.jsonl")):
        row = {"task": r["task"], "run": r["run"], "wall_s": r["wall_s"]}
        if r.get("error"):
            row.update(verdict="error", error=r["error"])
            rows.append(row)
            continue
        final_text, ledger, cost_usd, turns = parse_transcript(
            os.path.join(out, "transcripts", r["transcript"]))
        if final_text is None:
            row.update(verdict="error", error="no result event in transcript")
            rows.append(row)
            continue
        m = behavior_metrics(ledger)
        seen = "".join(str(e["result"]) for e in ledger)
        g = grade(tasks[r["task"]], expected[r["task"]], final_text, m, files, seen)
        row.update(g)
        row.update({k: m[k] for k in ("calls", "nav_calls", "bytes_total",
                                      "invented_ids", "hints_seen", "hints_followed",
                                      "search_runs_max", "by_tool",
                                      "keyword_calls", "keyword_empty", "keyword_idless",
                                      "keyword_next", "first_retriever", "contained")})
        row["read_batch_max"] = max(m["read_batches"], default=0)
        row["cost_usd"] = cost_usd
        row["turns"] = turns
        rows.append(row)

    by_task = {}
    for row in rows:
        by_task.setdefault(row["task"], []).append(row)

    tiers = {}
    for tid, rr in by_task.items():
        tier = tasks[tid]["tier"]
        tiers.setdefault(tier, []).append((tid, rr))

    lines = [f"# eval-rag — rubric {RUBRIC_VERSION} · arm {meta.get('arm', 'graphin')}",
             "",
             f"corpus {meta['corpus']} · graphin {meta['graphin_commit']}"
             + (" (dirty)" if meta.get("worktree_dirty") else "")
             + f" · model {meta['model']} · runs {meta['runs']}"
             + f" · {'hybrid' if meta['semantic'] else 'lexical-only'}"
             + (f" (semantic-wait {meta['semantic_wait']})" if meta.get('semantic_wait') else ""),
             f"agent {meta['agent_sha'][:12]} · skill {meta['skill_sha'][:12]}"
             f" · taskset {meta['taskset_sha'][:12]} · cli {meta['cli_version']}",
             ""]
    agg = {}
    for tier in TIERS:
        if tier not in tiers:
            continue
        lines += [f"## {tier}", "",
                  "| task | pass | verdicts | bytes(med) | calls(med) | budget |",
                  "|---|---|---|---|---|---|"]
        t_pass = t_total = 0
        for tid, rr in sorted(tiers[tier]):
            vs = [x.get("verdict", "error") for x in rr]
            npass = vs.count("pass")
            t_pass += npass
            t_total += len(vs)
            byt = statistics.median([x.get("bytes_total", 0) for x in rr])
            cal = statistics.median([x.get("calls", 0) for x in rr])
            bud = ",".join(sorted({x.get("budget_state", "—") for x in rr}))
            lines.append(f"| {tid} | {npass}/{len(vs)} | {' '.join(vs)} | "
                         f"{byt:.0f} | {cal:.0f} | {bud} |")
        lines += ["", f"tier pass rate: {t_pass}/{t_total}", ""]
        agg[tier] = {"pass": t_pass, "total": t_total}

    esc = [r for r in rows if r.get("escaped")]
    bad = [r for r in rows if r.get("invented_ids")]
    fakes = [r for r in rows if r.get("fake_citations")]
    selfs = [r for r in rows if r.get("self_citations")]
    lines += ["## behavior", ""]
    lines.append(f"- left the snapshot (scored `escaped`, never pass): {len(esc)} run(s)"
                 + (f" — {[(r['task'], r['escaped']) for r in esc]}" if esc else ""))
    lines.append(f"- invented node ids: {len(bad)} run(s)"
                 + (f" — {[r['task'] for r in bad]}" if bad else ""))
    if meta.get("arm") == "grep":
        cont = sum(r.get("contained", 0) for r in rows)
        lines.append(f"- calls the containment hook denied: {cont} in "
                     f"{sum(1 for r in rows if r.get('contained'))} run(s)")
    lines.append(f"- fake citations: {len(fakes)} run(s)"
                 + (f" — {[(r['task'], r['fake_citations']) for r in fakes]}" if fakes else ""))
    elided = [r for r in rows if r.get("elided_citations")]
    if elided:
        lines.append(f"- elided paths (docs/eval/.../x — shortened, not cited; 1.3.3): "
                     f"{len(elided)} run(s) — {[(r['task'], r['elided_citations']) for r in elided]}")
    if selfs:
        lines.append(f"- self-tooling citations (reported, not filtered): "
                     f"{[(r['task'], r['self_citations']) for r in selfs]}")
    hs = sum(r.get("hints_seen", 0) for r in rows)
    hf = sum(r.get("hints_followed", 0) for r in rows)
    lines.append(f"- keyword hints seen {hs}, followed by search_keyword next {hf}")
    fr = {}
    for r in rows:
        fr[r.get("first_retriever") or "none"] = fr.get(r.get("first_retriever") or "none", 0) + 1
    lines.append("- first retriever: " + " · ".join(
        f"{k} {fr.get(k, 0)}" for k in ("hybrid", "keyword", "none")))
    kc = sum(r.get("keyword_calls", 0) for r in rows)
    if kc:
        ke = sum(r.get("keyword_empty", 0) for r in rows)
        ki = sum(r.get("keyword_idless", 0) for r in rows)
        nxt = {k: 0 for k in KEYWORD_NEXT_KINDS}
        for r in rows:
            for k, v in (r.get("keyword_next") or {}).items():
                nxt[k] = nxt.get(k, 0) + v
        lines.append(f"- search_keyword calls {kc}: empty {ke} · hits without a node id {ki}")
        lines.append("- after a keyword call, first consuming move (≤3 calls): "
                     + " · ".join(f"{k} {nxt[k]}" for k in KEYWORD_NEXT_KINDS if nxt[k]))
    sr = [r.get("self_report_ratio") for r in rows if r.get("self_report_ratio")]
    if sr:
        lines.append(f"- cost self-report ratio (reported/actual, median): "
                     f"{statistics.median(sr):.2f} over {len(sr)} run(s)")
    errs = [r for r in rows if r.get("verdict") == "error"]
    if errs:
        lines.append(f"- errored runs: {[(r['task'], r['run']) for r in errs]}")

    report = "\n".join(lines) + "\n"
    with open(os.path.join(out, "report.md"), "w", encoding="utf-8") as f:
        f.write(report)
    with open(os.path.join(out, "report.json"), "w", encoding="utf-8") as f:
        json.dump({"meta": meta, "rubric_version": RUBRIC_VERSION,
                   "tiers": agg, "runs": rows}, f, ensure_ascii=False, indent=2)
    print(report)
    print(f"→ {out}/report.md · {out}/report.json")

    if args.min_pass is not None:
        a = agg.get("answered", {"pass": 0, "total": 0})
        rate = a["pass"] / a["total"] if a["total"] else 0
        if rate < args.min_pass:
            print(f"FAIL: answered pass rate {rate:.0%} < floor {args.min_pass:.0%}")
            return 1

    # The release floor: overall pass / total runs, with error and
    # inconclusive counting against — a gate that cannot measure must not
    # pass. The benchmark is LLM-driven, so a near-miss is re-measured by
    # re-running the gate, not by lowering the number.
    if args.gate is not None:
        npass = sum(1 for r in rows if r.get("verdict") == "pass")
        rate = npass / len(rows) if rows else 0
        print(f"gate: overall {npass}/{len(rows)} = {rate:.1%} (floor {args.gate:.0%})")
        if rate < args.gate:
            print("FAIL: overall pass rate below the release floor")
            return 1
        # A pass mints the release marker the dispatch hook and the release
        # agent look for. It is bound to the measured commit, so it expires
        # the moment anything new is committed — and a --worktree run never
        # counts, because what it measured is not what a release would ship.
        # mode says HOW MUCH was measured: "full" unlocks any release,
        # "smoke" (the pinned patch subset) unlocks patch releases only, and
        # anything else is "partial" and unlocks nothing.
        mset = set(meta["tasks"])
        mode = "full" if mset == set(tasks) else \
            ("smoke" if mset == set(SMOKE_TASKS) else "partial")
        marker = {
            "commit": meta["graphin_commit"], "corpus": meta["corpus"],
            "mode": mode, "rate": round(rate, 4), "floor": args.gate,
            "rubric_version": RUBRIC_VERSION, "taskset_sha": meta["taskset_sha"],
            "scored_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
        }
        mdir = os.path.join(REPO, ".graphin")
        os.makedirs(mdir, exist_ok=True)
        with open(os.path.join(mdir, "rag-gate-pass.json"), "w", encoding="utf-8") as f:
            json.dump(marker, f, indent=2)
        if meta["corpus"] == "worktree":
            print("gate passed, but on a --worktree corpus — the marker will NOT "
                  "unlock a release; re-run against committed HEAD (drop --worktree)")
        elif mode == "partial":
            print("gate passed, but on an ad-hoc task selection — a partial marker "
                  "unlocks nothing; use the full set or --subset smoke")
        else:
            print(f"gate passed on commit {meta['graphin_commit']} ({mode}) — "
                  "release marker written to .graphin/rag-gate-pass.json"
                  + ("" if mode == "full" else " (patch releases only)"))
    return 0


# ---------------------------------------------------------------- waive

def waive_cmd(args):
    """The owner delegated one judgment to the main agent: a release whose
    diff cannot move what this benchmark measures may skip the gate. The
    judgment is recorded, and it is railed — if the diff since the last tag
    touches a measurement-relevant path, this refuses, and the rail is not
    overridable from here (changing it means changing this guarded file)."""
    def git(*a):
        return subprocess.run(["git", "-C", REPO, *a], capture_output=True,
                              text=True).stdout.strip()
    commit = git("rev-parse", "--short", "HEAD")
    prev = git("describe", "--tags", "--abbrev=0")
    if not prev:
        raise SystemExit("no previous tag to diff against — run the benchmark instead")
    changed = [p for p in git("diff", "--name-only", f"{prev}..HEAD").splitlines() if p]
    sensitive = [p for p in changed
                 if p.startswith(SENSITIVE_PREFIXES)
                 and not p.startswith(WAIVE_EXEMPT)]
    if sensitive:
        print("refusing to waive — the release diff touches measurement-relevant paths:")
        for p in sensitive[:20]:
            print(f"  {p}")
        if len(sensitive) > 20:
            print(f"  … and {len(sensitive) - 20} more")
        print("run the benchmark instead (docs/rag-bench-spec.md §8; this rail is "
              "not overridable)")
        return 1
    marker = {
        "type": "waiver", "commit": commit, "prev_tag": prev,
        "reason": args.reason, "by": "main-agent",
        "changed_files": len(changed),
        "dirty_at_waive": git("status", "--porcelain") != "",
        "date": time.strftime("%Y-%m-%dT%H:%M:%S"),
    }
    mdir = os.path.join(REPO, ".graphin")
    os.makedirs(mdir, exist_ok=True)
    with open(os.path.join(mdir, "rag-gate-waiver.json"), "w", encoding="utf-8") as f:
        json.dump(marker, f, indent=2)
    print(f"waived for commit {commit} — {len(changed)} files changed since {prev}, "
          f"none measurement-relevant. Reason: {args.reason}")
    return 0


# ---------------------------------------------------------------- main

def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)

    sub.add_parser("validate", help="schema + path + evidence checks, no LLM")

    rp = sub.add_parser("run", help="execute the agent over every selected task")
    rp.add_argument("--out", required=True, help="output dir (fresh per run)")
    rp.add_argument("--tier", default="all",
                    help=f"comma list from {', '.join(TIERS)} (default: all)")
    rp.add_argument("--subset", choices=["all", "smoke"], default="all",
                    help="smoke = the pinned patch-release subset "
                         f"({len(SMOKE_TASKS)} tasks)")
    rp.add_argument("--arm", choices=list(ARMS), default="graphin",
                    help="graphin (default): the graphin-rag agent with its skill and MCP "
                         "server; grep: the control group — same tasks, same contract, "
                         "host Read/Grep/Glob/Bash only, confined to the snapshot")
    rp.add_argument("--jobs", type=int, default=1,
                    help="parallel workers, each on its own snapshot copy")
    rp.add_argument("--runs", type=int, default=1)
    rp.add_argument("--max-tasks", type=int, default=0)
    rp.add_argument("--bin", default=os.path.join(REPO, "bin/graphin"))
    rp.add_argument("--ref", default="HEAD")
    rp.add_argument("--worktree", action="store_true")
    rp.add_argument("--semantic", action="store_true",
                    help="hybrid index; off by default for reproducibility")
    rp.add_argument("--model", default="sonnet",
                    help="matches the agent frontmatter (default: sonnet)")
    rp.add_argument("--max-turns", type=int, default=40)
    rp.add_argument("--run-timeout", type=int, default=900)
    rp.add_argument("--index-timeout", type=float, default=300.0)
    rp.add_argument("--detach", action="store_true",
                    help="setsid nohup itself; long runs die with the terminal otherwise")

    spp = sub.add_parser("score", help="grade a finished run dir")
    spp.add_argument("--out", required=True)
    spp.add_argument("--min-pass", type=float, default=None,
                     help="exit 1 when the answered tier's pass rate is below this")
    spp.add_argument("--gate", type=float, default=None,
                     help="release floor: exit 1 when overall pass/total falls below "
                          "this (errors and inconclusive count against)")

    lp = sub.add_parser("lock", help="re-record the guarded-file hashes after "
                                     "the owner's explicit approval")
    lp.add_argument("--approved-by", required=True,
                    help="who approved this change — recorded in rubric.lock")

    sub.add_parser("verify-lock", help="fail when a guarded file drifted from "
                                       "rubric.lock (runs in CI and the release gate)")

    wp = sub.add_parser("waive", help="skip the release gate for a commit whose diff "
                                      "touches nothing the benchmark measures — the "
                                      "judgment is recorded and path-railed")
    wp.add_argument("--reason", required=True,
                    help="why this release cannot move the benchmark — recorded "
                         "in the waiver marker")

    args = ap.parse_args()
    if args.cmd == "validate":
        return validate()
    if args.cmd == "run":
        return run(args)
    if args.cmd == "lock":
        return lock_cmd(args)
    if args.cmd == "verify-lock":
        return verify_lock_cmd()
    if args.cmd == "waive":
        return waive_cmd(args)
    return score(args)


if __name__ == "__main__":
    sys.exit(main())
