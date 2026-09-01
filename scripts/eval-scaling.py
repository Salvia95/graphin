#!/usr/bin/env python3
"""Does the cost of answering a question grow with the size of the codebase?

The claim under test: graphin's per-question cost is flat in repository size,
while text-search exploration pays more as the tree grows — more matches to
page through, more context spent before the answer, and a worse answer at the
end. This harness holds the QUESTION and its ANSWER fixed and grows only the
haystack, which is the only way to attribute a cost difference to size.

    scripts/eval-scaling.py corpora            # build/index each tier, report sizes
    scripts/eval-scaling.py run --out DIR --tiers t0,t2 --arms graphin,grep
    scripts/eval-scaling.py score --out DIR

A tier is graphin's own tree (where every answer lives, at its usual path) plus
ballast: unrelated real repositories dropped under ballast/. The needle never
moves; only the hay around it. Questions and their expected answers are read
from eval/rag — read-only; this script never writes there, and grades with the
same must_cite / evidence rules so the two harnesses agree on what "right" is.

Two honest caveats belong in every report this produces:

  1. A graphin tool response is capped at 12KB (mcp.MaxResponseBytes), so its
     per-call cost is O(1) BY CONSTRUCTION. The claim this experiment can
     actually test is that the NUMBER of calls does not grow — that a bigger
     haystack does not force more exploration to reach the same needle.
  2. Indexing cost is not query cost. Building the index grows with the tree
     and is measured separately (`corpora`); it is paid once per workspace,
     not once per question, and this experiment says nothing about amortizing
     it.
"""

import argparse
import glob as globmod
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
SANDBOX = "/home/tipa/projects/graphin-eval-sandbox/repos"

# Ballast is real code, not generated filler: a synthetic tree has a term
# distribution nothing like a real repository, and BM25 ranking is exactly what
# would be flattered by that.
#
# The first design used Python science repos (astropy, sympy, scikit-learn) and
# detected nothing — measured, both arms stayed flat to 20x. The reason was not
# volume but SEPARABILITY: the needle is Go infrastructure code and the hay was
# Python, so one `--include=*.go` erased 950k LOC for the grep arm and no
# astropy symbol ever competed for a graphin ranking slot. Size grew; difficulty
# did not.
#
# So the ballast is now Go that implements what graphin implements. bleve is a
# full-text search engine — stemmers for twenty languages, tokenizers, indexes,
# scoring — against a question set that asks when stemming runs and how tokens
# reach an index. bbolt and badger hold page and transaction locks against a
# question about workspace locking; fsnotify is a file watcher. These are the
# competitors a real monorepo has, and they cannot be filtered out by language.
#
# Tiers separate the two mechanisms the first design conflated:
#   t0 -> t2  raises COMPETITION at modest volume
#   t2 -> t3  raises VOLUME with competition already saturated
GO_BALLAST = "/home/tipa/projects/graphin-eval-sandbox/go-ballast"
MODCACHE = os.path.expanduser("~/go/pkg/mod")

TIERS = {
    "t0": [],
    "t1": [(GO_BALLAST, "fsnotify"), (GO_BALLAST, "bbolt"), (GO_BALLAST, "badger")],
    "t2": [(GO_BALLAST, "fsnotify"), (GO_BALLAST, "bbolt"), (GO_BALLAST, "badger"),
           (GO_BALLAST, "bleve")],
    "t3": [(GO_BALLAST, "fsnotify"), (GO_BALLAST, "bbolt"), (GO_BALLAST, "badger"),
           (GO_BALLAST, "bleve"), (MODCACHE, "")],
}

# Only source is copied out of a ballast tree — the module cache is mostly C
# grammar sources and zip archives that no arm would ever read.
BALLAST_EXT = (".go", ".mod")

# Questions whose answers live entirely in graphin's own files, so they stay
# answerable and correctly graded at every tier. Location-shaped and
# content-shaped both, since the two scale differently under text search.
QUESTIONS = ["rag-semantic-gate", "rag-lock-steal", "rag-usage-rotation",
             "rag-md-section-id", "rag-hop-stem-sides", "rag-hint-conditions"]

# `.claude/` goes too, unlike in eval-rag: this harness gives the child its own
# hooks, so the repository's own must not load into it. No question's answer
# lives there.
SELF_PREFIXES = ("eval/", ".claude/", "scripts/eval-")

