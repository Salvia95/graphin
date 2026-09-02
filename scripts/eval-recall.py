#!/usr/bin/env python3
"""Measure search recall@k of graphin over this repository.

The corpus is this repository. The golden set is two files that must be built
WITHOUT graphin (see .claude/skills/golden-set/SKILL.md); this script is the
other half of that split — it measures WITH graphin, driving the real binary
over the real MCP transport. Nothing here reaches into the index directly:
what it scores is what an agent would have received.

    scripts/eval-recall.py                          # the base tier, report only
    scripts/eval-recall.py --tier all               # every tier, one index pass
    scripts/eval-recall.py --min-recall 0.55        # gate: exit 1 below the floor
    scripts/eval-recall.py --worktree --json out.json

Tiers are scored separately on purpose. One mean over base + variants + hop is a
weighted average of different questions: the variants repeat base's questions
with a target filter, and the hop sets are a harder shape. Mixing them moves the
headline number for reasons that have nothing to do with search quality.

The workspace is a throwaway copy of the tree, never the repo itself: a live
MCP server already holds the workspace lock here, and indexing in place would
write .graphin/ into your checkout.
"""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Results pointing at these paths are the measurement reading its own tooling.
# They are reported, never filtered: a silent filter would hide the day the
# golden set starts answering its own questions.
TIERS = ("base", "variants", "hop", "tests")

SELF_PREFIXES = ("eval/golden/", ".claude/skills/", "scripts/eval-recall.py")

NODE_RE = re.compile(r'<node\s+([^>]*?)/>')
BLOCK_RE = re.compile(r'<code_block\s+([^>]*?)>')
ATTR_RE = re.compile(r'(\w+)="([^"]*)"')


# Directories scan.Walk prunes regardless of ignore files. grep must see the
# same tree, or the baseline is measured against a corpus graphin never had.
GREP_EXCLUDE = (".git", ".graphin", "node_modules", "dist", "build", "out",
                "target", ".gradle", "__pycache__", "venv", ".venv", ".next",
                "coverage")

IDENT_RE = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")


def terms_of(query):
    """internal/bench.Terms: the raw query plus its tokens, len>=2, lowercased.
    This is the repo's own §3.5 baseline definition — a literal grep of the
    question — and it is deliberately the worst case."""
    out, seen = [], set()
    for t in [query] + IDENT_RE.findall(query):
        t = t.strip().lower()
        if len(t) >= 2 and t not in seen:
            seen.add(t)
            out.append(t)
    return out


def ident_guess(query):
    """The tokens a grep-using agent could plausibly type: identifier-shaped
    ones only. Same rule graphin uses to pin Tier-0 (internal/search/router.go
    identTokens) — has an underscore, or mixed case, or is all caps. A prose
    question yields none, which is the finding, not a gap: the agent has to
    invent a term the question never contained."""
    out = []
    for t in IDENT_RE.findall(query):
        upper = any(c.isupper() for c in t)
        lower = any(c.islower() for c in t)
        if "_" in t or (upper and lower) or (upper and not lower and len(t) > 1):
            out.append(t)
    return out


def grep_arm(root, patterns, expected, evidence):
    """One grep arm: the files a case-insensitive grep would surface, the bytes
    an agent would ingest reading them whole (§3.5 scenario 1) and with -C 20
    (scenario 2), and whether the answer is in there at all."""
    if not patterns:
        return None
    args = []
    for t in patterns:
        args += ["-e", t]
    ex = [f"--exclude-dir={d}" for d in GREP_EXCLUDE]
    files = subprocess.run(["grep", "-rIl", "-i", *args, *ex, "."], cwd=root,
                           capture_output=True, text=True).stdout.split("\n")
    files = [f[2:] for f in files if f.startswith("./")]
    full = 0
    for rel in files:
        try:
            full += os.path.getsize(os.path.join(root, rel))
        except OSError:
            pass
    ctx = subprocess.run(["grep", "-rIn", "-i", "-C", "20", *args, *ex, "."], cwd=root,
                         capture_output=True, text=True).stdout
    hits = [f for f in expected if f in files]
    # Same delivery test the graphin arm gets: locating a file is not the same
    # as the agent holding the answer. grep pays for content, so it is scored
    # on content.
    got = [e for e in evidence if e in ctx] if evidence else []
    return {"patterns": patterns, "files": len(files), "full_bytes": full,
            "context_bytes": len(ctx.encode("utf-8")),
            "recall": len(hits) / len(expected), "found": len(hits),
            "delivery": (len(got) / len(evidence)) if evidence else None}


