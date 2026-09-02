#!/usr/bin/env python3
"""유지보수 드릴 — 에이전트가 세트를 고치는가, 그리고 맞게 고치는가.

위키 사본을 일부러 부순 뒤 `wiki-maintainer`에게 맡기고, 결과를 **LLM 없이 파일만
보고** 채점한다. 세 종류의 부숨에는 각각 정답이 있다:

- **dangling** — 엔트리의 앵커를 깨뜨린다. 정답은 원래 노드다. 복구된 타깃이 원래
  노드와 같으면 맞고, 다른 절이면 틀리고, 그대로면 손대지 않은 것이다. 두 모드가
  있다. `--rename anchor`(기본)는 세트의 링크만 망가뜨려 헤딩이 그대로 남는 **쉬운
  모드**다 — 옛 제목으로 grep하면 찾는다. `--rename heading`은 **문서의 헤딩을 다른
  낱말로 다시 써서** 앵커가 정말로 바뀌는 어려운 모드다. 옛 제목은 어디에도 없고,
  그 절을 인용하는 다른 세트도 함께 깨지므로 정답은 그 세트들 전부에 있다. 제목은
  haiku가 바꿔 쓰고(같은 뜻, 다른 낱말), 안 되면 결정론 규칙으로 바꾼다.
- **drift** — `pins.lock`의 해시를 바꾼다. 문서는 그대로이므로 요약은 여전히 맞고,
  정답은 "다시 읽고 repin"이다. 핀이 원래 해시로 돌아오면 처리된 것이다.
- **unread** — 마찰 로그에 coverage_hit 세 건을 심는다. description이 한 줄로
  바뀌면 처리된 것이다. 좋은 줄인지는 여기서 안 잰다.

그리고 **부수 피해**를 센다: 손대면 안 되는 것 — 문서, 다른 세트, 다른 엔트리 —
이 바뀌었는지, 손댄 세트가 전부 `reviewed: false`를 달았는지, 끝난 뒤
`graphin wiki check`가 깨끗한지.

    scripts/eval-wiki-maintain.py run --out out/maint-1 --runs 1
    scripts/eval-wiki-maintain.py score --out out/maint-1
"""

import argparse
import importlib.util
import json
import os
import posixpath
import random
import re
import shutil
import subprocess
import sys
import tempfile
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RUBRIC_VERSION = "0.1.0"

AGENT_MD = os.path.join(REPO, "plugin/graphin-guide/agents/wiki-maintainer.md")
SKILL_MD = os.path.join(REPO, "plugin/graphin-guide/skills/knowledge/SKILL.md")

ALLOWED = ("mcp__graphin__bootstrap_workspace,mcp__graphin__diagnose_index,"
           "mcp__graphin__search_hybrid,mcp__graphin__search_keyword,"
           "mcp__graphin__explore_graph,mcp__graphin__read_code,"
           "mcp__graphin__wiki_preflight,mcp__graphin__wiki_resolve,"
           "mcp__graphin__wiki_edit_set,Bash,Read,Grep,Glob")
DENIED = "Edit,Write,NotebookEdit,Agent,Task,Skill,ScheduleWakeup,WebFetch,WebSearch"

TASK = ("Work this repository's wiki maintenance queue. Run `graphin wiki check` and "
        "`graphin wiki queue --json`, repair every dangling, drifted and unread item that "
        "is yours with wiki_edit_set, then run `graphin wiki check` again and report.")

ENTRY_RE = re.compile(r"^(\s*-\s+\[[^\]]+\]\()([^)\s]+)(\).*)$")
HEADING_RE = re.compile(r"^(\s{0,3}#{1,6}\s+)(.*?)\s*#*\s*$")
NUMBER_RE = re.compile(r"^(\d+(?:\.\d+)*\.?\s+)(.*)$")


