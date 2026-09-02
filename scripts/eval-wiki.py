#!/usr/bin/env python3
"""주입 효용 — 위키 지식이 프롬프트에 있으면 답이 달라지는가.

팔은 둘이고 독립변수는 하나다: 시스템 프롬프트에 `wiki_resolve`가 돌려준 본문이
붙었는가. 질문도 도구도 코퍼스도 모델도 같다.

**에이전트에게 wiki 도구를 주지 않는 것은 의도다.** "에이전트가 위키를 스스로
부르는가"는 채택 질문이고, `graphin-rag` 프롬프트 201줄에 wiki가 한 번도 나오지
않으므로 지금 그 팔은 '주입 없음'과 구별되지 않는다. 여기서 재는 것은 효용 —
지식이 컨텍스트에 있을 때 답이 나아지는가 — 하나다.

`eval/rag`를 읽지 않는다. 위키가 답을 갖는 질문은 코드 질문과 다른 모집단이라
`eval/wiki`에 따로 산다. 채점 규칙(must_cite/evidence)만 eval-scaling과 같게 둬서
두 하니스가 "맞음"에 동의하게 한다.

**`docs/wiki`는 코퍼스에서 자르지 않는다.** 위키 파일은 실제로 저장소에 있고
검색으로 닿는다. 주입 없는 팔이 그것을 찾아내면 그것도 결과다 — 주입의 값은
"지식이 존재한다"가 아니라 "찾는 수고가 없다"이므로.

    scripts/eval-wiki.py run --out out/wiki-1 --runs 3
    scripts/eval-wiki.py score --out out/wiki-1

**세 번째 팔 `agent`는 선택이다.** `--agent-wiki DIR`로 다른 `docs/wiki`를 주면 —
`eval-wiki-maintain.py`가 남기는 `wiki-after-r0` 같은, 에이전트가 고친 위키 — 같은
질문에 그 위키의 세트를 주입한 팔이 하나 더 선다. 사람이 만든 세트와 에이전트가
고친 세트를 같은 자로 잰다(wiki-plan §P2 검증). 주입되는 것은 세트가 가리키는
**절의 본문**이므로, 에이전트가 앵커를 맞게 되돌렸다면 두 팔은 같은 프롬프트를
받는다 — 이 팔이 갈리는 것은 에이전트가 **다른 절**을 골랐을 때뿐이다.
"""

import argparse
import json
import os
import shutil
import subprocess
import statistics
import sys
import tempfile
import threading
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RUBRIC_VERSION = "0.1.0"

# 측정 장치는 코퍼스에서 잘라낸다. expected.jsonl의 evidence 리터럴이 코퍼스에
# 있으면 에이전트가 답을 거기서 읽는다 — rag 벤치가 실제로 겪은 사고다.
CUT_PREFIXES = ("eval/", "scripts/eval-")

ARMS = ("none", "injected")

# 위키 도구는 두 팔 모두에서 뺀다(위 docstring). --allowedTools는 print 모드에서
# 허용목록일 뿐이라 나머지 도구가 살아 있다 — 자르는 것은 --disallowedTools다.
DENIED = ("Edit,Write,NotebookEdit,Agent,Task,Skill,ScheduleWakeup,WebFetch,WebSearch,"
          "mcp__graphin__wiki_preflight,mcp__graphin__wiki_resolve,"
          "mcp__graphin__wiki_propose")
ALLOWED = ("mcp__graphin__bootstrap_workspace,mcp__graphin__search_hybrid,"
           "mcp__graphin__search_keyword,mcp__graphin__explore_graph,"
           "mcp__graphin__read_code,Read,Grep,Glob,Bash")

BASE_PROMPT = """You answer questions about this repository.

Ground every claim in the repository's own files. Name the file paths you relied
on — an answer without them is not an answer. Be specific and concise: state the
rule and the reason, not a survey of the area.
"""

INJECT_HEADER = """
# Project knowledge

These sections were selected for this task from the project's curated knowledge
sets. They are excerpts of this repository's own documentation, served with the
path each one came from.

"""