def measure_one(mcp, tmp, q, want_row, args):
    """Score one set: what graphin located, what it actually delivered into the
    agent's context, and what that cost — beside the same three numbers for a
    grep-using agent."""
    contaminated = []
    k = int(q.get("top_k", args.top_k))
    call = {"query": q["query"], "top_k": k}
    if q.get("target"):
        call["target"] = q["target"]
    xml = mcp.tool("search_hybrid", call)
    nodes = parse_nodes(xml)
    for n in nodes:
        if n.get("file", "").startswith(SELF_PREFIXES):
            contaminated.append((q["id"], n["file"]))
    want, evidence = want_row["files"], want_row.get("evidence", [])
    locate_bytes, explore_bytes = len(xml.encode("utf-8")), 0

    # The candidate pool an agent would be looking at. A hop set spends a
    # second turn on explore_graph, which is where the 2026-08-07 diagnosis put
    # the real failures — a locator that never follows an edge cannot see them.
    found = [n["id"] for n in nodes if n.get("id")]
    pool, neighbours = list(found), []
    if int(q.get("hops", 0)) >= 1 and found:
        gx = mcp.tool("explore_graph", {"node_id": found[0], "direction": "both"})
        explore_bytes = len(gx.encode("utf-8"))
        neighbours = [n["id"] for n in parse_nodes(gx)
                      if n.get("id") and n["id"] not in pool]
        pool += neighbours

    # An agent reads the top of the list, not the answers — it does not know
    # which ones those are. Scoring the read it would actually pay for is the
    # point of this arm.
    #
    # On a hop set it reads across the edge first. "What breaks if this changes"
    # is answered by the callers, not by the node the search landed on —
    # reading the anchor again is the one thing the second turn was not for.
    read_ids = ((neighbours + found) if neighbours else found)[:args.read_top]
    read_xml = mcp.tool("read_code", {"node_ids": read_ids}) if read_ids else ""
    read_bytes = len(read_xml.encode("utf-8"))
    read_files = [dict(ATTR_RE.findall(b)).get("file", "")
                  for b in BLOCK_RE.findall(read_xml)]

    seen = [n.get("file", "") for n in nodes] + read_files
    hits = [f for f in want if f in seen]
    delivered = [e for e in evidence if e in read_xml]

    return {
        "id": q["id"], "query": q["query"], "k": k, "hops": int(q.get("hops", 0)),
        "recall": len(hits) / len(want), "found": len(hits), "total": len(want),
        "delivery": (len(delivered) / len(evidence)) if evidence else None,
        "missing": [f for f in want if f not in seen],
        "undelivered": [e for e in evidence if e not in read_xml],
        "returned": [n.get("file", "") for n in nodes],
        "match_types": [n.get("match_type", "?") for n in nodes],
        "graphin_bytes": locate_bytes + explore_bytes + read_bytes,
        "locate_bytes": locate_bytes, "explore_bytes": explore_bytes,
        "read_bytes": read_bytes,
        "grep_literal": grep_arm(tmp, terms_of(q["query"]), want, evidence),
        "grep_symbol": grep_arm(tmp, q.get("grep") and [q["grep"]]
                                or ident_guess(q["query"]), want, evidence),
    }, contaminated


