#!/usr/bin/env python3
"""Token cost of eval-rag runs, read from the transcripts' own usage records.

bytes_total (the scorer's budget axis) counts only the tool results an agent
received. What a run costs is tokens: every turn re-reads the whole context —
system prompt, tool definitions, every earlier result — so a byte received on
turn 2 is paid for again on every later turn, and a longer tool description
is paid on every turn without ever showing up in bytes_total.

Per run: input = input + cache_read + cache_creation tokens summed over the
run (what the model processed), base = the first turn's input (system prompt +
tool definitions + question: the fixed cost), output tokens, turns, and the
billed cost. Paired comparisons are per task (median over runs), sign test.

    scripts/eval-rag-tokens.py OUT_A OUT_B [OUT_C ...]   # first is the reference
"""
import glob, json, math, os, statistics, sys
from collections import defaultdict


def run_tokens(path):
    first, inp, out, turns, cost = None, 0, 0, 0, 0.0
    for line in open(path, encoding="utf-8"):
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("type") == "assistant" and first is None:
            u = e.get("message", {}).get("usage") or {}
            first = (u.get("input_tokens", 0) + u.get("cache_read_input_tokens", 0)
                     + u.get("cache_creation_input_tokens", 0))
        if e.get("type") == "result":
            u = e.get("usage") or {}
            inp = (u.get("input_tokens", 0) + u.get("cache_read_input_tokens", 0)
                   + u.get("cache_creation_input_tokens", 0))
            out = u.get("output_tokens", 0)
            turns = e.get("num_turns") or 0
            cost = e.get("total_cost_usd") or 0.0
    return {"base": first or 0, "input": inp, "output": out, "turns": turns, "cost": cost}


def load(out):
    verdict = {}
    rp = os.path.join(out, "report.json")
    if os.path.exists(rp):
        for r in json.load(open(rp))["runs"]:
            verdict[f"{r['task']}_r{r['run']}"] = (r["verdict"] == "pass", r["bytes_total"])
    rows = []
    for tr in sorted(glob.glob(os.path.join(out, "transcripts", "*.jsonl"))):
        tag = os.path.basename(tr)[:-6]
        task = tag.rsplit("_r", 1)[0]
        t = run_tokens(tr)
        t["task"] = task
        t["pass"], t["bytes"] = verdict.get(tag, (None, None))
        rows.append(t)
    return rows


def sign(xs, ys):
    w = sum(x < y for x, y in zip(xs, ys))
    l = sum(x > y for x, y in zip(xs, ys))
    n, k = w + l, min(w, l)
    p = min(1, 2 * sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n) if n else 1.0
    return w, l, len(xs) - n, p


def per_task(rows, key):
    d = defaultdict(list)
    for r in rows:
        d[r["task"]].append(r[key])
    return {k: statistics.median(v) for k, v in d.items()}


def main():
    outs = sys.argv[1:]
    data = {os.path.basename(o.rstrip("/")): load(o) for o in outs}
    names = list(data)
    print(f"{'arm':14} {'runs':>4} {'pass':>5} {'base tok':>9} {'input tok':>10} {'output':>7} "
          f"{'turns':>5} {'bytes':>7} {'tok/pass':>9} {'$/run':>6} {'$ total':>7}")
    for n in names:
        rows = data[n]
        med = lambda k: statistics.median(r[k] for r in rows)
        passes = sum(1 for r in rows if r["pass"])
        tot_in = sum(r["input"] + r["output"] for r in rows)
        bts = [r["bytes"] for r in rows if r["bytes"] is not None]
        print(f"{n:14} {len(rows):>4} {passes:>5} {med('base'):>9.0f} {med('input'):>10.0f} {med('output'):>7.0f} "
              f"{med('turns'):>5.0f} {statistics.median(bts) if bts else 0:>7.0f} "
              f"{(tot_in / passes if passes else 0):>9.0f} {med('cost'):>6.3f} {sum(r['cost'] for r in rows):>7.2f}")
    ref = names[0]
    for n in names[1:]:
        for key in ("input", "cost", "turns"):
            a, b = per_task(data[n], key), per_task(data[ref], key)
            tasks = sorted(set(a) & set(b))
            w, l, t, p = sign([a[k] for k in tasks], [b[k] for k in tasks])
            ratio = statistics.median(r[key] for r in data[n]) / max(1e-9, statistics.median(r[key] for r in data[ref]))
            print(f"  {n} vs {ref} {key:6}: lower {w} · higher {l} · tie {t}  p={p:.3f}  median ×{ratio:.2f}")


if __name__ == "__main__":
    main()