GRAPHIN_TOOLS = ("mcp__graphin__bootstrap_workspace,mcp__graphin__search_hybrid,"
                 "mcp__graphin__search_keyword,mcp__graphin__explore_graph,"
                 "mcp__graphin__read_code,mcp__graphin__diagnose_index,"
                 "Read,Grep,Glob,Bash")
GREP_TOOLS = "Read,Grep,Glob,Bash"
DENIED = "Edit,Write,NotebookEdit,Agent,Task,Skill,ScheduleWakeup,WebFetch,WebSearch"


def sh(*a, **kw):
    return subprocess.run(a, capture_output=True, text=True, **kw).stdout


def load_jsonl(p):
    return [json.loads(l) for l in open(p, encoding="utf-8") if l.strip()]


def strip_front(t):
    if t.startswith("---"):
        e = t.find("\n---", 3)
        if e != -1:
            t = t[e + 4:]
    return re.sub(r"\A\s*<!--.*?-->\s*", "", t, count=1, flags=re.S).strip()


# ---------------------------------------------------------------- corpora

def build_corpus(tier, dest):
    """graphin at HEAD (answers at their usual paths) + ballast under ballast/."""
    tar = subprocess.run(["git", "-C", REPO, "archive", "HEAD"],
                         capture_output=True, check=True)
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
    for root, name in TIERS[tier]:
        src = os.path.join(root, name) if name else root
        if not os.path.isdir(src):
            raise SystemExit(f"ballast missing: {src}")
        label = name or os.path.basename(root.rstrip("/"))
        copy_sources(src, os.path.join(dest, "ballast", label))
    return corpus_size(dest)


def copy_sources(src, dst):
    """Copy just the source files, and make them writable: the Go module cache
    is mode 0444 and a read-only tree cannot be cleaned up afterwards."""
    for dirpath, dirs, names in os.walk(src):
        dirs[:] = [d for d in dirs if d not in (".git", "testdata", "__pycache__")]
        rel = os.path.relpath(dirpath, src)
        for n in names:
            if not n.endswith(BALLAST_EXT):
                continue
            out = os.path.join(dst, rel, n)
            os.makedirs(os.path.dirname(out), exist_ok=True)
            shutil.copyfile(os.path.join(dirpath, n), out)
            os.chmod(out, 0o644)


def corpus_size(root):
    files = loc = byts = 0
    for dirpath, dirs, names in os.walk(root):
        dirs[:] = [d for d in dirs if d not in (".git", ".graphin", "node_modules",
                                                "__pycache__", "dist", "build")]
        for n in names:
            if not n.endswith((".go", ".py", ".md", ".js", ".ts", ".java", ".kt")):
                continue
            p = os.path.join(dirpath, n)
            try:
                b = open(p, "rb").read()
            except OSError:
                continue
            files += 1
            byts += len(b)
            loc += b.count(b"\n")
    return {"files": files, "loc": loc, "bytes": byts}


class MCP:
    def __init__(self, argv):
        self.p = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                  stderr=subprocess.DEVNULL, text=True, bufsize=1)
        self.id = 0
        self.modern = False

    def call(self, method, params=None):
        self.id += 1
        params = dict(params or {})
        if self.modern:
            params["_meta"] = {"io.modelcontextprotocol/protocolVersion": "2026-07-28",
                               "io.modelcontextprotocol/clientCapabilities": {}}
        self.p.stdin.write(json.dumps({"jsonrpc": "2.0", "id": self.id,
                                       "method": method, "params": params}) + "\n")
        self.p.stdin.flush()
        while True:
            line = self.p.stdout.readline()
            if not line:
                raise SystemExit("graphin exited")
            m = json.loads(line)
            if m.get("id") == self.id:
                return m

    def tool(self, n, a):
        return self.call("tools/call", {"name": n, "arguments": a})["result"]["content"][0]["text"]

    def close(self):
        try:
            self.p.stdin.close()
            self.p.wait(timeout=15)
        except Exception:
            self.p.kill()


def index_corpus(binpath, root, timeout=1800):
    """Index and wait for lexical readiness. Returns seconds and node count."""
    argv = [binpath, "--workspace", root, "--offline", "--ort-lib", "/nonexistent-ort"]
    m = MCP(argv)
    if "result" in m.call("server/discover"):
        m.modern = True
    t0 = time.time()
    m.tool("bootstrap_workspace", {})
    end = time.time() + timeout
    while True:
        if 'lexical_ready="true"' in m.tool("search_hybrid", {"query": "___warmup___"}):
            break
        if time.time() > end:
            raise SystemExit(f"index not ready within {timeout}s")
        time.sleep(1.0)
    secs = round(time.time() - t0, 1)
    diag = m.tool("diagnose_index", {})
    nodes = int((re.search(r'nodes="(\d+)"', diag) or re.search(r'(\d+)', "0")).group(1))
    m.close()
    return secs, nodes, argv


