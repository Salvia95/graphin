#!/usr/bin/env python3
"""통합 벤치 — 제품 전체와 베이스라인을 한 질문에서 만나게 한다.

설계는 `docs/combined-bench-spec.md`. 요지만 적자면:

- 팔 둘. `grep`은 Read/Grep/Glob/Bash만 들고 주입 없이 시작하고, `wiki-rag`는
  graphin 검색 도구에 `wiki_preflight` → `wiki_resolve`의 본문을 프롬프트로 받는다.
  **도구와 주입이 함께 움직인다** — 기여도를 가르는 벤치가 아니라 제품 전체를
  베이스라인과 재는 벤치다.
- 네 영역을 따로 채점한다: 탐색(코드) · 지식(규칙) · 통찰(결합해야 나오는 결론) ·
  종합(셋 AND). 층을 섞은 평균은 내지 않는다.
- 여섯 문항 중 둘은 위키가 답을 갖지 않는다(`wiki_covered: false`). 주입이 손해인지
  재는 프로브이고, 없으면 이 벤치의 수치는 상한일 뿐이다.

    scripts/eval-combined.py run --out out/cb-1 --runs 3 --jobs 2
    scripts/eval-combined.py score --out out/cb-1
"""

import argparse
import json
import math
import os
import re
import shutil
import statistics
import subprocess
import threading
import time
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RUBRIC_VERSION = "0.1.0"
CUT_PREFIXES = ("eval/", "scripts/eval-")
ARMS = ("grep", "wiki-rag", "wiki-gated")
SYSTEM_PATHS = ("/dev/", "/tmp", "/proc/", "/sys/", "/usr/", "/bin/", "/etc/", "/var/")

DENIED_BASE = "Edit,Write,NotebookEdit,Agent,Task,Skill,ScheduleWakeup,WebFetch,WebSearch"
WIKI_TOOLS = ("mcp__graphin__wiki_preflight,mcp__graphin__wiki_resolve,"
              "mcp__graphin__wiki_propose")
# wiki-gated 팔만 위키 도구를 쥔다. 나머지 둘에서는 잘라야 하고, 자르는 것은
# --allowedTools가 아니라 --disallowedTools다.
DENIED = {"grep": DENIED_BASE + "," + WIKI_TOOLS,
          "wiki-rag": DENIED_BASE + "," + WIKI_TOOLS,
          "wiki-gated": DENIED_BASE}
GREP_TOOLS = "Read,Grep,Glob,Bash"
RAG_TOOLS = ("mcp__graphin__bootstrap_workspace,mcp__graphin__search_hybrid,"
             "mcp__graphin__search_keyword,mcp__graphin__explore_graph,"
             "mcp__graphin__read_code,Read,Grep,Glob,Bash")
GATED_TOOLS = RAG_TOOLS + "," + WIKI_TOOLS

BASE_PROMPT = """You answer questions about this repository.

Ground every claim in the repository's own files. Name the file paths you relied
on — an answer without them is not an answer. When a question asks what follows
from a rule, say the conclusion outright, not just the rule.
"""