class MCP:
    """eval-recall.py의 클라이언트와 같다 — 두 시대의 핸드셰이크를 모두 탄다."""

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
    return [json.loads(l) for l in open(path, encoding="utf-8") if l.strip()]


def build_corpus(dest):
    """워킹 트리 사본. 측정 장치만 빼고 docs/wiki는 남긴다."""
    out = subprocess.run(["git", "-C", REPO, "ls-files", "-co", "--exclude-standard"],
                         capture_output=True, text=True, check=True).stdout.split("\n")
    n = 0
    for rel in out:
        if not rel or rel.startswith(CUT_PREFIXES):
            continue
        src = os.path.join(REPO, rel)
        if not os.path.isfile(src):
            continue
        dst = os.path.join(dest, rel)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copyfile(src, dst)
        n += 1
    return n


def mcp_argv(binary, root):
    return [binary, "--workspace", root, "--offline", "--ort-lib", "/nonexistent-ort"]


def with_wiki(root, wiki_dir):
    """docs/wiki만 바꿔 낀 코퍼스 사본. 문서는 같고 세트만 다르다."""
    alt = root + "-agentwiki"
    shutil.copytree(root, alt)
    shutil.rmtree(os.path.join(alt, "docs/wiki"))
    shutil.copytree(wiki_dir, os.path.join(alt, "docs/wiki"))
    return alt


def resolve_injection(binary, root, sets, index_timeout):
    """각 세트의 wiki_resolve 출력을 그대로 받아 온다 — 실제로 주입되는 그것."""
    m = MCP(mcp_argv(binary, root))
    try:
        era = m.handshake()
        m.tool("bootstrap_workspace", {})
        end = time.time() + index_timeout
        while time.time() < end:
            txt = m.tool("diagnose_index", {})
            if 'lexical_ready="true"' in txt:
                break
            time.sleep(1.0)
        else:
            raise SystemExit("index never became ready")
        return era, {s: m.tool("wiki_resolve", {"sets": [s]}) for s in sets}
    finally:
        m.close()


def read_transcript(path):
    names, inputs, ledger, final, usage, turns = {}, {}, [], "", {}, None
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
                ledger.append({"name": names.get(tid, "?"), "bytes": len(str(c).encode())})
        elif ev.get("type") == "result":
            final = ev.get("result") or ""
            usage = ev.get("modelUsage") or {}
            turns = ev.get("num_turns")
    tin = 0
    for k, v in usage.items():
        if "haiku" in k:
            continue
        tin += (v.get("inputTokens", 0) + v.get("cacheReadInputTokens", 0)
                + v.get("cacheCreationInputTokens", 0))
    return final, ledger, tin, turns


