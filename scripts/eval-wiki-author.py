#!/usr/bin/env python3
"""생성 드릴 — 에이전트가 작업 마무리에 쓰는 세트는 사람이 쓴 세트에 얼마나 가까운가.

사람이 쓴 세트 하나를 코퍼스에서 지우고, 그 세트가 답하던 질문으로 preflight를 돌려
miss를 기록한 뒤, 에이전트에게 **질문에 답하고 마무리에 세트를 쓰라**고 시킨다.
채점은 LLM 없이 파일만 본다:

- **썼는가** — 세트 파일이 생겼고 `origin: agent`·`reviewed: false`를 달았는가,
  `graphin wiki check`가 깨끗한가.
- **닿는가** — 같은 질문으로 다시 preflight하면 그 세트가 카탈로그에 뜨는가.
- **겹치는가** — 지운 사람 세트의 절과 얼마나 겹치는가(절 단위 recall/precision,
  파일 단위 recall). 사람 세트가 정답은 아니지만 유일한 기준점이다.
- **거부됐는가** — 도구가 어떤 규칙으로 몇 번 거부했는가(트랜스크립트).

고친 위키를 `wiki-after-rN/`에 남기므로 `eval-wiki.py --agent-wiki`가 그것을 주입해
사람 세트·에이전트 세트의 답 품질을 같은 자로 잴 수 있다. 세트 이름은 지운 것과
같게 쓰게 한다 — 그 팔이 이름으로 주입을 찾기 때문이다.

    scripts/eval-wiki-author.py run --out out/author-1 --task wiki-rescore-compat
    scripts/eval-wiki-author.py score --out out/author-1
"""

import argparse
import importlib.util
import json
import os
import re
import shutil
import subprocess
import tempfile
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RUBRIC_VERSION = "0.1.0"
SKILL_MD = os.path.join(REPO, "plugin/graphin-guide/skills/knowledge/SKILL.md")

ALLOWED = ("mcp__graphin__bootstrap_workspace,mcp__graphin__diagnose_index,"
           "mcp__graphin__search_hybrid,mcp__graphin__search_keyword,"
           "mcp__graphin__explore_graph,mcp__graphin__read_code,"
           "mcp__graphin__wiki_preflight,mcp__graphin__wiki_resolve,"
           "mcp__graphin__wiki_write_set,Bash,Read,Grep,Glob")
DENIED = ("Edit,Write,NotebookEdit,Agent,Task,Skill,ScheduleWakeup,WebFetch,WebSearch,"
          "mcp__graphin__wiki_edit_set,mcp__graphin__wiki_propose")

WRAPUP = """
# At wrap-up

This task's `wiki_preflight` returned an empty catalogue. The exact task sentence was:

    {task}

Answer the question first. Then, before your final report, write the knowledge set
that would have made this task shorter: call `wiki_write_set` with `task` set to that
sentence verbatim and `name` set to `{name}`. List only sections you actually read,
each with one line saying what it claims. If the tool rejects the draft, read the
rule and fix the draft — do not give up after one refusal. Then report the answer
and, in one line, what you wrote.
"""


