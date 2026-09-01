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

RUBRIC_VERSION = "1.3.0"

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
RUN_COMPAT = ("1.3.0",)

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

# Sentence-level negation vocabulary for the not-here absence check. The
# failure direction to avoid is a fabrication passing, so the list is loose on
# purpose and an answer that neither denies nor fabricates lands in
# "inconclusive", never in "pass".
NEGATIONS = ("no ", "not ", "n't", "never", "nothing", "none", "absent",
             "nowhere", "does not", "isn't", "aren't", "없", "zero", " 0 ")

TRUNCATION_STATED = re.compile(r"budget|truncat|ran out|stopped early|cut off|예산", re.I)

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
SELF_PREFIXES = ("eval/rag/", "eval/golden/", ".claude/skills/", "scripts/eval-")


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


def compose_prompt():
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
    if not os.access(args.bin, os.X_OK):
        raise SystemExit(f"no graphin binary at {args.bin} — run `make build` or pass --bin")

    out = args.out
    if os.path.exists(os.path.join(out, "summary.json")):
        raise SystemExit(f"{out} already holds a finished run — pick a fresh --out "
                         "(partial output must be deleted, finished output must not be reused)")
    os.makedirs(os.path.join(out, "transcripts"), exist_ok=True)

    prompt = compose_prompt()
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
            argv = [args.bin, "--workspace", s, "--offline"]
            if not args.semantic:
                argv += ["--ort-lib", "/nonexistent-ort"]
            p = os.path.join(out, f"mcp-config-{i}.json")
            with open(p, "w") as f:
                json.dump({"mcpServers": {"graphin": {
                    "type": "stdio", "command": argv[0], "args": argv[1:]}}}, f)
            cfgs.append(p)

        # Not --bare: bare never reads OAuth, and this machine authenticates
        # that way. Isolation is by subtraction instead — hooks (the wiki gate
        # would block the very tools under test) and plugins off, MCP strictly
        # ours, cwd in the snapshot so no CLAUDE.md or auto-memory resolves.
        settings_path = os.path.join(out, "settings.json")
        with open(settings_path, "w") as f:
            json.dump({"disableAllHooks": True, "enabledPlugins": {}}, f)

        meta = {
            "rubric_version": RUBRIC_VERSION,
            "graphin_commit": commit, "worktree_dirty": dirty, "corpus": origin,
            "model": args.model, "cli_version": cli_ver,
            "semantic": args.semantic, "runs": args.runs,
            "agent_sha": sha256_file(AGENT_MD), "skill_sha": sha256_file(SKILL_MD),
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
                       "--settings", settings_path, "--strict-mcp-config",
                       "--mcp-config", cfgs[wi],
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
         "search_runs_max": 0, "escaped": escapes_of(ledger)}
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
    return m


def grade(task, exp, final_text, metrics, files, seen=""):
    budget = task.get("budget_bytes", DEFAULT_BUDGET)
    g = {"end_state": exp["end_state"], "budget": budget,
         "bytes": metrics["bytes_total"]}

    cited = set()
    for m in PATH_RE.finditer(final_text):
        if EXAMPLE_CUE.search(final_text[max(0, m.start() - 60):m.start()]):
            continue
        cited.add(m.group(1))
    # Fabrication means citing a path the agent never saw. A path that is not
    # in the snapshot but DID appear in a tool result is the agent quoting
    # retrieved text — a calibration run failed a correct refusal for
    # attributing a fixture-internal fictional path to the e2e prose it
    # actually lives in.
    fake = sorted(p for p in cited if p not in files and p not in seen)
    g["fake_citations"] = fake
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
        forb = [w for w in exp.get("forbidden", [])
                if any(w.lower() in s and not any(n in s for n in NEGATIONS)
                       for s in sentences)]
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
                                      "search_runs_max", "by_tool")})
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

    lines = [f"# eval-rag — rubric {RUBRIC_VERSION}",
             "",
             f"corpus {meta['corpus']} · graphin {meta['graphin_commit']}"
             + (" (dirty)" if meta.get("worktree_dirty") else "")
             + f" · model {meta['model']} · runs {meta['runs']}"
             + f" · {'hybrid' if meta['semantic'] else 'lexical-only'}",
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
    lines.append(f"- fake citations: {len(fakes)} run(s)"
                 + (f" — {[(r['task'], r['fake_citations']) for r in fakes]}" if fakes else ""))
    if selfs:
        lines.append(f"- self-tooling citations (reported, not filtered): "
                     f"{[(r['task'], r['self_citations']) for r in selfs]}")
    hs = sum(r.get("hints_seen", 0) for r in rows)
    hf = sum(r.get("hints_followed", 0) for r in rows)
    lines.append(f"- keyword hints seen {hs}, followed by search_keyword next {hf}")
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