class MCP:
    """A dual-era MCP client: it probes for 2026-07-28 and falls back to the
    initialize handshake, which is what lets one script measure both an old
    binary and a new one."""

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
            return "2026-07-28 (stateless)"
        res = self.call("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}})
        if "error" in res:
            raise SystemExit(f"initialize failed: {res['error']}")
        return res["result"].get("protocolVersion", "?") + " (handshake)"

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


def materialize(dest, ref, worktree):
    """Copy the corpus. Tracked files only — an index over your build output
    and stray scratch files is not the repository anyone else would index."""
    if worktree:
        out = subprocess.run(["git", "-C", REPO, "ls-files", "-co", "--exclude-standard"],
                             capture_output=True, text=True, check=True).stdout
        for rel in out.splitlines():
            src, dst = os.path.join(REPO, rel), os.path.join(dest, rel)
            if not os.path.isfile(src):
                continue
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            shutil.copy2(src, dst)
        return "worktree"
    tar = subprocess.run(["git", "-C", REPO, "archive", ref], capture_output=True, check=True)
    subprocess.run(["tar", "-x", "-C", dest], input=tar.stdout, check=True)
    sha = subprocess.run(["git", "-C", REPO, "rev-parse", "--short", ref],
                         capture_output=True, text=True, check=True).stdout.strip()
    return sha


def parse_nodes(xml):
    out = []
    for raw in NODE_RE.findall(xml):
        out.append(dict(ATTR_RE.findall(raw)))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tier", default="base",
                    help="which tiers to score: base | variants | hop | all, or a "
                         "comma-separated list. They are scored separately because "
                         "one mean over all of them is a weighted average of "
                         "different questions (default: base)")
    ap.add_argument("--queries", help="override the tier layout with one query file")
    ap.add_argument("--expected", help="override the tier layout with one answer file")
    ap.add_argument("--bin", default=os.path.join(REPO, "bin/graphin"),
                    help="graphin binary (default: bin/graphin — run `make build`)")
    ap.add_argument("--ref", default="HEAD", help="git ref to index (default: HEAD)")
    ap.add_argument("--worktree", action="store_true",
                    help="index the working tree instead of a committed ref")
    ap.add_argument("--top-k", type=int, default=5, help="default top_k (default: 5)")
    ap.add_argument("--min-recall", type=float, default=None,
                    help="fail (exit 1) when mean recall falls below this")
    ap.add_argument("--semantic", action="store_true",
                    help="allow the embedding stack; off by default because a "
                         "gate must not depend on which artifacts a machine happens to cache")
    ap.add_argument("--read-top", type=int, default=3,
                    help="how many of the returned nodes the simulated agent reads (default: 3)")
    ap.add_argument("--index-timeout", type=float, default=300.0)
    ap.add_argument("--json", dest="json_out", help="write scores here")
    args = ap.parse_args()

    if not os.access(args.bin, os.X_OK):
        raise SystemExit(f"no graphin binary at {args.bin} — run `make build` or pass --bin")

    if bool(args.queries) != bool(args.expected):
        raise SystemExit("--queries and --expected go together or not at all")
    if args.queries:
        tiers = [("custom", args.queries, args.expected)]
    else:
        names = TIERS if args.tier == "all" else [t.strip() for t in args.tier.split(",")]
        for n in names:
            if n not in TIERS:
                raise SystemExit(f"unknown tier {n!r} — pick from {', '.join(TIERS)}, or all")
        tiers = [(n, os.path.join(REPO, "eval/golden", n, "queries.jsonl"),
                  os.path.join(REPO, "eval/golden", n, "expected.jsonl")) for n in names]

    loaded = []
    for name, qp, ep in tiers:
        queries = load_jsonl(qp)
        expected = {r["id"]: r for r in load_jsonl(ep)}
        if not queries:
            raise SystemExit(f"{name}: the query file is empty")
        # The two files are separate on purpose, so their agreement is the one
        # thing nobody can forget to check.
        qids = [q["id"] for q in queries]
        if len(set(qids)) != len(qids):
            raise SystemExit(f"{name}: duplicate ids in the query file")
        missing = [i for i in qids if i not in expected]
        orphan = [i for i in expected if i not in set(qids)]
        if missing or orphan:
            raise SystemExit(f"{name}: queries and expected disagree — no answers for "
                             f"{missing}, no questions for {orphan}")
        loaded.append((name, queries, expected))

    tmp = tempfile.mkdtemp(prefix="graphin-recall-")
    try:
        origin = materialize(tmp, args.ref, args.worktree)
        argv = [args.bin, "--workspace", tmp, "--offline"]
        if not args.semantic:
            argv += ["--ort-lib", "/nonexistent-ort"]  # lexical only, reproducibly
        mcp = MCP(argv)
        era = mcp.handshake()

        mcp.tool("bootstrap_workspace", {})
        # Readiness is not one condition, and getting it wrong is silent.
        # Lexical-only never reaches the FSM's ready phase at all — the model
        # it is waiting for will never arrive — so it waits on the lexical
        # flag. Hybrid has to wait for the model AND for the embedding backlog
        # to drain; stopping at "model loaded" scores a half-filled vector
        # index and reports it as a hybrid number.
        end = time.time() + args.index_timeout
        while True:
            text = mcp.tool("search_hybrid", {"query": "___warmup___"})
            if args.semantic:
                if 'code="MODEL_UNAVAILABLE"' in text:
                    raise SystemExit("semantic is unavailable here (no artifacts, or an "
                                     "unsupported platform) — rerun without --semantic")
                if 'semantic_ready="true"' in text:
                    diag = mcp.tool("diagnose_index", {})
                    if 'drained="true"' in diag and 'pending="0"' in diag:
                        break
            elif 'lexical_ready="true"' in text:
                break
            if time.time() > end:
                raise SystemExit(f"index never became ready within {args.index_timeout}s")
            time.sleep(0.2)

        results = []
        for name, queries, expected in loaded:
            rows, contaminated = [], []
            for q in queries:
                row, bad = measure_one(mcp, tmp, q, expected[q["id"]], args)
                rows.append(row)
                contaminated += bad
            results.append((name, rows, contaminated))
        mcp.close()
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    def kb(n):
        if n is None:
            return "—"
        return f"{n / 1024:.0f}K" if n >= 1024 else f"{n}B"

    def pct(v):
        return "  —" if v is None else f"{v:>3.0%}"

    def cell(a):
        if a is None:
            return f"{'n/a':>4} {'—':>4} {'—':>7}"
        return f"{a['recall']:>4.0%} {pct(a.get('delivery')):>4} {kb(a['context_bytes']):>7}"

    def mean_of(rows, key):
        vals = [r[key] for r in rows if r[key] is not None]
        return sum(vals) / len(vals) if vals else None

    label = "hybrid" if args.semantic else "lexical only"
    print(f"corpus {origin} · {era} · {label}")
    print("\n찾았나(recall) · 실제로 받아 읽었나(delivery) · 그 대가로 삼킨 바이트")

    failures, summary = [], []
    for name, rows, contaminated in results:
        mean = sum(r["recall"] for r in rows) / len(rows)
        deliv = mean_of(rows, "delivery")
        g_bytes = sum(r["graphin_bytes"] for r in rows)

        print(f"\n[{name}] {len(rows)}세트")
        print(f"{'':<28}{'graphin':^19}{'grep 문장 그대로':^19}{'grep 심볼 추측':^19}")
        print(f"{'':<28}{'rec  del   bytes':^19}{'rec  del   bytes':^19}{'rec  del   bytes':^19}")
        print("-" * 88)
        for r in rows:
            tag = "↳" if r.get("hops") else " "
            g = f"{r['recall']:>4.0%} {pct(r['delivery']):>4} {kb(r['graphin_bytes']):>7}"
            print(f"{tag}{r['id']:<27}{g:^19}{cell(r['grep_literal']):^19}"
                  f"{cell(r['grep_symbol']):^19}")
        print("-" * 88)

        def arm(key, field):
            vals = [r[key] for r in rows if r[key] is not None]
            if not vals:
                return None, 0
            return (sum(v["recall"] for v in vals) / len(vals),
                    sum(v[field] for v in vals))

        lit_r, lit_b = arm("grep_literal", "context_bytes")
        sym_r, sym_b = arm("grep_symbol", "context_bytes")
        n_sym = sum(1 for r in rows if r["grep_symbol"] is not None)
        print(f"  graphin           recall {mean:>5.1%}  delivery {pct(deliv)}  {kb(g_bytes)}"
              f"  (위치 {kb(sum(r['locate_bytes'] for r in rows))}"
              f" + 탐색 {kb(sum(r['explore_bytes'] for r in rows))}"
              f" + 읽기 {kb(sum(r['read_bytes'] for r in rows))})")
        if lit_r is not None:
            print(f"  grep 문장 그대로  recall {lit_r:>5.1%}  "
                  f"delivery {pct(mean_of([r['grep_literal'] for r in rows if r['grep_literal']], 'delivery'))}"
                  f"  {kb(lit_b)}")
        if sym_r is not None:
            print(f"  grep 심볼 추측    recall {sym_r:>5.1%}  "
                  f"delivery {pct(mean_of([r['grep_symbol'] for r in rows if r['grep_symbol']], 'delivery'))}"
                  f"  {kb(sym_b)}  · 추측 가능 {n_sym}/{len(rows)}")

        if contaminated:
            print("  경고 — 측정이 자기 도구를 되받았다 (필터하지 않았다):")
            for qid, f in contaminated:
                print(f"    {qid}: {f}")

        summary.append({"tier": name, "sets": len(rows), "mean_recall": mean,
                        "mean_delivery": deliv, "graphin_bytes": g_bytes,
                        "grep_literal_recall": lit_r, "grep_literal_bytes": lit_b,
                        "grep_symbol_recall": sym_r, "grep_symbol_bytes": sym_b,
                        "queries": rows, "contaminated": contaminated})
        if args.min_recall is not None and mean < args.min_recall:
            failures.append((name, mean))

    if args.json_out:
        with open(args.json_out, "w", encoding="utf-8") as f:
            json.dump({"corpus": origin, "era": era, "semantic": args.semantic,
                       "tiers": summary}, f, ensure_ascii=False, indent=2)
        print(f"\nscores → {args.json_out}")

    if failures:
        print()
        for name, mean in failures:
            print(f"FAIL: [{name}] mean recall {mean:.1%} < floor {args.min_recall:.1%}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