def _load(name):
    spec = importlib.util.spec_from_file_location(name, os.path.join(REPO, "scripts", name + ".py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


wiki_runner = _load("eval-wiki")
maint = _load("eval-wiki-maintain")


def strip_frontmatter(text):
    if text.startswith("---"):
        end = text.find("\n---", 3)
        if end != -1:
            text = text[end + 4:]
    text = re.sub(r"\A\s*<!--.*?-->\s*", "", text, count=1, flags=re.S)
    return text.strip() + "\n"


def remove_set(root, name):
    os.remove(os.path.join(root, "docs/wiki/sets", name + ".md"))
    pp = os.path.join(root, "docs/wiki/pins.lock")
    pins = json.load(open(pp, encoding="utf-8"))
    pins["pins"].pop(name, None)
    json.dump(pins, open(pp, "w", encoding="utf-8"), indent=2, ensure_ascii=False)
    open(pp, "a").write("\n")


def preflight(binary, root, question, index_timeout):
    """색인을 끝내고 preflight 한 번. 카탈로그가 비었는지와 뜬 세트를 돌려준다."""
    m = wiki_runner.MCP(wiki_runner.mcp_argv(binary, root))
    try:
        era = m.handshake()
        m.tool("bootstrap_workspace", {})
        end = time.time() + index_timeout
        while time.time() < end:
            if 'lexical_ready="true"' in m.tool("diagnose_index", {}):
                break
            time.sleep(1.0)
        else:
            raise SystemExit("index never became ready")
        cat = m.tool("wiki_preflight", {"task": question, "role": ""})
        return era, re.findall(r'<set name="([^"]+)"', cat), "<none>" in cat
    finally:
        m.close()


def rejections(path):
    """wiki_write_set이 거부한 횟수와 규칙."""
    out = []
    for line in open(path, encoding="utf-8"):
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("type") != "user":
            continue
        for b in ev["message"].get("content", []):
            if b.get("type") != "tool_result":
                continue
            c = b.get("content", "")
            if isinstance(c, list):
                c = "".join(x.get("text", "") for x in c if isinstance(x, dict))
            c = str(c)
            if 'op="create" applied="false"' in c:
                out.append(re.findall(r'rule="([^"]+)"', c))
    return out


def score_corpus(root, pristine_docs, name, human_path, binary, question, index_timeout):
    res = {"created": False, "origin_agent": False, "unreviewed": False, "entries": 0,
           "recall_nodes": None, "precision_nodes": None, "recall_files": None,
           "reachable": None, "check_after": "", "collateral": []}
    path = os.path.join(root, "docs/wiki/sets", name + ".md")
    human_entries, _, _ = maint.parse_set(human_path, "docs/wiki/sets/" + name + ".md")
    hn = {e["node"] for e in human_entries}
    hf = {e["node"].split("#")[0] for e in human_entries}
    if os.path.exists(path):
        res["created"] = True
        txt = open(path, encoding="utf-8").read()
        res["origin_agent"] = "origin: agent" in txt
        res["unreviewed"] = "reviewed: false" in txt
        entries, desc, _ = maint.parse_set(path, "docs/wiki/sets/" + name + ".md")
        an = {e["node"] for e in entries}
        af = {e["node"].split("#")[0] for e in entries}
        res["entries"] = len(entries)
        res["description"] = desc
        res["nodes"] = sorted(an)
        res["recall_nodes"] = round(len(an & hn) / len(hn), 2) if hn else None
        res["precision_nodes"] = round(len(an & hn) / len(an), 2) if an else None
        res["recall_files"] = round(len(af & hf) / len(hf), 2) if hf else None
        _, sets, _ = preflight(binary, root, question, index_timeout)
        res["reachable"] = name in sets
        res["catalogue"] = sets
    # 다른 세트나 문서를 건드렸는가. 새 세트 파일과 pins.lock은 도구가 쓰는 것이라
    # 부수 피해가 아니다.
    own = {"docs/wiki/sets/" + name + ".md", "docs/wiki/pins.lock"}
    for dp, _, files in os.walk(os.path.join(pristine_docs, "docs")):
        for fn in files:
            p = os.path.join(dp, fn)
            rel = os.path.relpath(p, pristine_docs)
            q = os.path.join(root, rel)
            if rel in own:
                continue
            if not os.path.exists(q) or open(q, "rb").read() != open(p, "rb").read():
                res["collateral"].append(rel)
    chk = subprocess.run([binary, "wiki", "check", "--root", root], capture_output=True, text=True)
    res["check_after"] = (chk.stdout.strip().splitlines() or [""])[-1]
    res["human_nodes"] = sorted(hn)
    return res


def run(args):
    tasks = {t["id"]: t for t in wiki_runner.load_jsonl(os.path.join(REPO, "eval/wiki/tasks.jsonl"))}
    task = tasks[args.task]
    name = task["sets"][0]
    question = task["question"]
    human_path = os.path.join(REPO, "docs/wiki/sets", name + ".md")

    os.makedirs(os.path.join(args.out, "transcripts"), exist_ok=True)
    st = os.path.join(args.out, "settings.json")
    json.dump({"hooks": {}, "enabledPlugins": {}}, open(st, "w"))
    prompt = (wiki_runner.BASE_PROMPT + "\n" + strip_frontmatter(open(SKILL_MD, encoding="utf-8").read())
              + WRAPUP.format(task=question, name=name))
    prompt_path = os.path.join(args.out, "system-prompt.md")
    open(prompt_path, "w", encoding="utf-8").write(prompt)

    rows = []
    tmp = tempfile.mkdtemp(prefix="graphin-author-")
    try:
        for ri in range(args.runs):
            root = os.path.join(tmp, f"corpus-{ri}")
            os.makedirs(root)
            files = wiki_runner.build_corpus(root)
            remove_set(root, name)
            era, before, empty = preflight(args.bin, root, question, args.index_timeout)
            pristine = root + "-pristine"
            shutil.copytree(os.path.join(root, "docs"), os.path.join(pristine, "docs"))

            cfg = os.path.join(args.out, f"mcp-{ri}.json")
            a = wiki_runner.mcp_argv(args.bin, root)
            json.dump({"mcpServers": {"graphin": {"type": "stdio", "command": a[0], "args": a[1:]}}},
                      open(cfg, "w"))
            tag = f"author_r{ri}"
            tp = os.path.join(args.out, "transcripts", tag + ".jsonl")
            env = dict(os.environ)
            env["PATH"] = os.path.dirname(os.path.abspath(args.bin)) + os.pathsep + env.get("PATH", "")
            env["GRAPHIN_WIKI_GATE"] = "off"
            cmd = ["claude", "-p", question, "--settings", st, "--model", args.model,
                   "--system-prompt-file", prompt_path,
                   "--output-format", "stream-json", "--verbose",
                   "--max-turns", str(args.max_turns),
                   "--strict-mcp-config", "--mcp-config", cfg,
                   "--allowedTools", ALLOWED, "--disallowedTools", DENIED]
            t0, err = time.time(), None
            try:
                with open(tp, "w") as tf:
                    r = subprocess.run(cmd, stdout=tf, stderr=subprocess.PIPE, text=True,
                                       cwd=root, env=env, timeout=args.run_timeout)
                if r.returncode != 0:
                    err = f"exit {r.returncode}: {r.stderr[-300:]}"
            except subprocess.TimeoutExpired:
                err = f"timeout {args.run_timeout}s"
            wall = round(time.time() - t0, 1)
            scored = score_corpus(root, pristine, name, human_path, args.bin, question, args.index_timeout)
            final, ledger, tin, turns = wiki_runner.read_transcript(tp)
            exp = {e["id"]: e for e in wiki_runner.load_jsonl(os.path.join(REPO, "eval/wiki/expected.jsonl"))}
            rows.append({"run": ri, "transcript": tag + ".jsonl", "wall_s": wall, "error": err,
                         "miss_recorded": empty, "catalogue_before": before,
                         "result": scored, "rejections": rejections(tp),
                         "answer": wiki_runner.grade(exp[args.task], final),
                         "tokens_in": tin, "turns": turns, "calls": len(ledger),
                         "tools": sorted({x["name"] for x in ledger})})
            print(f"  [{ri + 1}/{args.runs}] {tag} {wall}s" + (f" ERR {err[:80]}" if err else ""), flush=True)
            shutil.copytree(os.path.join(root, "docs/wiki"), os.path.join(args.out, f"wiki-after-r{ri}"),
                            dirs_exist_ok=True)
        meta = {"rubric_version": RUBRIC_VERSION, "era": era, "corpus_files": files,
                "model": args.model, "runs": args.runs, "task": args.task, "set": name,
                "graphin_commit": subprocess.run(["git", "-C", REPO, "rev-parse", "--short", "HEAD"],
                                                 capture_output=True, text=True).stdout.strip(),
                "started": time.strftime("%Y-%m-%dT%H:%M:%S")}
        json.dump({"meta": meta, "rows": rows}, open(os.path.join(args.out, "summary.json"), "w"),
                  ensure_ascii=False, indent=1)
        print(f"\n완주 — {args.out}/summary.json")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def score(args):
    s = json.load(open(os.path.join(args.out, "summary.json")))
    m = s["meta"]
    print(f"\n# eval-wiki-author — rubric {m['rubric_version']}")
    print(f"graphin {m['graphin_commit']} · {m['model']} · 태스크 {m['task']} · 지운 세트 `{m['set']}` · runs {m['runs']}\n")
    print("| 런 | miss 기록 | 썼는가 | 표시 | 엔트리 | recall 절 | precision 절 | recall 파일 | 닿는가 | 거부 | check 후 | 답 정답 | 입력토큰 | 턴 | 벽시계 |")
    print("|---|---|---|---|--:|--:|--:|--:|---|---|---|---|--:|--:|--:|")
    for r in s["rows"]:
        x = r["result"]
        mark = "agent+unreviewed" if x["origin_agent"] and x["unreviewed"] else ("없음" if x["created"] else "-")
        rej = ", ".join("+".join(k) for k in r["rejections"]) or "0"
        print(f"| r{r['run']} | {r['miss_recorded']} | {x['created']} | {mark} | {x['entries']} | "
              f"{x['recall_nodes']} | {x['precision_nodes']} | {x['recall_files']} | {x['reachable']} | "
              f"{rej} | {x['check_after']} | {r['answer']['correct']} | {r['tokens_in']} | {r['turns']} | {r['wall_s']}s |"
              + (f" ERR {r['error'][:60]}" if r["error"] else ""))
    print("\n## 절 대조")
    for r in s["rows"]:
        x = r["result"]
        print(f"- r{r['run']} 사람: {', '.join(x['human_nodes'])}")
        if x["created"]:
            print(f"- r{r['run']} 에이전트: {', '.join(x['nodes'])}")
            print(f"- r{r['run']} description: {x.get('description')}")
            print(f"- r{r['run']} 다시 preflight한 카탈로그: {x.get('catalogue')}")
        for c in x["collateral"]:
            print(f"- r{r['run']} 부수 피해: {c}")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    for name in ("run", "score"):
        p = sub.add_parser(name)
        p.add_argument("--out", required=True)
        if name == "run":
            p.add_argument("--bin", default=os.path.join(REPO, "bin/graphin"))
            p.add_argument("--model", default="sonnet")
            p.add_argument("--runs", type=int, default=1)
            p.add_argument("--task", default="wiki-rescore-compat")
            p.add_argument("--max-turns", type=int, default=60)
            p.add_argument("--run-timeout", type=float, default=1200.0)
            p.add_argument("--index-timeout", type=float, default=300.0)
    a = ap.parse_args()
    (run if a.cmd == "run" else score)(a)


if __name__ == "__main__":
    main()