def slugify(h):
    """internal/parse/markdown.go의 규칙 그대로 — 연속 하이픈도 양끝 하이픈도 안 지운다."""
    out = []
    for ch in h.lower():
        if ch.isalnum() or ch in "_-":
            out.append(ch)
        elif ch.isspace():
            out.append("-")
    return "".join(out)


def headings_of(lines):
    """(slug → 줄 번호). 펜스 안은 건너뛰고, 중복 슬러그는 파서처럼 -1, -2를 단다."""
    out, seen, fence = {}, {}, False
    for i, line in enumerate(lines):
        if line.lstrip().startswith("```") or line.lstrip().startswith("~~~"):
            fence = not fence
            continue
        if fence:
            continue
        m = HEADING_RE.match(line)
        if not m:
            continue
        sl = slugify(m.group(2)) or "section"
        n = seen.get(sl, 0)
        seen[sl] = n + 1
        out[sl if n == 0 else f"{sl}-{n}"] = i
    return out


def paraphrase(heading, settings, model="haiku"):
    """같은 뜻, 다른 낱말. LLM이 없거나 답이 이상하면 결정론 규칙으로 물러선다."""
    m = NUMBER_RE.match(heading)
    prefix, text = (m.group(1), m.group(2)) if m else ("", heading)
    empty_mcp = os.path.join(os.path.dirname(settings), "mcp-empty.json")
    if not os.path.exists(empty_mcp):
        json.dump({"mcpServers": {}}, open(empty_mcp, "w"))
    prompt = ("아래 마크다운 제목을 뜻은 같되 낱말은 다르게 한 줄로 다시 써라. 원래 제목의 "
              "낱말을 그대로 쓰지 말고, 제목 텍스트만 출력해라. 따옴표·번호·설명 없이.\n\n" + text)
    try:
        r = subprocess.run(["claude", "-p", prompt, "--model", model, "--output-format", "text",
                            "--max-turns", "1", "--settings", settings,
                            "--strict-mcp-config", "--mcp-config", empty_mcp,
                            "--disallowedTools", "*"],
                           capture_output=True, text=True, timeout=120)
        new = r.stdout.strip().strip('"\'「」').strip()
    except Exception:
        new = ""
    # 모델이 제목 대신 거절문이나 되묻는 문장을 내놓을 때가 있다 — 실제로 한 번
    # "마크다운 제목이 보이지 않습니다…"가 헤딩으로 들어갔다. 문장 꼴이면 버린다.
    refusal = any(k in new for k in ("보이지 않", "제시해", "주세요", "없습니다", "cannot", "can't", "unable"))
    if not new or "\n" in new or slugify(new) == slugify(text) or len(new) > 80 or refusal:
        new = text + " — 다시 쓴 제목"
    return prefix + new


