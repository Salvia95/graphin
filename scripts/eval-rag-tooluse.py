#!/usr/bin/env python3
"""Tool-use metrics for the tool-contract eval (docs/tool-contract-spec.md §5)
that eval-rag's scorer does not report: input misuse, unresolved refs, how
often a graphin search is followed by graphin vs host tools, and use of the
new symbol/detail parameters. Usage: scripts/eval-rag-tooluse.py OUT_DIR [OUT_DIR ...]

Observation only — nothing here gates. The keyword-list rule is a heuristic
(3+ words, no function word) and flags some terse sentences too."""
import glob, importlib.util, json, os, re, sys, statistics

spec = importlib.util.spec_from_file_location(
    "er", os.path.join(os.path.dirname(os.path.abspath(__file__)), "eval-rag.py"))
er = importlib.util.module_from_spec(spec)
spec.loader.exec_module(er)

FUNC = {"how", "what", "where", "when", "why", "which", "who", "does", "do", "is", "are", "the",
        "a", "an", "to", "of", "in", "for", "that", "with", "by", "on", "from", "after", "before",
        "when", "into", "it", "its", "be", "gets", "get"}
NAV = {"mcp__graphin__search_hybrid", "mcp__graphin__search_keyword",
       "mcp__graphin__explore_graph", "mcp__graphin__read_code"}
SEARCH = {"mcp__graphin__search_hybrid", "mcp__graphin__search_keyword"}
FOLLOW = {"mcp__graphin__explore_graph", "mcp__graphin__read_code"}
HOST = {"Read", "Grep", "Glob", "Bash"}


def words(s):
    return re.findall(r"[A-Za-z_][A-Za-z0-9_]*", s)


def hybrid_keyword_list(q):
    """3+ words, none of them a function word, no identifier-shape: a bag of
    keywords rather than an identifier or a sentence."""
    w = words(q)
    if len(w) < 3:
        return False
    if any(x.lower() in FUNC for x in w):
        return False
    return True


def keyword_prose(p, regex):
    """A question or sentence passed as literal text."""
    if regex:
        return False
    w = words(p)
    return len(w) >= 4 and any(x.lower() in FUNC for x in w) and p.strip()[:1].isalpha()


def analyze(out):
    rows = {}
    rp = os.path.join(out, "runs.jsonl")
    if os.path.exists(rp):
        for l in open(rp):
            r = json.loads(l)
            rows[f"{r['task']}_r{r.get('run', 0)}"] = r
    m = {"runs": 0, "cost": 0.0, "calls": [], "bytes": [], "nav_calls": 0, "host_calls": 0,
         "by_tool": {}, "hybrid": 0, "hybrid_kwlist": 0, "keyword": 0, "keyword_prose": 0,
         "not_found": 0, "ambiguous": 0, "symbol_inputs": 0, "detail_full": 0,
         "searches": 0, "search_then_graphin": 0, "search_then_host": 0, "search_then_none": 0,
         "runs_no_graphin": 0, "examples_kwlist": [], "examples_prose": []}
    for tr in sorted(glob.glob(os.path.join(out, "transcripts", "*.jsonl"))):
        final, ledger, cost, _ = er.parse_transcript(tr)
        m["runs"] += 1
        m["cost"] += cost or 0
        m["calls"].append(len(ledger))
        m["bytes"].append(sum(e["bytes"] for e in ledger))
        if not any(e["name"] in NAV for e in ledger):
            m["runs_no_graphin"] += 1
        for i, e in enumerate(ledger):
            n, inp, res = e["name"], e["input"], str(e["result"])
            m["by_tool"][n] = m["by_tool"].get(n, 0) + 1
            if n in NAV:
                m["nav_calls"] += 1
            if n in HOST:
                m["host_calls"] += 1
            if n == "mcp__graphin__search_hybrid":
                m["hybrid"] += 1
                q = inp.get("query", "")
                if hybrid_keyword_list(q):
                    m["hybrid_kwlist"] += 1
                    m["examples_kwlist"].append(q)
            if n == "mcp__graphin__search_keyword":
                m["keyword"] += 1
                p = inp.get("pattern", "")
                if keyword_prose(p, inp.get("regex")):
                    m["keyword_prose"] += 1
                    m["examples_prose"].append(p)
            if 'code="NODE_NOT_FOUND"' in res:
                m["not_found"] += 1
            if 'code="AMBIGUOUS_REF"' in res:
                m["ambiguous"] += 1
            if inp.get("symbol") or inp.get("symbols"):
                m["symbol_inputs"] += 1
            if inp.get("detail") == "full":
                m["detail_full"] += 1
            if n in SEARCH and "<node" in res:
                m["searches"] += 1
                nxt = [x["name"] for x in ledger[i + 1:i + 4]
                       if x["name"] in NAV | HOST and x["name"] not in SEARCH]
                if not nxt:
                    m["search_then_none"] += 1
                elif nxt[0] in FOLLOW:
                    m["search_then_graphin"] += 1
                else:
                    m["search_then_host"] += 1
    return m


def pct(a, b):
    return f"{100 * a / b:.0f}%" if b else "–"


def main():
    for out in sys.argv[1:]:
        m = analyze(out)
        if not m["runs"]:
            print(out, "no runs"); continue
        print(f"== {os.path.basename(out)}: {m['runs']} runs, ${m['cost']:.2f}")
        print(f"  calls/run median {statistics.median(m['calls'])}  bytes/run median {statistics.median(m['bytes']):.0f}")
        print(f"  graphin nav calls {m['nav_calls']}  host calls {m['host_calls']}  runs with no graphin call {m['runs_no_graphin']}")
        print(f"  hybrid keyword-list {m['hybrid_kwlist']}/{m['hybrid']} ({pct(m['hybrid_kwlist'], m['hybrid'])})"
              f"  keyword prose {m['keyword_prose']}/{m['keyword']} ({pct(m['keyword_prose'], m['keyword'])})")
        print(f"  NODE_NOT_FOUND {m['not_found']}  AMBIGUOUS_REF {m['ambiguous']}  symbol inputs {m['symbol_inputs']}  detail=full asked {m['detail_full']}")
        print(f"  after a search with results: graphin follow-up {pct(m['search_then_graphin'], m['searches'])}"
              f"  host tool {pct(m['search_then_host'], m['searches'])}  nothing {pct(m['search_then_none'], m['searches'])}  (n={m['searches']})")
        print("  by tool:", dict(sorted(m["by_tool"].items(), key=lambda x: -x[1])))
        if m["examples_kwlist"]:
            print("  e.g. keyword-list:", m["examples_kwlist"][:3])
        if m["examples_prose"]:
            print("  e.g. prose:", m["examples_prose"][:3])


if __name__ == "__main__":
    main()