def grade(exp, final_text):
    """eval-scaling과 같은 규칙 — 두 하니스가 '맞음'에 동의해야 한다."""
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
    tasks = {t["id"]: t for t in load_jsonl(os.path.join(REPO, "eval/wiki/tasks.jsonl"))}
    qids = args.tasks.split(",") if args.tasks else list(tasks)
    os.makedirs(os.path.join(args.out, "transcripts"), exist_ok=True)

    tmp = tempfile.mkdtemp(prefix="graphin-wiki-")
    try:
        root = os.path.join(tmp, "corpus")
        os.makedirs(root)
        files = build_corpus(root)

        wanted = sorted({s for q in qids for s in tasks[q]["sets"]})
        era, inj = resolve_injection(args.bin, root, wanted, args.index_timeout)
        json.dump(inj, open(os.path.join(args.out, "injection.json"), "w"), ensure_ascii=False, indent=1)
        arms = list(ARMS)
        inj_by_arm = {"injected": inj}
        if args.agent_wiki:
            _, inj_agent = resolve_injection(args.bin, with_wiki(root, args.agent_wiki),
                                             wanted, args.index_timeout)
            json.dump(inj_agent, open(os.path.join(args.out, "injection-agent.json"), "w"),
                      ensure_ascii=False, indent=1)
            inj_by_arm["agent"] = inj_agent
            arms.append("agent")

        # 훅 전부와 플러그인을 끈다. 위키 게이트가 측정 대상 도구를 막고,
        # 설치된 플러그인은 이 코퍼스가 아니라 실제 저장소를 가리킨다.
        st = os.path.join(args.out, "settings.json")
        json.dump({"hooks": {}, "enabledPlugins": {}}, open(st, "w"))

        roots, jobs = [root], max(1, args.jobs)
        for i in range(1, jobs):
            r2 = f"{root}-w{i}"
            shutil.copytree(root, r2)
            roots.append(r2)
        cfgs = []
        for i, r in enumerate(roots):
            a = mcp_argv(args.bin, r)
            c = os.path.join(args.out, f"mcp-{i}.json")
            json.dump({"mcpServers": {"graphin": {"type": "stdio",
                                                  "command": a[0], "args": a[1:]}}},
                      open(c, "w"))
            cfgs.append(c)

        env = dict(os.environ)
        work = [(arm, q, r) for r in range(args.runs) for arm in arms for q in qids]
        rows, lk, state = [], threading.Lock(), {"n": 0}
        total = len(work)

        def worker(wi):
            while True:
                with lk:
                    if not work:
                        return
                    arm, q, ri = work.pop(0)
                prompt = BASE_PROMPT
                if arm in inj_by_arm:
                    prompt += INJECT_HEADER + "\n\n".join(
                        inj_by_arm[arm][s] for s in tasks[q]["sets"])
                pf = os.path.join(args.out, f"prompt-{arm}-{q}.md")
                open(pf, "w", encoding="utf-8").write(prompt)

                tag = f"{arm}_{q}_r{ri}"
                tp = os.path.join(args.out, "transcripts", tag + ".jsonl")
                cmd = ["claude", "-p", tasks[q]["question"],
                       "--settings", st, "--model", args.model,
                       "--system-prompt-file", pf,
                       "--output-format", "stream-json", "--verbose",
                       "--max-turns", str(args.max_turns),
                       "--strict-mcp-config", "--mcp-config", cfgs[wi],
                       "--allowedTools", ALLOWED, "--disallowedTools", DENIED]
                t0, err = time.time(), None
                try:
                    with open(tp, "w") as tf:
                        r = subprocess.run(cmd, stdout=tf, stderr=subprocess.PIPE,
                                           text=True, cwd=roots[wi], env=env,
                                           timeout=args.run_timeout)
                    if r.returncode != 0:
                        err = f"exit {r.returncode}: {r.stderr[-300:]}"
                except subprocess.TimeoutExpired:
                    err = f"timeout {args.run_timeout}s"
                row = {"arm": arm, "task": q, "run": ri, "transcript": tag + ".jsonl",
                       "wall_s": round(time.time() - t0, 1), "error": err,
                       "prompt_bytes": len(prompt.encode())}
                with lk:
                    rows.append(row)
                    state["n"] += 1
                    print(f"  [{state['n']}/{total}] {tag} {row['wall_s']}s"
                          + (f" ERR {err[:60]}" if err else ""), flush=True)

        ts = [threading.Thread(target=worker, args=(i,)) for i in range(jobs)]
        for t in ts:
            t.start()
        for t in ts:
            t.join()

        meta = {"rubric_version": RUBRIC_VERSION, "era": era, "corpus_files": files,
                "model": args.model, "runs": args.runs, "arms": arms,
                "agent_wiki": args.agent_wiki,
                "tasks": qids, "graphin_commit": subprocess.run(
                    ["git", "-C", REPO, "rev-parse", "--short", "HEAD"],
                    capture_output=True, text=True).stdout.strip(),
                "started": time.strftime("%Y-%m-%dT%H:%M:%S")}
        json.dump({"meta": meta, "rows": rows},
                  open(os.path.join(args.out, "summary.json"), "w"),
                  ensure_ascii=False, indent=1)
        print(f"\n완주 — {args.out}/summary.json")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def score(args):
    s = json.load(open(os.path.join(args.out, "summary.json")))
    exp = {r["id"]: r for r in load_jsonl(os.path.join(REPO, "eval/wiki/expected.jsonl"))}
    rows = []
    for row in s["rows"]:
        tp = os.path.join(args.out, "transcripts", row["transcript"])
        r = dict(row)
        if row["error"] or not os.path.exists(tp):
            r.update(correct=None, calls=None, retrieval=None, tokens_in=None)
        else:
            final, ledger, tin, turns = read_transcript(tp)
            r.update(grade(exp[row["task"]], final))
            r.update(calls=len(ledger), retrieval=sum(x["bytes"] for x in ledger),
                     tokens_in=tin, turns=turns)
        rows.append(r)

    def agg(arm, key):
        v = [r[key] for r in rows if r["arm"] == arm and r.get(key) is not None]
        return statistics.median(v) if v else None

    print(f"\n# eval-wiki — rubric {s['meta']['rubric_version']}")
    print(f"graphin {s['meta']['graphin_commit']} · {s['meta']['model']} · "
          f"runs {s['meta']['runs']} · 코퍼스 {s['meta']['corpus_files']}파일\n")
    print("## 팔 비교 — 중앙값")
    print("| 팔 | 정답 | 툴콜 | 리트리벌 | 입력토큰 | 벽시계 | 프롬프트 |")
    print("|---|---|--:|--:|--:|--:|--:|")
    for arm in s["meta"]["arms"]:
        got = [r for r in rows if r["arm"] == arm and r.get("correct") is not None]
        ok = sum(1 for r in got if r["correct"])
        pb = agg(arm, "prompt_bytes")
        print(f"| {arm} | {ok}/{len(got)} | {agg(arm,'calls')} | "
              f"{agg(arm,'retrieval')} | {agg(arm,'tokens_in')} | "
              f"{agg(arm,'wall_s')}s | {pb/1024:.1f}KB |")

    arms = s["meta"]["arms"]
    print("\n## 태스크별")
    print("| 태스크 | " + " | ".join(arms) + " | " + " | ".join(a + " 콜" for a in arms)
          + " | " + " | ".join(a + " 바이트" for a in arms) + " |")
    print("|---|" + "---|" * len(arms) + "--:|" * (2 * len(arms)))
    for q in s["meta"]["tasks"]:
        cell = {}
        for arm in arms:
            g = [r for r in rows if r["task"] == q and r["arm"] == arm
                 and r.get("correct") is not None]
            cell[arm] = (sum(1 for r in g if r["correct"]), len(g),
                         statistics.median([r["calls"] for r in g]) if g else None,
                         statistics.median([r["retrieval"] for r in g]) if g else None)
        print(f"| {q} | " + " | ".join(f"{cell[a][0]}/{cell[a][1]}" for a in arms) + " | "
              + " | ".join(str(cell[a][2]) for a in arms) + " | "
              + " | ".join(str(cell[a][3]) for a in arms) + " |")

    misses = [(r["task"], r["arm"], r.get("must_cite_miss"),
               r.get("evidence_miss") + ([] if r.get("evidence_any_hit") else ["<evidence_any 없음>"]))
              for r in rows if r.get("correct") is False]
    if misses:
        print("\n## 실패한 런이 놓친 것")
        seen = set()
        for t, a, mc, me in misses:
            k = (t, a, tuple(mc or []), tuple(me or []))
            if k in seen:
                continue
            seen.add(k)
            print(f"- `{t}` [{a}] cite={mc} evidence={me}")

    json.dump({"meta": s["meta"], "rows": rows},
              open(os.path.join(args.out, "report.json"), "w"),
              ensure_ascii=False, indent=1)


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    for name in ("run", "score"):
        p = sub.add_parser(name)
        p.add_argument("--out", required=True)
        if name == "run":
            p.add_argument("--bin", default=os.path.join(REPO, "bin/graphin"))
            p.add_argument("--model", default="sonnet")
            p.add_argument("--runs", type=int, default=3)
            p.add_argument("--jobs", type=int, default=2)
            p.add_argument("--tasks")
            p.add_argument("--agent-wiki", help="다른 docs/wiki 디렉터리 — 주면 `agent` 팔이 선다")
            p.add_argument("--max-turns", type=int, default=40)
            p.add_argument("--run-timeout", type=float, default=300.0)
            p.add_argument("--index-timeout", type=float, default=300.0)
    a = ap.parse_args()
    (run if a.cmd == "run" else score)(a)


if __name__ == "__main__":
    main()