# ---------------------------------------------------------------- run

def compose_prompts():
    agent = strip_front(open(os.path.join(
        REPO, "plugin/graphin-guide/agents/graphin-rag.md")).read())
    skill = strip_front(open(os.path.join(
        REPO, "plugin/graphin-guide/skills/graphin/SKILL.md")).read())
    grep = open(os.path.join(REPO, "eval/scaling/grep-navigator.md")).read()
    return {"graphin": agent + "\n\n" + skill + "\n", "grep": grep}


CONTAINMENT = r'''#!/usr/bin/env bash
# The experiment's independent variable is corpus size, and it only varies if
# the agent is confined to the corpus. Measured without this: the grep arm ran
# `find / -iname "*graphin*"`, found a pristine clone of this repository under
# ~/.claude/plugins/marketplaces/, and read the answer out of it — so it never
# searched the ballasted tree at all and its cost looked flat for a reason that
# had nothing to do with search. Both arms get the same rule, so neither is
# advantaged.
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
        f.write(CONTAINMENT)
    os.chmod(p, 0o755)
    return p


def parse_run(path):
    """Tokens are the headline here, not bytes: input tokens carry the context
    the agent had to hold, which is what 'pollution' costs in practice."""
    names, inputs, ledger = {}, {}, []
    final, usage, turns = None, {}, None
    for line in open(path, encoding="utf-8"):
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
                               "bytes": len(str(c).encode())})
        elif ev.get("type") == "result":
            final = ev.get("result") or ""
            usage = ev.get("modelUsage") or {}
            turns = ev.get("num_turns")
    tin = tout = 0
    for k, v in usage.items():
        if "haiku" in k:   # CLI-internal helper, not the agent's reasoning
            continue
        # Nearly all input is cache reads, so `inputTokens` alone reports ~20
        # for a run that actually carried a quarter-million tokens of context.
        # The sum of the three is what the agent had to hold.
        tin += (v.get("inputTokens", 0) + v.get("cacheReadInputTokens", 0)
                + v.get("cacheCreationInputTokens", 0))
        tout += v.get("outputTokens", 0)
    return final, ledger, tin, tout, turns


def grade(exp, final_text):
    low = final_text.lower().replace("*", "").replace("`", "")
    miss_cite = [p for p in exp.get("must_cite", [])
                 if not any(a in final_text for a in p.split("|"))]
    miss_ev = [e for e in exp.get("evidence", [])
               if not any(a.lower() in low for a in e.split("|"))]
    anyl = exp.get("evidence_any", [])
    ok_any = any(e.lower() in low for e in anyl) if anyl else True
    return {"correct": not miss_cite and not miss_ev and ok_any,
            "must_cite_miss": miss_cite, "evidence_miss": miss_ev,
            "evidence_any_hit": ok_any}


def run(args):
    if args.detach and not os.environ.get("SCALING_CHILD"):
        os.makedirs(args.out, exist_ok=True)
        log = open(os.path.join(args.out, "run.log"), "a")
        argv = [sys.executable, os.path.abspath(__file__)] + \
            [a for a in sys.argv[1:] if a != "--detach"]
        p = subprocess.Popen(["setsid", "nohup"] + argv, stdout=log, stderr=log,
                             env=dict(os.environ, SCALING_CHILD="1"),
                             start_new_session=True)
        print(f"detached pid {p.pid} → {args.out}/run.log")
        return 0

    expected = {r["id"]: r for r in load_jsonl(os.path.join(REPO, "eval/rag/expected.jsonl"))}
    tasks = {t["id"]: t for t in load_jsonl(os.path.join(REPO, "eval/rag/tasks.jsonl"))}
    qids = [q for q in (args.questions.split(",") if args.questions else QUESTIONS)]
    tiers = args.tiers.split(",")
    arms = args.arms.split(",")
    os.makedirs(os.path.join(args.out, "transcripts"), exist_ok=True)
    if os.path.exists(os.path.join(args.out, "summary.json")):
        raise SystemExit(f"{args.out} already holds a finished run — pick a fresh --out")

    prompts = compose_prompts()
    pp = {}
    for k, v in prompts.items():
        pp[k] = os.path.join(args.out, f"prompt-{k}.md")
        open(pp[k], "w").write(v)
    hook = write_containment_hook(args.out)
    st = os.path.join(args.out, "settings.json")
    json.dump({"enabledPlugins": {},
               "hooks": {"PreToolUse": [{"matcher": "Bash|Read|Grep|Glob",
                                         "hooks": [{"type": "command",
                                                    "command": hook,
                                                    "timeout": 10}]}]}},
              open(st, "w"))
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("CLAUDE_CODE_", "CLAUDECODE"))}

    meta = {"tiers": {}, "arms": arms, "questions": qids, "runs": args.runs,
            "graphin_commit": sh("git", "-C", REPO, "rev-parse", "--short", "HEAD").strip(),
            "started": time.strftime("%Y-%m-%dT%H:%M:%S")}
    runs_path = os.path.join(args.out, "runs.jsonl")

    for tier in tiers:
        root = tempfile.mkdtemp(prefix=f"graphin-scale-{tier}-")
        try:
            print(f"[{tier}] building corpus …", flush=True)
            size = build_corpus(tier, root)
            print(f"[{tier}] {size['files']} files, {size['loc']:,} LOC — indexing …",
                  flush=True)
            isecs, nodes, argv = index_corpus(args.bin, root)
            print(f"[{tier}] indexed in {isecs}s, {nodes} nodes", flush=True)
            meta["tiers"][tier] = dict(size, index_s=isecs, nodes=nodes,
                                       ballast=TIERS[tier])
            with open(os.path.join(args.out, "meta.json"), "w") as f:
                json.dump(meta, f, indent=2)

            # One server per worker: two agents on one workspace fight the lock.
            roots = [root]
            jobs = max(1, args.jobs)
            for i in range(1, jobs):
                r2 = f"{root}-w{i}"
                shutil.copytree(root, r2)
                roots.append(r2)
            cfgs = []
            for i, r in enumerate(roots):
                a = [args.bin, "--workspace", r, "--offline", "--ort-lib", "/nonexistent-ort"]
                c = os.path.join(args.out, f"mcp-{tier}-{i}.json")
                json.dump({"mcpServers": {"graphin": {"type": "stdio",
                                                      "command": a[0], "args": a[1:]}}},
                          open(c, "w"))
                cfgs.append(c)

            work = [(arm, q, r) for r in range(args.runs) for arm in arms for q in qids]
            lk, state = threading.Lock(), {"n": 0}
            total = len(work)

            def worker(wi):
                while True:
                    with lk:
                        if not work:
                            return
                        arm, q, ri = work.pop(0)
                    tag = f"{tier}_{arm}_{q}_r{ri}"
                    tp = os.path.join(args.out, "transcripts", tag + ".jsonl")
                    cmd = ["claude", "-p", tasks[q]["question"],
                           "--settings", st, "--model", args.model,
                           "--system-prompt-file", pp[arm],
                           "--output-format", "stream-json", "--verbose",
                           "--max-turns", str(args.max_turns),
                           "--disallowedTools", DENIED]
                    if arm == "graphin":
                        cmd += ["--strict-mcp-config", "--mcp-config", cfgs[wi],
                                "--allowedTools", GRAPHIN_TOOLS]
                    else:
                        cmd += ["--allowedTools", GREP_TOOLS]
                    t0, err = time.time(), None
                    try:
                        with open(tp, "w") as tf:
                            r = subprocess.run(cmd, stdout=tf, stderr=subprocess.PIPE,
                                               text=True, cwd=roots[wi], env=env,
                                               timeout=args.run_timeout)
                        if r.returncode != 0:
                            err = f"exit {r.returncode}: {r.stderr[-300:]}"
                    except subprocess.TimeoutExpired:
                        # Not an error to discard: an arm that cannot answer
                        # within the budget IS the measurement.
                        err = f"timeout {args.run_timeout}s"
                    row = {"tier": tier, "arm": arm, "task": q, "run": ri,
                           "transcript": tag + ".jsonl",
                           "wall_s": round(time.time() - t0, 1), "error": err}
                    with lk:
                        with open(runs_path, "a") as f:
                            f.write(json.dumps(row) + "\n")
                        state["n"] += 1
                        print(f"[{state['n']}/{total}] {tag} {row['wall_s']}s"
                              + (f" {err}" if err else ""), flush=True)

            ths = [threading.Thread(target=worker, args=(i,)) for i in range(jobs)]
            for t in ths:
                t.start()
            for t in ths:
                t.join()
        finally:
            for r in globmod.glob(root + "*"):
                shutil.rmtree(r, ignore_errors=True)

    meta["finished"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    with open(os.path.join(args.out, "summary.json"), "w") as f:
        json.dump(meta, f, indent=2)
    print(f"done → {args.out}")
    return 0


# ---------------------------------------------------------------- score

def score(args):
    out = args.out
    if not os.path.exists(os.path.join(out, "summary.json")):
        raise SystemExit(f"{out} has no summary.json — the run did not finish")
    meta = json.load(open(os.path.join(out, "meta.json")))
    expected = {r["id"]: r for r in load_jsonl(os.path.join(REPO, "eval/rag/expected.jsonl"))}
    rows = []
    for r in load_jsonl(os.path.join(out, "runs.jsonl")):
        row = dict(r)
        if r.get("error") and "timeout" in r["error"]:
            row.update(correct=False, timed_out=True, tokens_in=None,
                       bytes_total=None, calls=None)
            rows.append(row)
            continue
        if r.get("error"):
            row.update(correct=None, errored=True)
            rows.append(row)
            continue
        final, ledger, tin, tout, turns = parse_run(
            os.path.join(out, "transcripts", r["transcript"]))
        if final is None:
            row.update(correct=None, errored=True)
            rows.append(row)
            continue
        row.update(grade(expected[r["task"]], final))
        row.update(tokens_in=tin, tokens_out=tout, turns=turns,
                   calls=len(ledger), bytes_total=sum(e["bytes"] for e in ledger))
        rows.append(row)

    def med(vals):
        vals = [v for v in vals if v is not None]
        return statistics.median(vals) if vals else None

    lines = ["# eval-scaling — 코드베이스가 커질 때 질문 하나의 값",
             "",
             f"graphin {meta['graphin_commit']} · 모델 {meta['arms'] and ''}"
             f"{len(meta['questions'])}질문 × {meta['runs']}런 · 층 {list(meta['tiers'])}",
             ""]
    lines += ["| 층 | 파일 | LOC | 노드 | 인덱싱 |", "|---|---:|---:|---:|---:|"]
    for t, s in meta["tiers"].items():
        lines.append(f"| {t} | {s['files']:,} | {s['loc']:,} | {s['nodes']:,} | {s['index_s']}s |")
    lines += ["", "## 층 × 팔 — 질문 하나당 중앙값", "",
              "| 층 | 팔 | 정답 | 입력토큰 | 리트리벌 | 콜 | 벽시계 | 타임아웃 |",
              "|---|---|---|---:|---:|---:|---:|---:|"]
    agg = {}
    for t in meta["tiers"]:
        for arm in meta["arms"]:
            g = [r for r in rows if r["tier"] == t and r["arm"] == arm]
            if not g:
                continue
            ok = [r for r in g if r.get("correct") is not None]
            npass = sum(1 for r in ok if r.get("correct"))
            to = sum(1 for r in g if r.get("timed_out"))
            a = {"pass": npass, "n": len(ok),
                 "tokens_in": med([r.get("tokens_in") for r in g]),
                 "bytes": med([r.get("bytes_total") for r in g]),
                 "calls": med([r.get("calls") for r in g]),
                 "wall": med([r["wall_s"] for r in g]), "timeouts": to}
            agg[(t, arm)] = a
            lines.append(
                f"| {t} | {arm} | {npass}/{len(ok)} | "
                f"{a['tokens_in']:,.0f} | {a['bytes']:,.0f} | {a['calls']:.0f} | "
                f"{a['wall']:.0f}s | {to} |" if a["tokens_in"] is not None else
                f"| {t} | {arm} | {npass}/{len(ok)} | — | — | — | {a['wall']:.0f}s | {to} |")

    lines += ["", "## 기울기 — 가장 작은 층 대비 배수", "",
              "| 팔 | 지표 | " + " | ".join(meta["tiers"]) + " |",
              "|---|---|" + "---|" * len(meta["tiers"])]
    base_t = list(meta["tiers"])[0]
    for arm in meta["arms"]:
        for key, label in (("tokens_in", "입력토큰"), ("bytes", "리트리벌"),
                           ("calls", "콜")):
            b = agg.get((base_t, arm), {}).get(key)
            if not b:
                continue
            cells = []
            for t in meta["tiers"]:
                v = agg.get((t, arm), {}).get(key)
                cells.append(f"{v / b:.2f}×" if v else "—")
            lines.append(f"| {arm} | {label} | " + " | ".join(cells) + " |")

    # Per question: the aggregate hides the axis that matters. A distinctive
    # identifier is grep's best case (few matches however big the tree); a
    # question whose terms are common in the ballast is where a bigger corpus
    # actually costs more. Both live in this set on purpose.
    last_t = list(meta["tiers"])[-1]
    lines += ["", f"## 질문별 — {base_t} → {last_t} 배수와 정답", "",
              "| 질문 | graphin 토큰 | grep 토큰 | graphin 콜 | grep 콜 | "
              f"정답 {base_t} | 정답 {last_t} |",
              "|---|---:|---:|---:|---:|---|---|"]
    for q in meta["questions"]:
        r_ = {}
        for arm in ("graphin", "grep"):
            for key in ("tokens_in", "calls"):
                a = med([x.get(key) for x in rows
                         if x["task"] == q and x["arm"] == arm and x["tier"] == base_t])
                b = med([x.get(key) for x in rows
                         if x["task"] == q and x["arm"] == arm and x["tier"] == last_t])
                r_[(arm, key)] = f"{b / a:.2f}×" if a and b else "—"
        acc = []
        for t in (base_t, last_t):
            parts = []
            for arm in meta["arms"]:
                g = [x for x in rows if x["task"] == q and x["tier"] == t
                     and x["arm"] == arm and x.get("correct") is not None]
                parts.append(f"{arm[0]}{sum(1 for x in g if x['correct'])}·{len(g)}")
            acc.append(" / ".join(parts))
        lines.append(f"| {q} | {r_[('graphin','tokens_in')]} | {r_[('grep','tokens_in')]} "
                     f"| {r_[('graphin','calls')]} | {r_[('grep','calls')]} "
                     f"| {acc[0]} | {acc[1]} |")
    lines.append("")
    lines.append("정답 칸은 `팔머리글자<맞은 수>·<런 수>` — `g2·2 / g1·2`는 graphin 2/2, grep 1/2.")

    lines += ["", "## 읽을 때 유의할 것", "",
              "- graphin 응답은 12KB로 캡돼 있어 **콜당 비용은 설계상 O(1)**이다. "
              "이 실험이 실제로 검정하는 것은 **콜 수가 늘지 않는가**이다.",
              "- 인덱싱 비용은 질의 비용이 아니다. 위 표의 인덱싱 열은 트리와 함께 "
              "커지며, 워크스페이스당 한 번 낸다.",
              "- 타임아웃은 버리지 않고 센다 — 예산 안에 답하지 못한 것도 결과다."]

    rep = "\n".join(lines) + "\n"
    open(os.path.join(out, "report.md"), "w").write(rep)
    json.dump({"meta": meta, "runs": rows}, open(os.path.join(out, "report.json"), "w"),
              ensure_ascii=False, indent=2)
    print(rep)
    return 0


def corpora(args):
    for tier in TIERS:
        d = tempfile.mkdtemp(prefix=f"graphin-size-{tier}-")
        try:
            s = build_corpus(tier, d)
            print(f"{tier}: {s['files']:,} files  {s['loc']:,} LOC  "
                  f"{s['bytes']/1e6:.1f} MB  ballast={TIERS[tier] or '없음'}")
        finally:
            shutil.rmtree(d, ignore_errors=True)
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("corpora", help="build each tier and print its size")
    rp = sub.add_parser("run")
    rp.add_argument("--out", required=True)
    rp.add_argument("--tiers", default="t0,t1,t2")
    rp.add_argument("--arms", default="graphin,grep")
    rp.add_argument("--questions", default="")
    rp.add_argument("--runs", type=int, default=1)
    rp.add_argument("--jobs", type=int, default=3)
    rp.add_argument("--bin", default=os.path.join(REPO, "bin/graphin"))
    rp.add_argument("--model", default="sonnet")
    rp.add_argument("--max-turns", type=int, default=40)
    rp.add_argument("--run-timeout", type=int, default=600)
    rp.add_argument("--detach", action="store_true")
    sp = sub.add_parser("score")
    sp.add_argument("--out", required=True)
    args = ap.parse_args()
    if args.cmd == "corpora":
        return corpora(args)
    if args.cmd == "run":
        return run(args)
    return score(args)


if __name__ == "__main__":
    sys.exit(main())
