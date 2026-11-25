#!/usr/bin/env python3
"""Score `graphin eval swe-explore` submissions with the official scorer.

The harness only writes ranked (path, start, end) submissions; scoring is the
benchmark's own `ExploreEvaluator` (SWE-Explore-Bench `eval.py`), imported from
a checkout pinned to the commit every earlier run used (`3c12dc5`). This file
only adapts the submission JSONL to the evaluator's callback and aggregates.

    scripts/swe-explore-score.py --scorer <SWE-Explore-Bench checkout @3c12dc5> \
        --bench bench.jsonl --group-by repo sub-graphin.jsonl sub-grep.jsonl

Prints one table per submission (overall, then per repository) and writes the
per-instance metrics next to each submission as <name>.scores.json.
"""
import argparse
import importlib.util
import json
import os
import re
import statistics
import sys

METRICS = ["recall", "precision", "hit_file_rate", "recall_at_300", "ndcg_at_300",
           "first_useful_hit"]


def load_evaluator(scorer_dir):
    spec = importlib.util.spec_from_file_location("swe_explore_eval", os.path.join(scorer_dir, "eval.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.ExploreEvaluator


def repo_of(instance_id):
    base = re.sub(r"-[0-9a-f]{20,}(-v.*)?$", "", instance_id)
    return re.sub(r"-\d+$", "", base)


def score(evaluator_cls, bench, submission):
    preds = {}
    for line in open(submission, encoding="utf-8"):
        row = json.loads(line)
        preds[row["instance_id"]] = [(r["path"], r["start"], r["end"]) for r in row["regions"]]
    ev = evaluator_cls(bench)
    ids = [i for i in preds if i in ev.bench_data_dict]
    res = ev.evaluate(lambda _issue, iid: preds.get(iid, []), ids, METRICS)
    for iid in ids:
        res[iid]["lines"] = sum(max(0, e - s + 1) for _, s, e in preds[iid])
    return res


def table(rows, title):
    keys = METRICS + ["lines"]
    print(f"\n### {title}  (n={len(rows)})")
    print("| " + " | ".join(keys) + " |")
    print("|" + "---|" * len(keys))
    print("| " + " | ".join(f"{statistics.mean(r[k] for r in rows):.3f}" if k != "lines"
                             else f"{statistics.mean(r[k] for r in rows):.0f}" for k in keys) + " |")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--scorer", required=True, help="SWE-Explore-Bench checkout pinned to 3c12dc5")
    ap.add_argument("--bench", required=True)
    ap.add_argument("--group-by", choices=["none", "repo"], default="repo")
    ap.add_argument("submissions", nargs="+")
    args = ap.parse_args()
    evaluator_cls = load_evaluator(args.scorer)
    for sub in args.submissions:
        res = score(evaluator_cls, args.bench, sub)
        with open(os.path.splitext(sub)[0] + ".scores.json", "w") as f:
            json.dump(res, f, indent=1)
        print(f"\n## {os.path.basename(sub)}")
        table(list(res.values()), "all")
        if args.group_by == "repo":
            groups = {}
            for iid, r in res.items():
                groups.setdefault(repo_of(iid), []).append(r)
            for g in sorted(groups):
                table(groups[g], g)
    return 0


if __name__ == "__main__":
    sys.exit(main())