INJECT_HEADER = """
# Project knowledge

These sections were selected for this task from the project's curated knowledge
sets, served with the path each one came from.

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



GATE_HOOK = r"""#!/bin/sh
# plugin/graphin/hooks/wiki.sh와 같은 일을 한다: 20만 차단이고 나머지는 통과다.
# stdout은 버린다 — Claude Code가 훅의 stdout을 결정 문서로 파싱한다.
GRAPHIN_WS_ROOT="$(pwd)" __BIN__ wiki "$1" >/dev/null 2>&1
rc=$?
[ "$rc" = 20 ] && exit 2
exit 0
"""


def write_gate_hook(out, binary):
    p = os.path.join(out, "gate.sh")
    with open(p, "w") as f:
        f.write(GATE_HOOK.replace("__BIN__", binary))
    os.chmod(p, 0o755)
    return p


def write_containment_hook(out):
    p = os.path.join(out, "contain.sh")
    with open(p, "w") as f:
        f.write(CONTAINMENT)
    os.chmod(p, 0o755)
    return p


def load_jsonl(path):
    return [json.loads(l) for l in open(path, encoding="utf-8") if l.strip()]


def build_corpus(dest):
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


SET_RE = re.compile(r'<set name="([^"]+)"')


def select_and_resolve(binary, root, tasks, qids, index_timeout):
    """세트를 preflight가 고른다 — 사람이 고른 목록을 넘기면 매칭 품질이 측정에서 빠진다."""
    m = MCP(mcp_argv(binary, root))
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
        chosen, inj = {}, {}
        for q in qids:
            cat = m.tool("wiki_preflight", {"task": tasks[q]["question"], "role": ""})
            names = SET_RE.findall(cat)
            chosen[q] = names
            # 빈 카탈로그는 정상적인 답이다 — 그 문항의 주입은 없다.
            inj[q] = m.tool("wiki_resolve", {"sets": names}) if names else ""
        return era, chosen, inj
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


def _miss_lit(exp, field, text):
    """경로는 원문 그대로 본다 — 대소문자를 접으면 다른 파일이 히트한다."""
    return [p for p in exp.get(field, []) if not any(a in text for a in p.split("|"))]


def _miss_ev(exp, field, low):
    return [e for e in exp.get(field, []) if not any(a.lower() in low for a in e.split("|"))]


def grade(exp, final_text):
    """네 영역을 따로 낸다. 종합만 AND이고, 하나로 평균하지 않는다."""
    low = final_text.lower().replace("*", "").replace("`", "")
    mc, me = _miss_lit(exp, "code_cite", final_text), _miss_ev(exp, "code_evidence", low)
    kc, ke = _miss_lit(exp, "knowledge_cite", final_text), _miss_ev(exp, "knowledge_evidence", low)
    dv = _miss_ev(exp, "derived", low)
    fb = [f for f in exp.get("forbidden", []) if f.lower() in low]
    nav, know, ins = not mc and not me, not kc and not ke, not dv
    return {"nav": nav, "knowledge": know, "insight": ins,
            "overall": nav and know and ins and not fb,
            "miss": {"code_cite": mc, "code_evidence": me, "knowledge_cite": kc,
                     "knowledge_evidence": ke, "derived": dv, "forbidden_hit": fb}}


def wiki_calls(path):
    """게이트 팔의 관측: 위키를 몇 번 묻고, 무엇을 읽기로 골랐는가.

    압력 가설이 재는 것이 여기 있다 — 카탈로그가 비었거나 무관할 때 에이전트가
    빈 손으로 나가는지(empty resolve), 아니면 뭐라도 집어넣는지."""
    pre, res = 0, []
    for line in open(path, encoding="utf-8"):
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("type") != "assistant":
            continue
        for b in ev.get("message", {}).get("content", []):
            if b.get("type") != "tool_use":
                continue
            name = b.get("name", "")
            if "wiki_preflight" in name:
                pre += 1
            elif "wiki_resolve" in name:
                a = b.get("input", {}) or {}
                sets = list(a.get("sets") or [])
                res.append({"sets": sets, "nodes": len(a.get("node_ids") or []),
                            "empty": not sets and not a.get("node_ids")})
    return {"preflight": pre, "resolve": res}


def escape_attempts(path, root):
    """봉쇄 훅이 막은 이탈 시도를 센다.

    무효화하지 않는다 — 훅이 deny하면 코퍼스는 오염되지 않았고, 시도했다는 사실만
    남는다. 그 사실은 지표로 값이 있다(eval-scaling에서 grep 33 대 graphin 1).
    봉쇄가 없던 첫 스모크에서는 실제로 읽혔고, 그때는 무효가 맞았다."""
    out = []
    for line in open(path, encoding="utf-8"):
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("type") != "assistant":
            continue
        for b in ev.get("message", {}).get("content", []):
            if b.get("type") != "tool_use":
                continue
            blob = json.dumps(b.get("input", {}), ensure_ascii=False)
            # 앞에 경로 문자가 붙어 있으면 `docs/wiki/agents.md`의 꼬리를 절대 경로로
            # 오인한 것이다. 시스템 경로는 코퍼스 이탈이 아니다.
            for m in re.findall(r'(?<![A-Za-z0-9._/*?-])(/[A-Za-z0-9._/-]{6,})', blob):
                if m.startswith(root) or m.startswith(SYSTEM_PATHS):
                    continue
                out.append(m)
    return sorted(set(out))[:5]


def run(args):
    tasks = {t["id"]: t for t in load_jsonl(os.path.join(REPO, "eval/combined/tasks.jsonl"))}
    qids = args.tasks.split(",") if args.tasks else list(tasks)
    os.makedirs(os.path.join(args.out, "transcripts"), exist_ok=True)

    tmp = tempfile.mkdtemp(prefix="graphin-cb-")
    try:
        root = os.path.join(tmp, "corpus")
        os.makedirs(root)
        files = build_corpus(root)
        era, chosen, inj = select_and_resolve(args.bin, root, tasks, qids, args.index_timeout)
        json.dump({"chosen": chosen, "bytes": {k: len(v.encode()) for k, v in inj.items()}},
                  open(os.path.join(args.out, "injection.json"), "w"), ensure_ascii=False, indent=1)

        # 스모크에서 grep 팔이 ~/.claude/plugins/cache 안의 이 저장소 사본을 읽었다.
        # 봉쇄 없이는 독립변수가 존재하지 않는다(eval-scaling이 겪은 그대로).
        hook = write_containment_hook(args.out)
        gate = write_gate_hook(args.out, args.bin)
        contain = [{"matcher": "Bash|Read|Grep|Glob",
                    "hooks": [{"type": "command", "command": hook}]}]
        st = os.path.join(args.out, "settings.json")
        json.dump({"enabledPlugins": {}, "hooks": {"PreToolUse": contain}}, open(st, "w"))
        # 게이트 팔의 매처는 플러그인 hooks.json과 같다. Bash가 들어 있어서
        # 에이전트는 첫 셸 호출에서 막히고, 그때 게이트의 메시지를 읽는다.
        stg = os.path.join(args.out, "settings-gated.json")
        json.dump({"enabledPlugins": {}, "hooks": {
            "PreToolUse": contain + [{"matcher": "Task|Agent|Edit|MultiEdit|Write|NotebookEdit|Bash",
                                      "hooks": [{"type": "command", "command": gate + " gate"}]}],
            "PostToolUse": [{"matcher": "*",
                             "hooks": [{"type": "command", "command": gate + " mark"}]}]}},
            open(stg, "w"))
        settings = {"grep": st, "wiki-rag": st, "wiki-gated": stg}

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
                                                  "command": a[0], "args": a[1:]}}}, open(c, "w"))
            cfgs.append(c)

        arms = tuple(args.arms.split(",")) if args.arms else ARMS
        bad = set(arms) - set(ARMS)
        if bad:
            raise SystemExit(f"unknown arm(s) {sorted(bad)} — pick from {ARMS}")
        work = [(arm, q, r) for r in range(args.runs) for arm in arms for q in qids]
        rows, lk, state, total = [], threading.Lock(), {"n": 0}, len(work)

        def worker(wi):
            while True:
                with lk:
                    if not work:
                        return
                    arm, q, ri = work.pop(0)
                prompt = BASE_PROMPT
                # wiki-gated는 주입하지 않는다. 게이트가 에이전트를 위키로 보내고,
                # 무엇을 읽을지는 에이전트가 정한다 — 그 선택이 이 팔의 관측 대상이다.
                if arm == "wiki-rag" and inj[q]:
                    prompt += INJECT_HEADER + inj[q]
                pf = os.path.join(args.out, f"prompt-{arm}-{q}.md")
                open(pf, "w", encoding="utf-8").write(prompt)

                tag = f"{arm}_{q}_r{ri}"
                tp = os.path.join(args.out, "transcripts", tag + ".jsonl")
                cmd = ["claude", "-p", tasks[q]["question"],
                       "--settings", settings[arm], "--model", args.model,
                       "--system-prompt-file", pf,
                       "--output-format", "stream-json", "--verbose",
                       "--max-turns", str(args.max_turns),
                       "--disallowedTools", DENIED[arm]]
                if arm == "grep":
                    cmd += ["--allowedTools", GREP_TOOLS]
                else:
                    cmd += ["--strict-mcp-config", "--mcp-config", cfgs[wi],
                            "--allowedTools",
                            GATED_TOOLS if arm == "wiki-gated" else RAG_TOOLS]
                t0, err = time.time(), None
                try:
                    with open(tp, "w") as tf:
                        r = subprocess.run(cmd, stdout=tf, stderr=subprocess.PIPE, text=True,
                                           cwd=roots[wi], env=dict(os.environ),
                                           timeout=args.run_timeout)
                    if r.returncode != 0:
                        err = f"exit {r.returncode}: {r.stderr[-300:]}"
                except subprocess.TimeoutExpired:
                    err = f"timeout {args.run_timeout}s"
                row = {"arm": arm, "task": q, "run": ri, "transcript": tag + ".jsonl",
                       "wall_s": round(time.time() - t0, 1), "error": err,
                       "root": roots[wi], "prompt_bytes": len(prompt.encode()),
                       "sets": chosen[q] if arm == "wiki-rag" else []}
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
                "model": args.model, "runs": args.runs, "arms": list(arms), "tasks": qids,
                "semantic": False, "chosen_sets": chosen,
                "graphin_commit": subprocess.run(["git", "-C", REPO, "rev-parse", "--short", "HEAD"],
                                                 capture_output=True, text=True).stdout.strip(),
                "started": time.strftime("%Y-%m-%dT%H:%M:%S")}
        json.dump({"meta": meta, "rows": rows},
                  open(os.path.join(args.out, "summary.json"), "w"), ensure_ascii=False, indent=1)
        print(f"\n완주 — {args.out}/summary.json")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

FIELDS = (("nav", "탐색"), ("knowledge", "지식"), ("insight", "통찰"), ("overall", "종합"))


def score(args):
    s = json.load(open(os.path.join(args.out, "summary.json")))
    exp = {r["id"]: r for r in load_jsonl(os.path.join(REPO, "eval/combined/expected.jsonl"))}
    inj = json.load(open(os.path.join(args.out, "injection.json")))
    rows = []
    for row in s["rows"]:
        tp = os.path.join(args.out, "transcripts", row["transcript"])
        r = dict(row)
        if row["error"] or not os.path.exists(tp):
            r.update({k: None for k, _ in FIELDS})
            r.update(calls=None, retrieval=None, tokens_in=None, escaped=None)
        else:
            final, ledger, tin, turns = read_transcript(tp)
            r.update(grade(exp[row["task"]], final))
            r.update(calls=len(ledger), retrieval=sum(x["bytes"] for x in ledger),
                     tokens_in=tin, turns=turns,
                     escaped=escape_attempts(tp, row["root"]),
                     wiki=wiki_calls(tp))
        rows.append(r)

    def med(arm, key, pool=None):
        v = [r[key] for r in (pool or rows)
             if r["arm"] == arm and r.get(key) is not None]
        return round(statistics.median(v)) if v else None

    def tally(arm, field, pool=None):
        g = [r for r in (pool or rows) if r["arm"] == arm and r.get(field) is not None]
        return sum(1 for r in g if r[field]), len(g)

    m = s["meta"]
    print(f"\n# eval-combined — rubric {m['rubric_version']}")
    print(f"graphin {m['graphin_commit']} · {m['model']} · runs {m['runs']} · "
          f"코퍼스 {m['corpus_files']}파일 · lexical-only\n")

    print("## 종합 성적표")
    print("| 팔 | 탐색 | 지식 | 통찰 | **종합** | 툴콜 | 리트리벌 | 입력토큰 | 벽시계 |")
    print("|---|---|---|---|---|--:|--:|--:|--:|")
    for arm in m["arms"]:
        cells = []
        for f, _ in FIELDS:
            ok, n = tally(arm, f)
            cells.append(f"{ok}/{n}" if n else "—")
        print(f"| {arm} | {cells[0]} | {cells[1]} | {cells[2]} | **{cells[3]}** | "
              f"{med(arm,'calls')} | {med(arm,'retrieval')} | {med(arm,'tokens_in')} | "
              f"{med(arm,'wall_s')}s |")

    print("\n## 위키가 답을 갖는가로 갈라 본다")
    print("| 구간 | 팔 | 탐색 | 지식 | 통찰 | 종합 | 입력토큰 |")
    print("|---|---|---|---|---|---|--:|")
    for cov, label in ((True, "위키 커버"), (False, "**위키 미커버**")):
        ids = [q for q in m["tasks"] if exp[q]["wiki_covered"] is cov]
        pool = [r for r in rows if r["task"] in ids]
        for arm in m["arms"]:
            c = []
            for f, _ in FIELDS:
                ok, n = tally(arm, f, pool)
                c.append(f"{ok}/{n}" if n else "—")
            print(f"| {label} ({len(ids)}문항) | {arm} | {c[0]} | {c[1]} | {c[2]} | "
                  f"**{c[3]}** | {med(arm,'tokens_in',pool)} |")

    arms = m["arms"]
    print("\n## 문항별 — 종합 통과와 preflight가 고른 세트")
    print("| 태스크 | 위키 | " + " | ".join(arms) + " | "
          + " | ".join(a + " 토큰" for a in arms) + " | preflight 선택 |")
    print("|---|---|" + "---|" * len(arms) + "--:|" * len(arms) + "---|")
    ratios = []
    for q in m["tasks"]:
        pool = [r for r in rows if r["task"] == q]
        cell = {}
        for arm in arms:
            ok, n = tally(arm, "overall", pool)
            cell[arm] = (f"{ok}/{n}" if n else "—", med(arm, "tokens_in", pool))
        if "grep" in cell and "wiki-rag" in cell:
            g, w = cell["grep"][1], cell["wiki-rag"][1]
            if g and w:
                ratios.append(g / w)
        sets = ",".join(inj["chosen"].get(q) or []) or "—"
        print(f"| {q} | {'○' if exp[q]['wiki_covered'] else '✗'} | "
              + " | ".join(cell[a][0] for a in arms) + " | "
              + " | ".join(str(cell[a][1]) for a in arms) + f" | {sets} |")

    if ratios:
        wins = sum(1 for x in ratios if x > 1)
        n = len(ratios)
        # 실제 이항검정이다. 전에는 "전부 한 방향일 때의 값"을 찍었는데, 그건
        # 관측이 무엇이든 같은 숫자가 나오므로 검정이 아니라 상수였다.
        k = min(wins, n - wins)
        tail = sum(math.comb(n, i) for i in range(k + 1))
        pv = min(1.0, 2 * tail / (2 ** n))
        print(f"\n## 부호검정 — 입력 토큰 (문항 단위 짝 비교)")
        print(f"wiki-rag가 더 싼 문항 **{wins}/{n}** · 양측 p = **{pv:.3f}**"
              + ("  — 유의" if pv < 0.05 else "  — 유의하지 않다"))

    gated = [r for r in rows if r["arm"] == "wiki-gated" and r.get("wiki")]
    if gated:
        chosen = inj["chosen"]
        n_pre = sum(1 for r in gated if r["wiki"]["preflight"] > 0)
        calls = [c for r in gated for c in r["wiki"]["resolve"]]
        empties = sum(1 for c in calls if c["empty"])
        no_res = sum(1 for r in gated if not r["wiki"]["resolve"])
        offered, taken, extra = 0, 0, 0
        for r in gated:
            cat = set(chosen.get(r["task"]) or [])
            for c in r["wiki"]["resolve"]:
                for nm in c["sets"]:
                    taken += 1
                    if nm in cat:
                        offered += 1
                    else:
                        extra += 1
        print("\n## 게이트 팔의 행동 — 무엇을 읽기로 골랐나")
        print(f"- preflight를 부른 런 {n_pre}/{len(gated)} · resolve를 한 번도 안 부른 런 {no_res}")
        print(f"- resolve 호출 {len(calls)}건 중 **빈 호출 {empties}건**")
        print(f"- 이름 댄 세트 {taken}개 — 카탈로그에 있던 것 {offered} · 카탈로그 밖 {extra}")
        blind = [r["task"] for r in gated
                 if not (chosen.get(r["task"]) or [])
                 and any(c["sets"] for c in r["wiki"]["resolve"])]
        if blind:
            print(f"- **카탈로그가 비었는데 세트를 이름 댄 런**: {sorted(set(blind))}")

    att = {}
    for r in rows:
        if r.get("escaped"):
            att.setdefault(r["arm"], []).append(r["task"])
    if att:
        print("\n## 코퍼스 밖으로 나가려 한 런 (봉쇄 훅이 막았다 — 무효 아님)")
        for a, ts in att.items():
            print(f"- {a}: {len(ts)}런 — {', '.join(sorted(set(ts)))}")

    bad = [r for r in rows if r.get("overall") is False]
    if bad:
        print("\n## 종합에서 떨어진 런이 놓친 것")
        seen = set()
        for r in bad:
            k = (r["task"], r["arm"], json.dumps(r["miss"], sort_keys=True))
            if k in seen:
                continue
            seen.add(k)
            gaps = {kk: vv for kk, vv in r["miss"].items() if vv}
            print(f"- `{r['task']}` [{r['arm']}] {gaps}")

    json.dump({"meta": m, "rows": rows},
              open(os.path.join(args.out, "report.json"), "w"), ensure_ascii=False, indent=1)


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
            p.add_argument("--arms", help="쉼표로 구분. 생략하면 전부")
            p.add_argument("--max-turns", type=int, default=40)
            p.add_argument("--run-timeout", type=float, default=300.0)
            p.add_argument("--index-timeout", type=float, default=300.0)
    a = ap.parse_args()
    (run if a.cmd == "run" else score)(a)


if __name__ == "__main__":
    main()