def _load(name):
    """같은 디렉터리의 다른 러너에서 코퍼스 빌더와 트랜스크립트 리더를 빌린다."""
    spec = importlib.util.spec_from_file_location(name, os.path.join(REPO, "scripts", name + ".py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


wiki_runner = _load("eval-wiki")


def strip_frontmatter(text):
    if text.startswith("---"):
        end = text.find("\n---", 3)
        if end != -1:
            text = text[end + 4:]
    text = re.sub(r"\A\s*<!--.*?-->\s*", "", text, count=1, flags=re.S)
    return text.strip() + "\n"


def compose_prompt():
    with open(AGENT_MD, encoding="utf-8") as f:
        agent = strip_frontmatter(f.read())
    with open(SKILL_MD, encoding="utf-8") as f:
        skill = strip_frontmatter(f.read())
    return agent + "\n\n" + skill


def parse_set(path, rel):
    """세트 파일의 엔트리 (title, node_id, line_index)와 description을 읽는다."""
    lines = open(path, encoding="utf-8").read().split("\n")
    entries, desc = [], ""
    in_front, fence = False, False
    for i, line in enumerate(lines):
        if i == 0 and line == "---":
            in_front = True
            continue
        if in_front:
            if line == "---":
                in_front = False
            elif line.startswith("description:"):
                desc = line.split(":", 1)[1].strip()
            continue
        if line.lstrip().startswith("```"):
            fence = not fence
            continue
        if fence:
            continue
        m = ENTRY_RE.match(line)
        if not m:
            continue
        target = m.group(2)
        file, _, anchor = target.partition("#")
        node = posixpath.normpath(posixpath.join(posixpath.dirname(rel), file))
        if anchor:
            node += "#" + anchor
        title = re.search(r"\[([^\]]+)\]", line).group(1)
        entries.append({"title": title, "node": node, "line": i, "target": target})
    return entries, desc, lines


def sets_in(root):
    d = os.path.join(root, "docs/wiki/sets")
    out = {}
    for n in sorted(os.listdir(d)):
        if n.endswith(".md"):
            rel = "docs/wiki/sets/" + n
            out[n[:-3]] = (os.path.join(root, rel), rel)
    return out


def break_wiki(root, rng, n_dangling, n_drift, n_unread, rename="anchor", settings=None):
    """부순 것과 그 정답을 돌려준다. 한 엔트리는 한 방식으로만 부순다."""
    truth = {"dangling": [], "drift": [], "unread": []}
    pool = []
    for name, (path, rel) in sets_in(root).items():
        entries, desc, _ = parse_set(path, rel)
        for e in entries:
            if "#" in e["node"]:
                pool.append((name, e))
    rng.shuffle(pool)
    picked = pool[: n_dangling + n_drift]
    used = set()

    renamed = set()
    for name, e in picked[:n_dangling]:
        if rename == "heading":
            # 문서 쪽을 바꾼다. 같은 절을 인용하는 세트가 전부 깨지고, 정답은 새 헤딩의 id다.
            file, _, anchor = e["node"].partition("#")
            if e["node"] in renamed:
                continue
            dpath = os.path.join(root, file)
            dlines = open(dpath, encoding="utf-8").read().split("\n")
            heads = headings_of(dlines)
            if anchor not in heads:
                continue
            i = heads[anchor]
            hm = HEADING_RE.match(dlines[i])
            new_text = paraphrase(hm.group(2), settings)
            new_slug = slugify(new_text)
            if new_slug in heads:
                new_text += " 2"
                new_slug = slugify(new_text)
            dlines[i] = hm.group(1) + new_text
            open(dpath, "w", encoding="utf-8").write("\n".join(dlines))
            new_id = file + "#" + new_slug
            renamed.add(e["node"])
            for other, (opath, orel) in sets_in(root).items():
                oentries, _, _ = parse_set(opath, orel)
                for oe in oentries:
                    if oe["node"] == e["node"]:
                        truth["dangling"].append({"set": other, "title": oe["title"], "original": new_id,
                                                  "broken": e["node"], "old_heading": hm.group(2),
                                                  "new_heading": new_text})
                        used.add(other)
            continue
        path, rel = sets_in(root)[name]
        lines = open(path, encoding="utf-8").read().split("\n")
        m = ENTRY_RE.match(lines[e["line"]])
        broken = m.group(2) + "-old"
        lines[e["line"]] = m.group(1) + broken + m.group(3)
        open(path, "w", encoding="utf-8").write("\n".join(lines))
        truth["dangling"].append({"set": name, "title": e["title"], "original": e["node"],
                                  "broken": e["node"] + "-old"})
        used.add(name)

    pins_path = os.path.join(root, "docs/wiki/pins.lock")
    pins = json.load(open(pins_path, encoding="utf-8"))
    for name, e in picked[n_dangling:]:
        pin = pins["pins"].get(name, {}).get(e["node"])
        if not pin:
            continue
        truth["drift"].append({"set": name, "node": e["node"], "original_hash": pin["h"]})
        pin["h"] = "b3:" + "0" * 64
        used.add(name)
    json.dump(pins, open(pins_path, "w", encoding="utf-8"), indent=2, ensure_ascii=False)
    open(pins_path, "a").write("\n")

    # unread는 부순 적 없는 세트에서 고른다 — 한 세트에 두 신호가 겹치면 어느 쪽이
    # description을 바꾸게 했는지 알 수 없다.
    candidates = [n for n in sets_in(root) if n not in used]
    rng.shuffle(candidates)
    fpath = os.path.join(root, ".graphin/wiki/friction.jsonl")
    os.makedirs(os.path.dirname(fpath), exist_ok=True)
    with open(fpath, "a", encoding="utf-8") as f:
        for name in candidates[:n_unread]:
            _, desc, _ = parse_set(*sets_in(root)[name])
            truth["unread"].append({"set": name, "original_description": desc})
            for k in range(3):
                f.write(json.dumps({"v": 1, "ts": f"2026-09-01T0{k}:00:00Z", "kind": "coverage_hit",
                                    "task": f"drill task {k} for {name}", "sets": [name]},
                                   ensure_ascii=False) + "\n")
    return truth


def prebootstrap(binary, root, index_timeout):
    """검색이 필요하므로 에이전트가 뜨기 전에 색인을 끝내 둔다. 상태는 디스크에 남는다."""
    m = wiki_runner.MCP(wiki_runner.mcp_argv(binary, root))
    try:
        era = m.handshake()
        m.tool("bootstrap_workspace", {})
        end = time.time() + index_timeout
        while time.time() < end:
            if 'lexical_ready="true"' in m.tool("diagnose_index", {}):
                return era
            time.sleep(1.0)
        raise SystemExit("index never became ready")
    finally:
        m.close()


def run_agent(args, root, out, tag, st, mcp_cfg, prompt_path):
    env = dict(os.environ)
    env["PATH"] = os.path.dirname(os.path.abspath(args.bin)) + os.pathsep + env.get("PATH", "")
    env["GRAPHIN_WIKI_GATE"] = "off"
    tp = os.path.join(out, "transcripts", tag + ".jsonl")
    cmd = ["claude", "-p", TASK, "--settings", st, "--model", args.model,
           "--system-prompt-file", prompt_path,
           "--output-format", "stream-json", "--verbose",
           "--max-turns", str(args.max_turns),
           "--strict-mcp-config", "--mcp-config", mcp_cfg,
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
    return round(time.time() - t0, 1), err


def score_corpus(root, pristine, truth, binary):
    """파일만 본다. 정답이 있는 것은 정답과, 없는 것은 '처리됐는가'와 비교한다."""
    res = {"dangling": [], "drift": [], "unread": [], "collateral": [], "unmarked": []}
    sets = sets_in(root)
    parsed = {n: parse_set(p, r) for n, (p, r) in sets.items()}

    for t in truth["dangling"]:
        entries, _, _ = parsed[t["set"]]
        by_node = {e["node"]: e for e in entries}
        by_title = {e["title"]: e for e in entries}
        if t["broken"] in by_node:
            verdict = "untouched"
        elif t["original"] in by_node:
            verdict = "correct"
        elif t["title"] in by_title:
            verdict = "wrong:" + by_title[t["title"]]["node"]
        else:
            verdict = "removed"
        res["dangling"].append({**t, "verdict": verdict})

    pins = json.load(open(os.path.join(root, "docs/wiki/pins.lock"), encoding="utf-8"))
    for t in truth["drift"]:
        h = pins["pins"].get(t["set"], {}).get(t["node"], {}).get("h")
        res["drift"].append({**t, "verdict": "repinned" if h == t["original_hash"] else "untouched"})

    for t in truth["unread"]:
        _, desc, _ = parsed[t["set"]]
        ok = desc and desc != t["original_description"] and "\n" not in desc
        res["unread"].append({**t, "description": desc, "verdict": "described" if ok else "untouched"})

    # 손댄 세트는 전부 표시가 있어야 하고, 손대면 안 되는 것은 그대로여야 한다.
    touched = set()
    for name, (path, rel) in sets.items():
        now = open(path, encoding="utf-8").read()
        was = open(os.path.join(pristine, rel), encoding="utf-8").read()
        if now != was:
            touched.add(name)
            if "reviewed: false" not in now:
                res["unmarked"].append(name)
    expected = {t["set"] for k in ("dangling", "drift", "unread") for t in truth[k]}
    for name in sorted(touched - expected):
        res["collateral"].append("set " + name + " changed without a task")
    for dp, _, files in os.walk(os.path.join(pristine, "docs")):
        for fn in files:
            p = os.path.join(dp, fn)
            rel = os.path.relpath(p, pristine)
            if rel.startswith("docs/wiki/"):
                continue
            q = os.path.join(root, rel)
            if not os.path.exists(q) or open(q, "rb").read() != open(p, "rb").read():
                res["collateral"].append("document " + rel + " changed")

    chk = subprocess.run([binary, "wiki", "check", "--root", root], capture_output=True, text=True)
    res["check_after"] = (chk.stdout.strip().splitlines() or [""])[-1]
    res["check_rc"] = chk.returncode
    res["touched"] = sorted(touched)
    return res


def run(args):
    os.makedirs(os.path.join(args.out, "transcripts"), exist_ok=True)
    prompt_path = os.path.join(args.out, "system-prompt.md")
    open(prompt_path, "w", encoding="utf-8").write(compose_prompt())
    st = os.path.join(args.out, "settings.json")
    json.dump({"hooks": {}, "enabledPlugins": {}}, open(st, "w"))

    rows = []
    tmp = tempfile.mkdtemp(prefix="graphin-maint-")
    try:
        for ri in range(args.runs):
            root = os.path.join(tmp, f"corpus-{ri}")
            os.makedirs(root)
            files = wiki_runner.build_corpus(root)
            rng = random.Random(args.seed + ri)
            truth = break_wiki(root, rng, args.dangling, args.drift, args.unread,
                               rename=args.rename, settings=st)
            # 부순 뒤에 색인한다 — 어려운 모드는 문서를 바꾸므로 색인이 새 헤딩을 알아야 한다.
            era = prebootstrap(args.bin, root, args.index_timeout)
            # 스냅샷은 부순 다음이다. "손댔다"는 에이전트가 바꾼 것만 뜻해야 한다.
            pristine = root + "-pristine"
            shutil.copytree(os.path.join(root, "docs"), os.path.join(pristine, "docs"))
            chk = subprocess.run([args.bin, "wiki", "check", "--root", root],
                                 capture_output=True, text=True)
            check_before = chk.stdout.strip()

            cfg = os.path.join(args.out, f"mcp-{ri}.json")
            a = wiki_runner.mcp_argv(args.bin, root)
            json.dump({"mcpServers": {"graphin": {"type": "stdio", "command": a[0], "args": a[1:]}}},
                      open(cfg, "w"))
            tag = f"maint_r{ri}"
            wall, err = run_agent(args, root, args.out, tag, st, cfg, prompt_path)
            scored = score_corpus(root, pristine, truth, args.bin)
            final, ledger, tin, turns = wiki_runner.read_transcript(
                os.path.join(args.out, "transcripts", tag + ".jsonl"))
            rows.append({"run": ri, "transcript": tag + ".jsonl", "wall_s": wall, "error": err,
                         "truth": truth, "result": scored, "check_before": check_before,
                         "tokens_in": tin, "turns": turns, "calls": len(ledger),
                         "tools": sorted({x["name"] for x in ledger}), "final": final})
            print(f"  [{ri + 1}/{args.runs}] {tag} {wall}s" + (f" ERR {err[:80]}" if err else ""),
                  flush=True)
            # 고친 위키를 남긴다 — eval-wiki.py --agent-wiki가 이것을 주입한다.
            shutil.copytree(os.path.join(root, "docs/wiki"),
                            os.path.join(args.out, f"wiki-after-r{ri}"), dirs_exist_ok=True)
        meta = {"rubric_version": RUBRIC_VERSION, "era": era, "corpus_files": files,
                "model": args.model, "runs": args.runs, "seed": args.seed, "rename": args.rename,
                "broke": {"dangling": args.dangling, "drift": args.drift, "unread": args.unread},
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
    print(f"\n# eval-wiki-maintain — rubric {s['meta']['rubric_version']}")
    print(f"graphin {s['meta']['graphin_commit']} · {s['meta']['model']} · runs {s['meta']['runs']} · "
          f"부숨 {s['meta']['broke']} · 모드 {s['meta'].get('rename', 'anchor')}\n")
    print("| 런 | dangling 정답/오답/미처리 | drift 처리 | unread 처리 | 부수 피해 | 표시 누락 | check 후 | 입력토큰 | 턴 | 벽시계 |")
    print("|---|---|---|---|---|---|---|--:|--:|--:|")
    for r in s["rows"]:
        d = r["result"]["dangling"]
        c = sum(1 for x in d if x["verdict"] == "correct")
        w = sum(1 for x in d if x["verdict"].startswith("wrong") or x["verdict"] == "removed")
        u = sum(1 for x in d if x["verdict"] == "untouched")
        dr = sum(1 for x in r["result"]["drift"] if x["verdict"] == "repinned")
        un = sum(1 for x in r["result"]["unread"] if x["verdict"] == "described")
        print(f"| r{r['run']} | {c}/{w}/{u} | {dr}/{len(r['result']['drift'])} | "
              f"{un}/{len(r['result']['unread'])} | {len(r['result']['collateral'])} | "
              f"{len(r['result']['unmarked'])} | {r['result']['check_after']} | "
              f"{r['tokens_in']} | {r['turns']} | {r['wall_s']}s |"
              + (f" ERR {r['error'][:60]}" if r["error"] else ""))
    print("\n## 항목별")
    for r in s["rows"]:
        for x in r["result"]["dangling"]:
            how = f" ({x['old_heading']} → {x['new_heading']})" if x.get("new_heading") else ""
            print(f"- r{r['run']} dangling `{x['set']}` [{x['title']}] {x['original']}{how} → **{x['verdict']}**")
        print(f"- r{r['run']} 쓴 도구: {', '.join(r['tools'])}")
        for x in r["result"]["drift"]:
            print(f"- r{r['run']} drift `{x['set']}` {x['node']} → **{x['verdict']}**")
        for x in r["result"]["unread"]:
            print(f"- r{r['run']} unread `{x['set']}` → **{x['verdict']}**: {x['description']}")
        for x in r["result"]["collateral"]:
            print(f"- r{r['run']} 부수 피해: {x}")
        for x in r["result"]["unmarked"]:
            print(f"- r{r['run']} 표시 누락: {x}")


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
            p.add_argument("--seed", type=int, default=1)
            p.add_argument("--dangling", type=int, default=3)
            p.add_argument("--drift", type=int, default=2)
            p.add_argument("--unread", type=int, default=1)
            p.add_argument("--rename", choices=("anchor", "heading"), default="anchor",
                           help="dangling을 만드는 법 — anchor: 세트 링크만(쉬움), heading: 문서 헤딩을 다시 씀(어려움)")
            p.add_argument("--max-turns", type=int, default=80)
            p.add_argument("--run-timeout", type=float, default=1200.0)
            p.add_argument("--index-timeout", type=float, default=300.0)
    a = ap.parse_args()
    (run if a.cmd == "run" else score)(a)


if __name__ == "__main__":
    main()
