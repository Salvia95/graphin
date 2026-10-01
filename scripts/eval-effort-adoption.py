#!/usr/bin/env python3
"""effort·채택 벤치 러너 — docs/effort-adoption-bench-design.md.

Two experiments over the rag golden set:

  A  the graphin-rag agent, exactly as the release gate runs it, under
     different (model, effort) cells — which cell should ship?
  B  a default Claude Code session with graphin attached but not urged, next
     to two forced anchors — how often is graphin chosen, and does the choice
     pay?

The corpus, tasks and grader are scripts/eval-rag.py's, imported and never
edited: that file and eval/rag are change-controlled (docs/rag-bench-spec.md
§8). Its RUBRIC_VERSION is recorded in meta, because its grader flows through.

Subcommands:
  selftest   no LLM — neutral-contract lint, classifyBash and Tokens parity
             with the Go fixtures in internal/usage
  probe      Phase 0 — effort precedence, auth, model aliases, the B surface,
             the sandbox, the g-only hook
  run        one phase: a single seeded, shuffled queue over
             cells × arms × tasks × runs, resumable across usage limits
  score      grade, then time/token/adoption fields → report.md / report.json
"""

import argparse
import hashlib
import importlib.util
import json
import os
import random
import re
import shutil
import statistics
import subprocess
import sys
import tempfile
import threading
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def _load(name):
    spec = importlib.util.spec_from_file_location(
        name.replace("-", "_"), os.path.join(REPO, "scripts", name + ".py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


rag = _load("eval-rag")

# ---------------------------------------------------------------- cells, arms

# The design's cell table (§2). C0 is the historical condition: eval-rag never
# passed --effort. Phase 0 measured what that condition was — not the user
# settings' global xhigh (which never reached a sonnet child) but the model's
# own default, indistinguishable from it in thinking and accuracy — so C0
# passes no effort flag at all (None).
CELLS = {
    "C0": ("sonnet", None),
    "C1": ("opus", "low"),
    "C2": ("sonnet", "medium"),
    "C3": ("sonnet", "low"),
    "C4": ("opus", "medium"),
    # 2026-10-01: the alias `sonnet` moved from Sonnet 5 to Sonnet 5.5 between
    # 09-26 and 09-30 (design §6.5), silently. Cells added after that name the
    # model outright so an alias move cannot change what they measure.
    "S55-default": ("claude-sonnet-5-5", None),
    "S55-medium": ("claude-sonnet-5-5", "medium"),
    "S55-high": ("claude-sonnet-5-5", "high"),
    "S55-xhigh": ("claude-sonnet-5-5", "xhigh"),
    "S5-default": ("claude-sonnet-5", None),
}

# arm → snapshot kind. `rag` is the gate's own condition (eval-rag's cut,
# lexical index); B's graphin arms get the hybrid index, and s-only gets no
# index at all so a shell loop never stumbles into .graphin/.
ARM_KIND = {"rag": "rag", "rag-x": "rag", "mixed": "bg", "mixed-al": "bg", "g-only": "bg", "s-only": "bs"}
# mixed-al (Phase 1 redesign, owner decision 2026-09-30): mixed with the
# graphin server marked alwaysLoad, so its tools are in the prompt from the
# first turn instead of behind ToolSearch — exposure as a manipulated
# treatment, and the one lever the plugin itself controls.
EXP_ARMS = {"A": ("rag", "rag-x"), "B": ("mixed", "mixed-al", "g-only", "s-only")}
# rag-x (2026-10-01): the gate's own condition with the agent prompt swapped
# for a candidate revision (--agent-variant), run in the same shuffled queue as
# `rag` so a prompt change is measured against a same-day control. Opt-in.
EXP_DEFAULT_ARMS = {"A": ("rag",), "B": EXP_ARMS["B"]}

GRAPHIN_TOOLS = ["mcp__graphin__bootstrap_workspace", "mcp__graphin__search_hybrid",
                 "mcp__graphin__search_keyword", "mcp__graphin__explore_graph",
                 "mcp__graphin__read_code", "mcp__graphin__diagnose_index"]

# B cuts what leaves the measurement, as eval-rag does — delegation (spend
# and evidence off-ledger), writes, the network — and nothing else: the
# default session's roster is the surface under test.
B_DENIED = ["Edit", "Write", "NotebookEdit", "Agent", "Task", "WebFetch",
            "WebSearch", "ScheduleWakeup"]
B_ALLOWED = {
    "mixed": GRAPHIN_TOOLS + ["Read", "Grep", "Glob", "Bash", "Skill"],
    "mixed-al": GRAPHIN_TOOLS + ["Read", "Grep", "Glob", "Bash", "Skill"],
    "g-only": GRAPHIN_TOOLS + ["Read", "Bash", "Skill"],
    "s-only": ["Read", "Grep", "Glob", "Bash", "Skill"],
}
B_EXTRA_DENIED = {"mixed": [], "mixed-al": [], "g-only": ["Grep", "Glob"], "s-only": []}

GUIDE_PLUGIN = os.path.join(REPO, "plugin/graphin-guide")
SEMANTIC_WAIT = rag.SEMANTIC_WAIT

# Auxiliary split inside the runner only (eval/rag is change-controlled): the
# literal-lookup tasks, where grep is expected to be the right tool.
LITERAL_PREFIX = "rag-lit-"

# ---------------------------------------------------------------- B contract

# The answer contract B appends to the default system prompt. It is the grep
# arm's prompt with every retrieval instruction taken out — role, budget,
# end states, checks and report shape, in the same words — so a B run is
# graded on the same terms as the gate without being told how to search.
NEUTRAL_CONTRACT = """# Answering this question

You answer a question about a codebase by **retrieving just enough evidence and
stopping**. Your final message is the deliverable: the answer, what it rests
on, and what you did not check. The current working directory is the whole
corpus: everything you may consult is under it, and nothing outside it counts
as evidence.

## The budget is the job

Unless the caller sets one, work to roughly **40,000 bytes** of retrieved
content — the tool results you receive — and treat two thirds of that as the
point where you start closing rather than opening. Nothing keeps that total for
you; keep it yourself.

Spending is not the goal and neither is thrift. An answer that cost 3,000 bytes
and is wrong is worse than one that cost 30,000 and is right. What is
unacceptable is spending the budget and *not saying* the answer is thin.

Stop when the evidence that would settle the question is in hand — not when
the tools stop returning things. Leftover budget is not waste.

## Three states end the work early, and each has its own report

- *Answered.* You have the evidence. Stop and write it up.
- *Not here.* You looked for the thing by name and by the words around it and
  nothing in this corpus spells it. Say that plainly — it is a real answer, and
  a far more useful one than five plausible near-misses.
- *Out of reach.* The evidence needs something the corpus does not hold: runtime
  values, execution order, a live database, logs, another repository. Say what
  is missing and what you would need to answer it.

## Check these against your draft before you report

- **No match is not proof of absence.** Say what you looked for, in which
  form, before concluding that something is not here.
- **Don't report a test or a document as the implementation.** If the best
  match is a test whose name restates the question, or a design note about
  the code, say the implementation was not found rather than citing it as one.
- **DB answers are snapshot-scoped.** Schema files in the corpus describe the
  committed schema, not a live database. Say "as committed", never "in
  production".

## What you must not do

- **Do not answer from what you already know.** Recognition tells you where to
  look, and nothing more. Retrieve the lines that show it and cite them, or say
  you did not verify it.
- **Do not leave the working directory.** Absolute paths and `~` are outside
  the corpus and are not evidence.
- **Do not silently truncate.** If you stopped because the budget ran out, the
  report says so.

## Report

Structure the final message as:

1. **The answer**, in prose, first.
2. **What it rests on** — repository-relative `path:line` per claim.
3. **What you did not verify** — this section is not optional.
4. **Cost** — roughly how many bytes of tool output you received and how many
   calls it took.
"""

# A retriever named in the contract is a nudge, and a nudge is the treatment
# B must not apply. The runner refuses to start on a match.
CONTRACT_FORBIDDEN = re.compile(r"grep|\brg\b|glob|graphin|search_|explore|read_code|ripgrep", re.I)


def lint_contract():
    hits = sorted({m.group(0) for m in CONTRACT_FORBIDDEN.finditer(NEUTRAL_CONTRACT)})
    if hits:
        raise SystemExit(f"neutral contract names a retriever: {hits}")


# ---------------------------------------------------------------- usage parity

# Ported from internal/usage (ingest.go classifyBash/splitSegments/fields,
# event.go Tokens/stopwords) so this bench and the real-usage report count the
# same thing. `selftest` replays the Go test fixtures against these.
SEARCH_COMMANDS = {"grep", "rg", "egrep", "fgrep", "ag", "ack", "fd", "find"}
VALUE_FLAGS = {"-A", "-B", "-C", "-m", "-g", "--glob", "-t", "--type", "-f", "--file"}


def split_segments(cmd):
    segs = [cmd]
    for sep in ("&&", "||", "|", ";", "\n"):
        segs = [p for s in segs for p in s.split(sep)]
    return segs


def shell_fields(s):
    out, cur, quote = [], [], None
    for ch in s:
        if quote:
            if ch == quote:
                quote = None
            else:
                cur.append(ch)
        elif ch in ("'", '"'):
            quote = ch
        elif ch in (" ", "\t"):
            if cur:
                out.append("".join(cur))
                cur = []
        else:
            cur.append(ch)
    if cur:
        out.append("".join(cur))
    return out


def classify_bash(cmd):
    """(is_search, pattern) — the Go classifyBash, segment by segment."""
    for seg in split_segments(cmd):
        argv = shell_fields(seg)
        if not argv:
            continue
        prog, rest = os.path.basename(argv[0]), argv[1:]
        if prog == "git" and rest and rest[0] == "grep":
            prog, rest = "grep", rest[1:]
        if prog not in SEARCH_COMMANDS:
            continue
        if prog == "find":
            for i, a in enumerate(rest):
                if a in ("-name", "-iname", "-path") and i + 1 < len(rest):
                    return True, rest[i + 1]
            return True, ""
        i = 0
        while i < len(rest):
            a = rest[i]
            if a.startswith("-"):
                if a in ("-e", "--regexp"):
                    if i + 1 < len(rest):
                        return True, rest[i + 1]
                elif a in VALUE_FLAGS:
                    i += 1
                i += 1
                continue
            return True, a
        return True, ""
    return False, ""


STOPWORDS = {"the", "and", "for", "where", "what", "how", "code", "file", "function",
             "class", "def", "func", "implementation", "find"}


def intent_tokens(s):
    out, w = set(), []

    def flush():
        if len(w) >= 3:
            t = "".join(w).lower()
            if t not in STOPWORDS:
                out.add(t)
        w.clear()
    for ch in s:
        if "A" <= ch <= "Z":
            flush()
            w.append(ch)
        elif "a" <= ch <= "z" or "0" <= ch <= "9":
            w.append(ch)
        else:
            flush()
    flush()
    return out


_BASH_READ = re.compile(r"^\s*(cat|head|tail|sed\s+-n|less|nl|wc)\b")


def call_class(e):
    """usage-spec §4.1 classes, for one ledger entry."""
    n, inp = e["name"], e["input"] if isinstance(e["input"], dict) else {}
    if n in ("mcp__graphin__search_hybrid", "mcp__graphin__search_keyword"):
        return "g_search"
    if n == "mcp__graphin__explore_graph":
        return "g_explore"
    if n == "mcp__graphin__read_code":
        return "g_read"
    if n in ("Grep", "Glob"):
        return "search"
    if n == "Read":
        return "read"
    if n == "Bash":
        cmd = str(inp.get("command", ""))
        if classify_bash(cmd)[0]:
            return "search"
        if any(_BASH_READ.match(s) for s in split_segments(cmd)):
            return "read"
    return "other"


NAV = ("g_search", "g_explore", "g_read")


def search_pattern(e):
    inp = e["input"] if isinstance(e["input"], dict) else {}
    if e["name"] == "Grep":
        return str(inp.get("pattern", ""))
    if e["name"] == "Glob":
        return str(inp.get("pattern", ""))
    if e["name"] == "Bash":
        return classify_bash(str(inp.get("command", "")))[1]
    if e["name"] == "mcp__graphin__search_hybrid":
        return str(inp.get("query", ""))
    if e["name"] == "mcp__graphin__search_keyword":
        return str(inp.get("pattern", ""))
    return ""


# A call a hook or the permission layer refused never retrieved anything, so
# it is not a choice that landed. Phase 1 showed why this matters: g-only runs
# open with a shell grep the hook denies, and counting that attempt made every
# one of them look text-first. Denied attempts are counted on their own —
# they are the pressure toward a tool, not its use.
_DENIED = re.compile(r"hook error|Path outside the workspace under test|"
                     r"requested permissions? to use|permission.{0,40}denied", re.I)


def denied(e):
    return bool(_DENIED.search(str(e["result"])[:400]))


def adoption_metrics(ledger):
    m = {"nav_calls": 0, "search_grep": 0, "search_glob": 0, "search_bash": 0,
         "read_calls": 0, "nav_bytes": 0, "search_bytes": 0, "read_bytes": 0,
         "first_retriever": None, "toolsearch_graphin": False, "skill_graphin": False,
         "fallbacks": 0, "same_intent_fallbacks": 0, "late_switch": False,
         "fallback_pairs": [], "search_denied": 0, "nav_denied": 0}
    classes = []
    for e in ledger:
        c = call_class(e)
        if c != "other" and denied(e):
            m["search_denied" if c == "search" else "nav_denied"] += c in NAV + ("search",)
            c = "other"
        classes.append(c)
    search_before_nav = 0
    for e, c in zip(ledger, classes):
        n = e["name"]
        inp = e["input"] if isinstance(e["input"], dict) else {}
        if n == "ToolSearch" and ("graphin" in json.dumps(inp) or
                                  "mcp__graphin__" in str(e["result"])):
            m["toolsearch_graphin"] = True
        if n == "Skill" and "graphin" in str(inp.get("skill", "")):
            m["skill_graphin"] = True
        if c in NAV:
            m["nav_calls"] += 1
            m["nav_bytes"] += e["bytes"]
            if m["first_retriever"] is None:
                m["first_retriever"] = "graphin"
        elif c == "search":
            key = {"Grep": "search_grep", "Glob": "search_glob"}.get(n, "search_bash")
            m[key] += 1
            m["search_bytes"] += e["bytes"]
            if m["first_retriever"] is None:
                m["first_retriever"] = "text"
            if m["nav_calls"] == 0:
                search_before_nav += 1
        elif c == "read":
            m["read_calls"] += 1
            m["read_bytes"] += e["bytes"]
    m["late_switch"] = m["nav_calls"] > 0 and search_before_nav >= 2
    # Fallback (usage-spec §4.2): a maximal run of graphin nav calls whose
    # first non-`other` successor is a text search. Same-intent when the run's
    # last graphin search and the fallback pattern share a token.
    i = 0
    while i < len(classes):
        if classes[i] not in NAV:
            i += 1
            continue
        last_q = ""
        while i < len(classes) and classes[i] in NAV + ("other",):
            if classes[i] == "g_search":
                last_q = search_pattern(ledger[i])
            i += 1
        if i < len(classes) and classes[i] == "search":
            m["fallbacks"] += 1
            pat = search_pattern(ledger[i])
            if intent_tokens(last_q) & intent_tokens(pat):
                m["same_intent_fallbacks"] += 1
                if len(m["fallback_pairs"]) < 3:
                    m["fallback_pairs"].append([last_q[:80], pat[:80]])
    text = m["search_grep"] + m["search_glob"] + m["search_bash"]
    tot = m["nav_calls"] + text
    m["search_calls"] = text
    m["adopt_call_share"] = round(m["nav_calls"] / tot, 3) if tot else None
    btot = m["nav_bytes"] + m["search_bytes"]
    m["adopt_byte_share"] = round(m["nav_bytes"] / btot, 3) if btot else None
    m["touched"] = m["nav_calls"] > 0
    return m


# ---------------------------------------------------------------- hooks

# g-only keeps Bash (reads through the shell are not the treatment) but a
# shell text search is denied, judged by the same classifier the metrics use.
G_ONLY_HOOK = r'''#!/usr/bin/env python3
import json, os, sys
{funcs}
ev = json.load(sys.stdin)
cmd = str((ev.get("tool_input") or {{}}).get("command", ""))
if classify_bash(cmd)[0]:
    print(json.dumps({{"hookSpecificOutput": {{"hookEventName": "PreToolUse",
        "permissionDecision": "deny",
        "permissionDecisionReason": "Text search from the shell is not available in this session."}}}}))
'''


def write_g_only_hook(out):
    import inspect
    funcs = "\n".join(inspect.getsource(f) for f in (split_segments, shell_fields))
    funcs = (f"SEARCH_COMMANDS = {SEARCH_COMMANDS!r}\nVALUE_FLAGS = {VALUE_FLAGS!r}\n"
             + funcs + "\n" + inspect.getsource(classify_bash))
    p = os.path.join(out, "g-only-hook.py")
    with open(p, "w") as f:
        f.write(G_ONLY_HOOK.format(funcs=funcs))
    os.chmod(p, 0o755)
    return p


# ---------------------------------------------------------------- environment

def child_env():
    # Same rule as eval-rag: the child must not inherit this session's plumbing.
    return {k: v for k, v in os.environ.items()
            if not k.startswith(("CLAUDE_CODE_", "CLAUDECODE"))}


class Env:
    """Snapshots, settings, MCP configs and prompts for a set of arms.
    Each worker gets its own copy of every snapshot kind it may need: two
    servers on one workspace fight over the lock."""

    def __init__(self, out, arms, jobs, args):
        self.out, self.arms, self.jobs, self.args = out, arms, jobs, args
        self.kinds = sorted({ARM_KIND[a] for a in arms})
        self.snaps = {}      # kind -> [path per worker]
        self.files = {}      # kind -> set(rel paths)
        self.cfgs = {}       # kind -> [mcp config path per worker]
        self.cfgs_al = []    # bg only: the same server marked alwaysLoad
        self.settings = {}   # arm -> settings path
        self.origin = None

    def build(self):
        args = self.args
        for kind in self.kinds:
            snap = tempfile.mkdtemp(prefix=f"graphin-ea-{kind}-")
            self.snaps[kind] = [snap]
            origin, files = rag.materialize(snap, args.ref, args.worktree)
            self.origin = origin
            if kind in ("bg", "bs"):
                # The default system prompt must not read the repository's
                # own hooks and agent roster (eval-scaling cuts it the same way).
                shutil.rmtree(os.path.join(snap, ".claude"), ignore_errors=True)
                files = [f for f in files if not f.startswith(".claude/")]
            self.files[kind] = set(files)
            with open(os.path.join(self.out, f"files-{kind}.txt"), "w") as f:
                f.write("\n".join(sorted(files)))
            if kind in ("rag", "bg"):
                semantic = kind == "bg"
                print(f"indexing {kind} snapshot ({origin}, "
                      f"{'hybrid' if semantic else 'lexical'}) …", flush=True)
                rag.preindex(args.bin, snap, semantic, args.index_timeout)
            for i in range(1, self.jobs):
                s2 = f"{snap}-w{i}"
                shutil.copytree(snap, s2)
                self.snaps[kind].append(s2)
            if kind in ("rag", "bg"):
                self.cfgs[kind] = []
                for i, s in enumerate(self.snaps[kind]):
                    argv = [args.bin, "--workspace", s, "--offline"]
                    argv += (["--semantic-wait", SEMANTIC_WAIT] if kind == "bg"
                             else ["--ort-lib", "/nonexistent-ort"])
                    p = os.path.join(self.out, f"mcp-{kind}-{i}.json")
                    with open(p, "w") as f:
                        json.dump({"mcpServers": {"graphin": {
                            "type": "stdio", "command": argv[0], "args": argv[1:]}}}, f)
                    self.cfgs[kind].append(p)
                    if kind == "bg":
                        pa = os.path.join(self.out, f"mcp-bg-al-{i}.json")
                        with open(pa, "w") as f:
                            json.dump({"mcpServers": {"graphin": {
                                "type": "stdio", "command": argv[0], "args": argv[1:],
                                "alwaysLoad": True}}}, f)
                        self.cfgs_al.append(pa)

        contain = rag.write_containment_hook(self.out)
        g_hook = write_g_only_hook(self.out) if "g-only" in self.arms else None
        for arm in self.arms:
            s = {"enabledPlugins": {"graphin@graphin": False,
                                    "graphin-guide@graphin": False},
                 "permissions": {"blockReadsOutsideWorkingDirectories": True},
                 "hooks": {"PreToolUse": [
                     {"matcher": "Bash|Read|Grep|Glob",
                      "hooks": [{"type": "command", "command": contain}]}]}}
            if ARM_KIND[arm] != "rag":
                # Every B arm grants Bash, and the repository is public: an
                # open network is a `git clone` away from the answer files.
                s["sandbox"] = {"enabled": True, "failIfUnavailable": True,
                                "network": {"allowedDomains": []}}
            if arm == "g-only":
                s["hooks"]["PreToolUse"].append(
                    {"matcher": "Bash", "hooks": [{"type": "command", "command": g_hook}]})
            p = os.path.join(self.out, f"settings-{arm}.json")
            with open(p, "w") as f:
                json.dump(s, f)
            self.settings[arm] = p

        self.rag_prompt = os.path.join(self.out, "system-prompt-rag.md")
        with open(self.rag_prompt, "w", encoding="utf-8") as f:
            f.write(rag.compose_prompt("graphin"))
        if "rag-x" in self.arms:
            with open(self.args.agent_variant, encoding="utf-8") as f:
                agent = rag.strip_frontmatter(f.read())
            with open(rag.SKILL_MD, encoding="utf-8") as f:
                skill = rag.strip_frontmatter(f.read())
            self.rag_x_prompt = os.path.join(self.out, "system-prompt-rag-x.md")
            with open(self.rag_x_prompt, "w", encoding="utf-8") as f:
                f.write(agent + "\n\n" + skill)
        self.contract = os.path.join(self.out, "contract.md")
        with open(self.contract, "w", encoding="utf-8") as f:
            f.write(NEUTRAL_CONTRACT)

    def cmd(self, cell, arm, question, wi):
        model, effort = CELLS[cell]
        kind = ARM_KIND[arm]
        c = ["claude", "-p", question,
             "--settings", self.settings[arm], "--strict-mcp-config",
             # User settings carry effortLevel and modelSettings; the cell's
             # effort must be the flag's alone (design §2).
             "--setting-sources", "project,local",
             "--model", model,
             "--output-format", "stream-json", "--verbose",
             "--max-turns", str(self.args.max_turns)]
        if effort:
            c += ["--effort", effort]
        if kind == "rag":
            c += ["--mcp-config", self.cfgs[kind][wi],
                  "--system-prompt-file",
                  self.rag_x_prompt if arm == "rag-x" else self.rag_prompt,
                  "--allowedTools", ",".join(rag.ALLOWED_TOOLS),
                  "--disallowedTools", rag.DENIED_TOOLS]
            return c
        if arm in ("mixed", "mixed-al", "g-only"):
            cfg = self.cfgs_al[wi] if arm == "mixed-al" else self.cfgs[kind][wi]
            c += ["--mcp-config", cfg, "--plugin-dir", GUIDE_PLUGIN]
        # The documented flag takes the text; the -file variant is not in
        # --help, and a runner must not lean on an undocumented flag.
        c += ["--append-system-prompt", NEUTRAL_CONTRACT,
              "--allowedTools", ",".join(B_ALLOWED[arm]),
              "--disallowedTools", ",".join(B_DENIED + B_EXTRA_DENIED[arm])]
        return c

    def cwd(self, arm, wi):
        return self.snaps[ARM_KIND[arm]][wi]

    def cleanup(self):
        for paths in self.snaps.values():
            for s in paths:
                shutil.rmtree(s, ignore_errors=True)


def question_of(t):
    q = t["question"]
    if t.get("budget_bytes"):
        q += f"\n\nWork to a retrieved-content budget of about {t['budget_bytes']} bytes."
    return q


def run_claude(cmd, cwd, tr_path, timeout):
    t0 = time.time()
    err = None
    try:
        with open(tr_path, "w") as tf:
            r = subprocess.run(cmd, stdout=tf, stderr=subprocess.PIPE, text=True,
                               cwd=cwd, env=child_env(), timeout=timeout)
        if r.returncode != 0:
            err = f"exit {r.returncode}: {r.stderr[-400:]}"
    except subprocess.TimeoutExpired:
        err = f"timeout {timeout}s"
    return round(time.time() - t0, 1), err


# ---------------------------------------------------------------- transcript

def parse_extra(path):
    """What eval-rag's parser drops: the init event and the result event's
    timing and per-model usage."""
    x = {"init": None, "duration_ms": None, "duration_api_ms": None, "ttft_ms": None,
         "tokens_in": 0, "tokens_out": 0, "thinking_tokens": 0, "cost_usd": 0.0,
         "models": [], "permission_denials": 0}
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            if ev.get("type") == "system" and ev.get("subtype") == "init":
                x["init"] = ev
            elif ev.get("type") == "result":
                for k in ("duration_ms", "duration_api_ms", "ttft_ms"):
                    x[k] = ev.get(k)
                x["permission_denials"] = len(ev.get("permission_denials") or [])
                for name, u in (ev.get("modelUsage") or {}).items():
                    # input alone reads a 250K-token run as 21 (scaling bench).
                    x["tokens_in"] += (u.get("inputTokens", 0) + u.get("cacheReadInputTokens", 0)
                                       + u.get("cacheCreationInputTokens", 0))
                    x["tokens_out"] += u.get("outputTokens", 0)
                    x["thinking_tokens"] += u.get("thinkingTokens", 0) or 0
                    x["cost_usd"] += u.get("costUSD", 0) or 0
                    x["models"].append(u.get("canonicalModel") or name)
    x["cost_usd"] = round(x["cost_usd"], 5)
    return x


# ---------------------------------------------------------------- run

def plan(args, tasks):
    arms = EXP_DEFAULT_ARMS[args.exp] if not args.arms else tuple(args.arms.split(","))
    if "rag-x" in arms and not getattr(args, "agent_variant", ""):
        raise SystemExit("arm rag-x needs --agent-variant <agent .md>")
    bad = [a for a in arms if a not in EXP_ARMS[args.exp]]
    if bad:
        raise SystemExit(f"arms {bad} do not belong to experiment {args.exp}")
    cells = tuple(args.cells.split(","))
    bad = [c for c in cells if c not in CELLS]
    if bad:
        raise SystemExit(f"unknown cells {bad} — pick from {sorted(CELLS)}")
    work = []
    for cell in cells:
        for arm in arms:
            n = args.runs if ARM_KIND[arm] == "rag" else (
                args.mixed_runs if arm in ("mixed", "mixed-al") else args.anchor_runs)
            for r in range(n):
                for t in tasks:
                    work.append((cell, arm, t["id"], r))
    # One shuffled queue for the whole phase: cells run interleaved, so API
    # latency by time of day cannot pose as a cell difference, and a phase cut
    # by usage limits leaves every cell equally covered.
    random.Random(args.seed).shuffle(work)
    return cells, arms, work


def select_tasks(args):
    tasks, _ = rag.load_set()
    if args.subset == "smoke":
        by = {t["id"]: t for t in tasks}
        tasks = [by[i] for i in rag.SMOKE_TASKS]
    if args.tasks:
        want = args.tasks.split(",")
        by = {t["id"]: t for t in tasks}
        missing = [i for i in want if i not in by]
        if missing:
            raise SystemExit(f"unknown tasks {missing}")
        tasks = [by[i] for i in want]
    return tasks


def fingerprint(args, cells, arms, tasks, commit):
    return {
        "exp": args.exp, "cells": {c: CELLS[c] for c in cells}, "arms": list(arms),
        "tasks": [t["id"] for t in tasks], "seed": args.seed,
        "runs": args.runs, "mixed_runs": args.mixed_runs, "anchor_runs": args.anchor_runs,
        "graphin_commit": commit, "ref": args.ref, "worktree": args.worktree,
        "agent_sha": rag.sha256_file(rag.AGENT_MD), "skill_sha": rag.sha256_file(rag.SKILL_MD),
        "contract_sha": hashlib.sha256(NEUTRAL_CONTRACT.encode()).hexdigest(),
        "taskset_sha": hashlib.sha256(
            open(os.path.join(REPO, "eval/rag/tasks.jsonl"), "rb").read() +
            open(os.path.join(REPO, "eval/rag/expected.jsonl"), "rb").read()).hexdigest(),
        "rag_rubric_version": rag.RUBRIC_VERSION,
        "agent_variant_sha": rag.sha256_file(args.agent_variant)
        if getattr(args, "agent_variant", "") else None,
        "max_turns": args.max_turns,
    }


def run(args):
    if args.detach and not os.environ.get("EA_RUN_CHILD"):
        os.makedirs(args.out, exist_ok=True)
        log = open(os.path.join(args.out, "run.log"), "a")
        argv = [sys.executable, os.path.abspath(__file__)] + \
            [a for a in sys.argv[1:] if a != "--detach"]
        p = subprocess.Popen(["setsid", "nohup"] + argv, stdout=log, stderr=log,
                             env=dict(os.environ, EA_RUN_CHILD="1"), start_new_session=True)
        with open(os.path.join(args.out, "run.pid"), "w") as f:
            f.write(str(p.pid))
        print(f"detached pid {p.pid} → {args.out}/run.log")
        return 0

    lint_contract()
    tasks = select_tasks(args)
    cells, arms, work = plan(args, tasks)
    if not os.access(args.bin, os.X_OK):
        raise SystemExit(f"no graphin binary at {args.bin} — run `make build` or pass --bin")
    out = args.out
    if os.path.exists(os.path.join(out, "summary.json")):
        raise SystemExit(f"{out} already holds a finished phase — pick a fresh --out")
    commit = subprocess.run(["git", "-C", REPO, "rev-parse", "--short", args.ref],
                            capture_output=True, text=True, check=True).stdout.strip()
    # Round-trip through JSON so tuples compare equal to the lists meta.json
    # stores — the first resume refused an unchanged phase over exactly that.
    fp = json.loads(json.dumps(fingerprint(args, cells, arms, tasks, commit)))
    meta_path = os.path.join(out, "meta.json")
    runs_path = os.path.join(out, "runs.jsonl")
    done = set()
    if os.path.exists(meta_path):
        if not args.resume:
            raise SystemExit(f"{out} holds a partial phase — pass --resume to continue it, "
                             "or pick a fresh --out")
        old = json.load(open(meta_path))
        diff = [k for k in fp if old.get("fingerprint", {}).get(k) != fp[k]]
        if diff:
            raise SystemExit(f"--resume refused: the phase changed in {diff}")
        if os.path.exists(runs_path):
            last = {}
            for r in rag.load_jsonl(runs_path):
                last[r["tag"]] = r
            done = {t for t, r in last.items() if not r.get("error")}
    os.makedirs(os.path.join(out, "transcripts"), exist_ok=True)

    work = [w for w in work if f"{w[0]}_{w[1]}_{w[2]}_r{w[3]}" not in done]
    total = len(work)
    jobs = max(1, min(args.jobs, total))
    by_id = {t["id"]: t for t in tasks}
    print(f"{len(done)} done, {total} to run over {jobs} worker(s)", flush=True)
    if not total:
        return finish(out, meta_path)

    env = Env(out, arms, jobs, args)
    try:
        env.build()
        warm_up(env, cells, arms, jobs, out, args)
        meta = {"fingerprint": fp, "corpus": env.origin,
                "cli_version": subprocess.run(["claude", "--version"], capture_output=True,
                                              text=True).stdout.strip(),
                "worktree_dirty": subprocess.run(
                    ["git", "-C", REPO, "status", "--porcelain"],
                    capture_output=True, text=True).stdout.strip() != "",
                "semantic_wait": SEMANTIC_WAIT, "jobs": jobs,
                "setting_sources": "project,local",
                "started": json.load(open(meta_path))["started"]
                if os.path.exists(meta_path) else time.strftime("%Y-%m-%dT%H:%M:%S"),
                "resumed": time.strftime("%Y-%m-%dT%H:%M:%S") if done else None}
        with open(meta_path, "w") as f:
            json.dump(meta, f, indent=2)

        lk = threading.Lock()
        abort = threading.Event()
        state = {"n": 0, "errors": 0}

        def worker(wi):
            while not abort.is_set():
                with lk:
                    if not work:
                        return
                    cell, arm, tid, r = work.pop(0)
                tag = f"{cell}_{arm}_{tid}_r{r}"
                tr = os.path.join(out, "transcripts", tag + ".jsonl")
                start = time.strftime("%Y-%m-%dT%H:%M:%S")
                wall, err = run_claude(env.cmd(cell, arm, question_of(by_id[tid]), wi),
                                       env.cwd(arm, wi), tr, args.run_timeout)
                row = {"tag": tag, "cell": cell, "arm": arm, "task": tid, "run": r,
                       "worker": wi, "start_ts": start, "wall_s": wall, "error": err}
                with lk:
                    with open(runs_path, "a") as f:
                        f.write(json.dumps(row) + "\n")
                    state["n"] += 1
                    state["errors"] += bool(err)
                    print(f"[{state['n']}/{total}] {tag} {wall}s"
                          + (f" ERROR {err}" if err else ""), flush=True)
                if err and rag._hit_limit(tr):
                    abort.set()
                    print("aborting: usage limit (HTTP 429). Rerun the same command with "
                          "--resume after the limit resets.", flush=True)

        threads = [threading.Thread(target=worker, args=(i,)) for i in range(jobs)]
        for th in threads:
            th.start()
        for th in threads:
            th.join()
        if abort.is_set():
            return 1
    finally:
        env.cleanup()
    return finish(out, meta_path)


def warm_up(env, cells, arms, jobs, out, args):
    """One unrecorded call per (model, arm, worker) before the queue starts.
    The prompt cache is keyed by the system prompt, which carries each
    worker's snapshot path, so every (model, arm, worker) pays a ~25–30K-token
    cache write on its first call — Phase 1 put all of it on whichever cell
    the shuffle drew first (C0's first three runs cost 1.5–2x its later ones).
    Runs again on every resume: a usage-limit pause outlives the cache."""
    seen = set()
    todo = []
    for cell in cells:
        for arm in arms:
            key = (CELLS[cell][0], arm)
            if key not in seen:
                seen.add(key)
                todo += [(cell, arm, wi) for wi in range(jobs)]
    wdir = os.path.join(out, "warmup")
    os.makedirs(wdir, exist_ok=True)
    print(f"warming the prompt cache: {len(todo)} call(s)", flush=True)

    def one(item):
        cell, arm, wi = item
        cmd = env.cmd(cell, arm, "Reply with the single word OK.", wi)
        cmd[cmd.index("--max-turns") + 1] = "1"
        tag = f"{CELLS[cell][0]}_{arm}_w{wi}_{time.strftime('%H%M%S')}"
        run_claude(cmd, env.cwd(arm, wi), os.path.join(wdir, tag + ".jsonl"), 300)
    threads = []
    for item in todo:
        th = threading.Thread(target=one, args=(item,))
        th.start()
        threads.append(th)
        if len(threads) >= jobs:
            for t in threads:
                t.join()
            threads = []
    for t in threads:
        t.join()


def finish(out, meta_path):
    meta = json.load(open(meta_path))
    meta["finished"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    with open(os.path.join(out, "summary.json"), "w") as f:
        json.dump(meta, f, indent=2)
    print(f"phase complete → {out} (score it next)")
    return 0


# ---------------------------------------------------------------- score

def med(xs):
    xs = [x for x in xs if x is not None]
    return statistics.median(xs) if xs else None


def fmt(x, nd=0):
    if x is None:
        return "—"
    return f"{x:,.{nd}f}"


def score(args):
    out = args.out
    if not os.path.exists(os.path.join(out, "summary.json")):
        raise SystemExit(f"{out} has no summary.json — the phase did not finish "
                         "(resume it; partial phases are not scored)")
    meta = json.load(open(os.path.join(out, "meta.json")))
    fp = meta["fingerprint"]
    if fp["rag_rubric_version"] != rag.RUBRIC_VERSION:
        raise SystemExit(f"recorded under eval-rag rubric {fp['rag_rubric_version']}, "
                         f"grader is now {rag.RUBRIC_VERSION} — not comparable")
    tasks, expected = rag.load_set()
    tasks = {t["id"]: t for t in tasks}
    files = {k: set(open(os.path.join(out, f"files-{k}.txt")).read().splitlines())
             for k in ("rag", "bg", "bs") if os.path.exists(os.path.join(out, f"files-{k}.txt"))}

    last = {}
    for r in rag.load_jsonl(os.path.join(out, "runs.jsonl")):
        last[r["tag"]] = r
    rows = []
    for r in last.values():
        row = {k: r[k] for k in ("tag", "cell", "arm", "task", "run", "start_ts", "wall_s")}
        row["tier"] = tasks[r["task"]]["tier"]
        row["literal"] = r["task"].startswith(LITERAL_PREFIX)
        if r.get("error"):
            row.update(verdict="error", error=r["error"])
            rows.append(row)
            continue
        tr = os.path.join(out, "transcripts", r["tag"] + ".jsonl")
        final, ledger, _, turns = rag.parse_transcript(tr)
        if final is None:
            row.update(verdict="error", error="no result event")
            rows.append(row)
            continue
        m = rag.behavior_metrics(ledger)
        seen = "".join(str(e["result"]) for e in ledger)
        g = rag.grade(tasks[r["task"]], expected[r["task"]], final, m,
                      files[ARM_KIND[r["arm"]]], seen)
        row.update({k: g[k] for k in ("verdict", "budget_state", "fake_citations",
                                      "elided_citations", "escaped", "invented_miss",
                                      "must_cite_miss", "evidence_miss")})
        row.update({"calls": m["calls"], "bytes_total": m["bytes_total"],
                    "contained": m["contained"], "turns": turns})
        x = parse_extra(tr)
        init = x.pop("init") or {}
        row.update(x)
        row["init_plugins"] = [p.get("name") for p in init.get("plugins", [])]
        row["init_mcp"] = [(s.get("name"), s.get("status")) for s in init.get("mcp_servers", [])]
        row.update(adoption_metrics(ledger))
        rows.append(row)
    rows.sort(key=lambda r: r["tag"])

    cells = list(fp["cells"])
    arms = fp["arms"]
    L = [f"# effort·채택 벤치 — 실험 {fp['exp']}", "",
         f"graphin {fp['graphin_commit']}" + (" (dirty)" if meta.get("worktree_dirty") else "")
         + f" · corpus {meta['corpus']} · eval-rag rubric {fp['rag_rubric_version']}"
         f" · cli {meta['cli_version']} · seed {fp['seed']} · jobs {meta['jobs']}",
         f"agent {fp['agent_sha'][:12]} · skill {fp['skill_sha'][:12]} · "
         f"contract {fp['contract_sha'][:12]} · taskset {fp['taskset_sha'][:12]}", ""]

    def group(cell, arm):
        return [r for r in rows if r["cell"] == cell and r["arm"] == arm]

    L += ["## 셀 × 팔", "",
          "| 셀 | 모델·effort | 팔 | pass | error | cost 중앙 | cost 합 | tokens_in 중앙 | "
          "out 중앙 | thinking 중앙 | duration 중앙 s | api 중앙 s | wall 중앙 s |",
          "|---|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|"]
    for c in cells:
        for a in arms:
            g = group(c, a)
            if not g:
                continue
            ok = [r for r in g if r.get("verdict") != "error"]
            npass = sum(r.get("verdict") == "pass" for r in g)
            mdl, eff = fp["cells"][c]
            eff = eff or "default"
            dur = med([r.get("duration_ms") for r in ok])
            api = med([r.get("duration_api_ms") for r in ok])
            L.append(f"| {c} | {mdl} {eff} | {a} | {npass}/{len(g)} | {len(g) - len(ok)} | "
                     f"{fmt(med([r['cost_usd'] for r in ok]), 3)} | "
                     f"{fmt(sum(r['cost_usd'] for r in ok), 2)} | "
                     f"{fmt(med([r['tokens_in'] for r in ok]))} | "
                     f"{fmt(med([r['tokens_out'] for r in ok]))} | "
                     f"{fmt(med([r['thinking_tokens'] for r in ok]))} | "
                     f"{fmt(dur / 1000 if dur else None, 1)} | "
                     f"{fmt(api / 1000 if api else None, 1)} | "
                     f"{fmt(med([r['wall_s'] for r in g]), 1)} |")
    L.append("")

    tiers = [t for t in rag.TIERS if any(r["tier"] == t for r in rows)]
    L += ["## 층별 pass", "",
          "| 셀 | 팔 | " + " | ".join(tiers) + " | 리터럴 계열 |",
          "|---|---|" + "--:|" * (len(tiers) + 1)]
    for c in cells:
        for a in arms:
            g = group(c, a)
            if not g:
                continue
            cells_txt = []
            for t in tiers:
                gt = [r for r in g if r["tier"] == t]
                cells_txt.append(f"{sum(r.get('verdict') == 'pass' for r in gt)}/{len(gt)}")
            gl = [r for r in g if r["literal"]]
            cells_txt.append(f"{sum(r.get('verdict') == 'pass' for r in gl)}/{len(gl)}" if gl else "—")
            L.append(f"| {c} | {a} | " + " | ".join(cells_txt) + " |")
    L.append("")

    L += ["## 채택", "",
          "| 셀 | 팔 | touched | 콜 비율 중앙 | 바이트 비율 중앙 | 첫 검색기 graphin/text | "
          "ToolSearch→graphin | 가이드 스킬 | 폴백(same-intent) | 늦은 전환 | 거부된 text 검색 |",
          "|---|---|--:|--:|--:|---|--:|--:|---|--:|--:|"]
    for c in cells:
        for a in arms:
            g = [r for r in group(c, a) if r.get("verdict") != "error"]
            if not g:
                continue
            n = len(g)
            fr = [r["first_retriever"] for r in g]
            L.append(f"| {c} | {a} | {sum(r['touched'] for r in g)}/{n} | "
                     f"{fmt(med([r['adopt_call_share'] for r in g]), 2)} | "
                     f"{fmt(med([r['adopt_byte_share'] for r in g]), 2)} | "
                     f"{fr.count('graphin')}/{fr.count('text')} | "
                     f"{sum(r['toolsearch_graphin'] for r in g)}/{n} | "
                     f"{sum(r['skill_graphin'] for r in g)}/{n} | "
                     f"{sum(r['fallbacks'] for r in g)}({sum(r['same_intent_fallbacks'] for r in g)}) | "
                     f"{sum(r['late_switch'] for r in g)}/{n} | "
                     f"{sum(r.get('search_denied', 0) for r in g)} |")
    L.append("")

    for mx in [a for a in ("mixed", "mixed-al") if a in arms]:
        L += [f"## 채택률 구간 — {mx} 팔", "",
              "> **관찰이지 인과 아님.** graphin이 실패해 grep으로 폴백한 런은 grep 비중이 높고 "
              "실패도 한다. 결론은 선택 후회와 태스크 안 비교(설계 §4.4 1·2)에서 낸다.", "",
              "| 셀 | 구간 | 런 | pass | cost 중앙 | tokens_in 중앙 | duration 중앙 s |",
              "|---|---|--:|--:|--:|--:|--:|"]
        buckets = [("0", lambda s: s == 0), ("(0, .5)", lambda s: 0 < s < 0.5),
                   ("[.5, 1)", lambda s: 0.5 <= s < 1), ("1", lambda s: s == 1),
                   ("검색 없음", None)]
        for c in cells:
            g = [r for r in group(c, mx) if r.get("verdict") != "error"]
            for name, pred in buckets:
                if pred is None:
                    gb = [r for r in g if r["adopt_call_share"] is None]
                else:
                    gb = [r for r in g if r["adopt_call_share"] is not None
                          and pred(r["adopt_call_share"])]
                if not gb:
                    continue
                dur = med([r["duration_ms"] for r in gb])
                L.append(f"| {c} | {name} | {len(gb)} | "
                         f"{sum(r['verdict'] == 'pass' for r in gb)}/{len(gb)} | "
                         f"{fmt(med([r['cost_usd'] for r in gb]), 3)} | "
                         f"{fmt(med([r['tokens_in'] for r in gb]))} | "
                         f"{fmt(dur / 1000 if dur else None, 1)} |")
        L.append("")

    L += ["## 환경 검증", ""]
    models = {}
    for r in rows:
        for mname in r.get("models", []):
            models.setdefault(r["cell"], set()).add(mname)
    for c in cells:
        L.append(f"- {c} {fp['cells'][c]}: 실제 모델 {sorted(models.get(c, []))}")
    plugs = sorted({(r["arm"], tuple(r.get("init_plugins", []))) for r in rows if "init_plugins" in r})
    for arm, p in plugs:
        L.append(f"- {arm} 로드된 플러그인: {list(p) or '없음'}")
    mcp = sorted({(r["arm"], tuple(map(tuple, r.get("init_mcp", [])))) for r in rows if "init_mcp" in r})
    for arm, s in mcp:
        L.append(f"- {arm} MCP: {list(s) or '없음'}")
    for key, label in (("escaped", "스냅샷 이탈(escaped)"), ("invented_miss", "빗나간 지어낸 id"),
                       ("fake_citations", "가짜 인용"), ("elided_citations", "말줄임 경로(모양 실패)")):
        hit = [r["tag"] for r in rows if r.get(key)]
        L.append(f"- {label}: {len(hit)}" + (f" — {hit[:6]}" if hit else ""))
    cont = sum(r.get("contained", 0) for r in rows)
    L.append(f"- containment 훅 거부: {cont} · 권한 거부(result) 합: "
             f"{sum(r.get('permission_denials', 0) for r in rows)}")
    errs = [(r["tag"], r["error"][:80]) for r in rows if r.get("verdict") == "error"]
    if errs:
        L.append(f"- error 런: {errs[:10]}")

    report = "\n".join(L) + "\n"
    with open(os.path.join(out, "report.md"), "w", encoding="utf-8") as f:
        f.write(report)
    with open(os.path.join(out, "report.json"), "w", encoding="utf-8") as f:
        json.dump({"meta": meta, "runs": rows}, f, ensure_ascii=False, indent=2)
    print(report)
    print(f"→ {out}/report.md · {out}/report.json")
    return 0


# ---------------------------------------------------------------- analyze

# Fixed before any run is seen (design §3, owner decision 2026-09-30).
NONINFERIOR_FLOOR = 0.80
NONINFERIOR_MARGIN = -0.05
BOOT = 10000
HONESTY_TIERS = ("not-here", "out-of-reach")


def _is_pass(r):
    return r.get("verdict") == "pass"


def _task_rates(rows):
    by = {}
    for r in rows:
        by.setdefault(r["task"], []).append(_is_pass(r))
    return {t: sum(v) / len(v) for t, v in by.items()}


def boot_ci(tasks, stat, seed=7):
    """95% percentile interval of `stat(sample_of_tasks)`, resampling tasks —
    runs of one task are not independent, so the task is the unit."""
    rng = random.Random(seed)
    vals = []
    for _ in range(BOOT):
        s = [rng.choice(tasks) for _ in tasks]
        v = stat(s)
        if v is not None:
            vals.append(v)
    vals.sort()
    if not vals:
        return None, None
    return vals[int(0.025 * len(vals))], vals[int(0.975 * len(vals)) - 1]


def honesty_breaks(rows):
    return sorted((r["task"], r["tag"]) for r in rows if r["tier"] in HONESTY_TIERS and
                  (r.get("fake_citations") or r.get("escaped") or r.get("invented_miss")))


def analyze_a(rows, cells, L):
    base = "C0"
    if base not in cells:
        L.append("C0이 없어 비열등 판정을 하지 않는다.")
        return
    g = {c: [r for r in rows if r["cell"] == c] for c in cells}
    rate = {c: sum(map(_is_pass, g[c])) / len(g[c]) for c in cells if g[c]}
    tr = {c: _task_rates(g[c]) for c in cells}
    cost = {c: {} for c in cells}
    dur = {c: {} for c in cells}
    for c in cells:
        for r in g[c]:
            if r.get("verdict") != "error":
                cost[c].setdefault(r["task"], []).append(r["cost_usd"])
                dur[c].setdefault(r["task"], []).append(r["duration_ms"])
    base_honest = {task for task, _ in honesty_breaks(g[base])}
    L += ["## 판정 — 실험 A (설계 §3, 런 전에 고정)", "",
          f"비열등: pass ≥ {NONINFERIOR_FLOOR:.0%} 그리고 C0 대비 차이의 95% 하한 > "
          f"{NONINFERIOR_MARGIN:+.0%}p (태스크 단위 부트스트랩 {BOOT:,}회). "
          "not-here·out-of-reach에서 C0에 없던 가짜 인용·escaped·빗나간 지어낸 id가 나오면 탈락.", "",
          "| 셀 | pass | C0 대비 | 95% CI | 정직성 | 비열등 | cost 중앙 | C0 대비 cost 95% CI | duration 중앙 s |",
          "|---|--:|--:|---|---|---|--:|---|--:|"]
    verdict = {}
    for c in cells:
        if not g[c]:
            continue
        tasks = sorted(set(tr[c]) & set(tr[base]))
        d = sum(tr[c][t] - tr[base][t] for t in tasks) / len(tasks)
        lo, hi = boot_ci(tasks, lambda s, c=c: sum(tr[c][t] - tr[base][t] for t in s) / len(s))
        new_breaks = [tag for task, tag in honesty_breaks(g[c]) if task not in base_honest]

        def med_cost(s, c=c):
            a = [x for t in s for x in cost[c].get(t, [])]
            b = [x for t in s for x in cost[base].get(t, [])]
            return (statistics.median(a) - statistics.median(b)) if a and b else None
        clo, chi = boot_ci(tasks, med_cost)
        ok = (c == base) or (rate[c] >= NONINFERIOR_FLOOR and lo is not None
                             and lo > NONINFERIOR_MARGIN and not new_breaks)
        verdict[c] = ok
        mc = med([x for v in cost[c].values() for x in v])
        md = med([x for v in dur[c].values() for x in v])
        L.append(f"| {c} | {rate[c]:.1%} | {d:+.1%} | "
                 + (f"[{lo:+.1%}, {hi:+.1%}]" if c != base else "—") + " | "
                 + (", ".join(new_breaks[:3]) or "ok") + f" | {'통과' if ok else '탈락'} | "
                 f"{fmt(mc, 3)} | "
                 + (f"[{clo:+.3f}, {chi:+.3f}]" if c != base and clo is not None else "—")
                 + f" | {fmt(md / 1000 if md else None, 1)} |")
    passed = [c for c in cells if verdict.get(c) and c != base]
    L.append("")
    if not passed:
        L.append("**비열등을 통과한 후보가 없다.**")
        return
    passed.sort(key=lambda c: med([x for v in cost[c].values() for x in v]))
    L.append(f"비열등 통과 후보(비용 순): {passed}. 비용 차이 CI가 0을 포함하는 쌍은 "
             "duration 중앙으로 가른다 — 위 표에서 읽는다.")
    L.append("")


MIXED_ARMS = ("mixed", "mixed-al")


def analyze_b(rows, cells, L):
    arms_present = {r["arm"] for r in rows}
    mixed_arms = [m for m in MIXED_ARMS if m in arms_present]
    tiers = {r["task"]: r["tier"] for r in rows}
    lit = {r["task"]: r["literal"] for r in rows}

    # §6.2: exposure is the manipulated treatment, so mixed vs mixed-al is the
    # one comparison in B that reads causally without caveats.
    if len(mixed_arms) == 2:
        L += ["## 노출 처치 — mixed(지연 로드) vs mixed-al(alwaysLoad)", "",
              "같은 표면에서 graphin MCP 설정의 `alwaysLoad`만 다르다. 무작위 순서로 섞여 돌았으므로 "
              "이 차이는 인과로 읽는다. 차이는 태스크 단위 부트스트랩 95% CI.", "",
              "| 셀 | 지표 | mixed | mixed-al | 차이 | 95% CI |", "|---|---|--:|--:|--:|---|"]
        for c in cells:
            g = {m: [r for r in rows if r["cell"] == c and r["arm"] == m and r.get("verdict") != "error"]
                 for m in mixed_arms}
            if not all(g.values()):
                continue
            ts = sorted({r["task"] for r in g["mixed"]} & {r["task"] for r in g["mixed-al"]})

            def per_task(m, f):
                out = {}
                for r in g[m]:
                    out.setdefault(r["task"], []).append(f(r))
                return {t: sum(v) / len(v) for t, v in out.items()}
            for label, f, nd in (("touched", lambda r: float(r["touched"]), 2),
                                 ("pass", lambda r: float(_is_pass(r)), 2),
                                 ("cost $", lambda r: r["cost_usd"], 3),
                                 ("tokens_in", lambda r: r["tokens_in"], 0),
                                 ("duration s", lambda r: (r["duration_ms"] or 0) / 1000, 1)):
                a, b = per_task("mixed", f), per_task("mixed-al", f)
                va = sum(a[t] for t in ts) / len(ts)
                vb = sum(b[t] for t in ts) / len(ts)
                lo, hi = boot_ci(ts, lambda s: sum(b[t] - a[t] for t in s) / len(s))
                L.append(f"| {c} | {label} | {va:,.{nd}f} | {vb:,.{nd}f} | {vb - va:+,.{nd}f} | "
                         f"[{lo:+,.{nd}f}, {hi:+,.{nd}f}] |")
        L.append("")

    L += ["## 선택 후회 — 실험 B (설계 §4.4 1)", "",
          "태스크마다 두 앵커 중 pass율이 높은 쪽(동률이면 cost 중앙이 낮은 쪽)이 그 태스크의 "
          "\"더 나은 도구\"다. 후회 = 그 앵커 pass율 − mixed pass율. 음수면 mixed가 둘 다 이겼다.", ""]
    for c in cells:
        for mx in mixed_arms:
            g = {a: [r for r in rows if r["cell"] == c and r["arm"] == a]
                 for a in (mx, "g-only", "s-only")}
            if not all(g.values()):
                L.append(f"- {c}/{mx}: 팔이 모자라 건너뜀")
                continue
            tr = {a: _task_rates(g[a]) for a in g}
            cm = {a: {} for a in g}
            for a in g:
                for r in g[a]:
                    if r.get("verdict") != "error":
                        cm[a].setdefault(r["task"], []).append(r["cost_usd"])
            L += [f"### {c} · {mx}", "",
                  f"| 태스크 | 층 | g-only | s-only | 더 나은 앵커 | {mx} | 후회 | 채택 콜 비율 중앙 | "
                  "cost / 앵커 cost |",
                  "|---|---|--:|--:|---|--:|--:|--:|--:|"]
            regrets = {}
            for t in sorted(tr[mx]):
                if t not in tr["g-only"] or t not in tr["s-only"]:
                    continue
                pg, ps = tr["g-only"][t], tr["s-only"][t]
                if pg != ps:
                    best = "g-only" if pg > ps else "s-only"
                else:
                    best = min(("g-only", "s-only"),
                               key=lambda a: med(cm[a].get(t, [])) or float("inf"))
                reg = tr[best][t] - tr[mx][t]
                regrets[t] = reg
                share = med([r["adopt_call_share"] for r in g[mx]
                             if r["task"] == t and r.get("verdict") != "error"])
                mc, bc = med(cm[mx].get(t, [])), med(cm[best].get(t, []))
                L.append(f"| {t} | {tiers[t]}{' (lit)' if lit[t] else ''} | {pg:.2f} | {ps:.2f} | "
                         f"{best} | {tr[mx][t]:.2f} | {reg:+.2f} | {fmt(share, 2)} | "
                         + (f"{mc / bc:.2f}" if mc and bc else "—") + " |")
            L.append("")
            ts = sorted(regrets)
            if ts:
                mean = sum(regrets.values()) / len(ts)
                lo, hi = boot_ci(ts, lambda s: sum(regrets[t] for t in s) / len(s))
                L.append(f"평균 후회 {mean:+.3f} (95% CI [{lo:+.3f}, {hi:+.3f}])")
                for name, pick in (("리터럴 계열", lambda t: lit[t]),
                                   ("그 외", lambda t: not lit[t])):
                    sub = [regrets[t] for t in ts if pick(t)]
                    if sub:
                        L.append(f"- {name}: 평균 후회 {sum(sub) / len(sub):+.3f} over {len(sub)} 태스크")
                by_tier = {}
                for t in ts:
                    by_tier.setdefault(tiers[t], []).append(regrets[t])
                L.append("- 층별: " + " · ".join(f"{k} {sum(v) / len(v):+.2f}({len(v)})"
                                                for k, v in sorted(by_tier.items())))
            L.append("")

            # §4.4 2: within-task, among runs that split on touching graphin.
            L += [f"#### {c} · {mx} — 태스크 안 비교 (graphin을 쓴 런 vs 안 쓴 런)", "",
                  "난이도는 태스크 고정으로 빠진다. graphin을 쓰고 text 검색으로 넘어간 런(폴백)은 "
                  "\"graphin이 실패한 런\"이라 따로 센다.", "",
                  "| 태스크 | 쓴 런(폴백 없음) pass | 폴백 런 pass | 안 쓴 런 pass | 쓴 런 cost 중앙 | 안 쓴 런 cost 중앙 |",
                  "|---|--:|--:|--:|--:|--:|"]
            split = 0
            for t in sorted(tr[mx]):
                rs = [r for r in g[mx] if r["task"] == t and r.get("verdict") != "error"]
                used = [r for r in rs if r["touched"] and not r["fallbacks"]]
                fb = [r for r in rs if r["touched"] and r["fallbacks"]]
                not_used = [r for r in rs if not r["touched"]]
                if not not_used or not (used or fb):
                    continue
                split += 1

                def pr(x):
                    return f"{sum(map(_is_pass, x))}/{len(x)}" if x else "—"
                L.append(f"| {t} | {pr(used)} | {pr(fb)} | {pr(not_used)} | "
                         f"{fmt(med([r['cost_usd'] for r in used + fb]), 3)} | "
                         f"{fmt(med([r['cost_usd'] for r in not_used]), 3)} |")
            L += ["", f"채택이 갈린 태스크 {split}개.", ""]

    # §4.4 4: does the cell move the choice itself?
    L += ["## 셀이 도구 선택을 바꾸나", "",
          "| 셀 | 팔 | touched | 콜 비율 평균 | 첫 검색기 graphin |", "|---|---|--:|--:|--:|"]
    for c in cells:
        for mx in mixed_arms:
            g = [r for r in rows if r["cell"] == c and r["arm"] == mx and r.get("verdict") != "error"]
            if not g:
                continue
            sh = [r["adopt_call_share"] for r in g if r["adopt_call_share"] is not None]
            L.append(f"| {c} | {mx} | {sum(r['touched'] for r in g)}/{len(g)} | "
                     f"{fmt(sum(sh) / len(sh) if sh else None, 2)} | "
                     f"{sum(r['first_retriever'] == 'graphin' for r in g)}/{len(g)} |")
    L.append("")


def analyze_variant(rows, cells, L):
    """rag-x against rag within each cell: a prompt revision measured against
    its same-day control, task as the bootstrap unit."""
    L += ["## 프롬프트 변형 — rag-x 대 rag (같은 셀·같은 큐)", "",
          "| 셀 | 지표 | rag | rag-x | 차이 | 95% CI |", "|---|---|--:|--:|--:|---|"]
    for c in cells:
        g = {a: [r for r in rows if r["cell"] == c and r["arm"] == a and r.get("verdict") != "error"]
             for a in ("rag", "rag-x")}
        if not all(g.values()):
            continue
        ts = sorted({r["task"] for r in g["rag"]} & {r["task"] for r in g["rag-x"]})

        def per_task(a, f):
            out = {}
            for r in g[a]:
                out.setdefault(r["task"], []).append(f(r))
            return {t: sum(v) / len(v) for t, v in out.items()}
        for label, f, nd in (("pass", lambda r: float(_is_pass(r)), 2),
                             ("calls", lambda r: r["calls"], 1),
                             ("bytes", lambda r: r["bytes_total"], 0),
                             ("cost $", lambda r: r["cost_usd"], 3),
                             ("duration s", lambda r: (r["duration_ms"] or 0) / 1000, 1)):
            a, b = per_task("rag", f), per_task("rag-x", f)
            va, vb = sum(a[t] for t in ts) / len(ts), sum(b[t] for t in ts) / len(ts)
            lo, hi = boot_ci(ts, lambda s: sum(b[t] - a[t] for t in s) / len(s))
            L.append(f"| {c} | {label} | {va:,.{nd}f} | {vb:,.{nd}f} | {vb - va:+,.{nd}f} | "
                     f"[{lo:+,.{nd}f}, {hi:+,.{nd}f}] |")
        # Full-set acceptance (fixed 2026-10-01 before the run): overall
        # pass >= floor, no tier more than 5 points below the control, and no
        # honesty break on not-here/out-of-reach that the control did not have.
        tiers_present = [t for t in rag.TIERS if any(r["tier"] == t for r in g["rag"])]
        if len(tiers_present) > 1:
            L += ["", f"### {c} 층별 pass와 수용 기준", "",
                  "| 층 | rag | rag-x | 차이 |", "|---|--:|--:|--:|"]
            worst = 0.0
            for t in tiers_present:
                ra = [r for r in g["rag"] if r["tier"] == t]
                rb = [r for r in g["rag-x"] if r["tier"] == t]
                pa_, pb_ = (sum(map(_is_pass, ra)) / len(ra), sum(map(_is_pass, rb)) / len(rb))
                worst = min(worst, pb_ - pa_)
                L.append(f"| {t} | {sum(map(_is_pass, ra))}/{len(ra)} | "
                         f"{sum(map(_is_pass, rb))}/{len(rb)} | {pb_ - pa_:+.1%} |")
            overall = sum(map(_is_pass, g["rag-x"])) / len(g["rag-x"])
            base_tasks = {task for task, _ in honesty_breaks(g["rag"])}
            new_breaks = [tag for task, tag in honesty_breaks(g["rag-x"]) if task not in base_tasks]
            checks = [(f"rag-x 전체 pass ≥ {NONINFERIOR_FLOOR:.0%}", overall >= NONINFERIOR_FLOOR,
                       f"{overall:.1%}"),
                      (f"원본 대비 {-NONINFERIOR_MARGIN:.0%}p 넘게 떨어진 층 없음",
                       worst >= NONINFERIOR_MARGIN, f"최악 {worst:+.1%}"),
                      ("not-here·out-of-reach 신규 정직성 위반 없음", not new_breaks,
                       ", ".join(new_breaks) or "없음")]
            L += [""] + [f"- {'통과' if ok else '**탈락**'} — {name}: {val}" for name, ok, val in checks]
            L.append(f"- **수용: {'통과' if all(ok for _, ok, _ in checks) else '탈락'}**")
        L += ["", f"### {c} 태스크별 pass (rag / rag-x)", ""]
        pa = per_task("rag", lambda r: float(_is_pass(r)))
        pb = per_task("rag-x", lambda r: float(_is_pass(r)))
        n = {a: {} for a in g}
        for a in g:
            for r in g[a]:
                n[a][r["task"]] = n[a].get(r["task"], 0) + 1
        for t in ts:
            L.append(f"- {t}: {round(pa[t] * n['rag'][t])}/{n['rag'][t]} → "
                     f"{round(pb[t] * n['rag-x'][t])}/{n['rag-x'][t]}")
        L.append("")


def analyze(args):
    rp = os.path.join(args.out, "report.json")
    if not os.path.exists(rp):
        raise SystemExit(f"{rp} missing — run `score` first")
    data = json.load(open(rp))
    fp = data["meta"]["fingerprint"]
    rows = data["runs"]
    cells = list(fp["cells"])
    L = [f"# 분석 — 실험 {fp['exp']}", "",
         f"런 {len(rows)} · 셀 {cells} · 팔 {fp['arms']} · 태스크 {len(fp['tasks'])}", ""]
    if fp["exp"] == "A":
        if "rag-x" in fp["arms"]:
            analyze_variant(rows, cells, L)
        else:
            analyze_a(rows, cells, L)
    else:
        analyze_b(rows, cells, L)
    text = "\n".join(L) + "\n"
    with open(os.path.join(args.out, "analysis.md"), "w", encoding="utf-8") as f:
        f.write(text)
    print(text)
    return 0


# ---------------------------------------------------------------- selftest

def selftest(_args):
    lint_contract()
    print("ok — neutral contract names no retriever")
    src = open(os.path.join(REPO, "internal/usage/ingest_test.go"), encoding="utf-8").read()
    body = src[src.index("func TestClassifyBashVariants"):]
    body = body[:body.index("\n}\n")]
    cases = re.findall(r'\{`([^`]*)`, (true|false), "([^"]*)"\}', body)
    if not cases:
        raise SystemExit("found no classifyBash fixtures in ingest_test.go — parity unchecked")
    bad = []
    for cmd, want_s, want_p in cases:
        got = classify_bash(cmd)
        if got != (want_s == "true", want_p):
            bad.append((cmd, got, (want_s, want_p)))
    if bad:
        raise SystemExit(f"classifyBash parity broke: {bad}")
    print(f"ok — classify_bash matches {len(cases)} Go fixtures")
    tok_cases = [("persistIndexesLocked", {"persist", "indexes", "locked"}),
                 ("where is the drain_signal code", {"drain", "signal"}),
                 ("HTTPServer", {"server"}),
                 ("ab c_de find", set())]
    for s, want in tok_cases:
        if intent_tokens(s) != want:
            raise SystemExit(f"intent_tokens({s!r}) = {intent_tokens(s)}, want {want}")
    print(f"ok — intent_tokens on {len(tok_cases)} cases")
    ledger = [
        {"name": "ToolSearch", "input": {"query": "select:mcp__graphin__search_hybrid"}, "result": "", "bytes": 0},
        {"name": "mcp__graphin__search_hybrid", "input": {"query": "drain signal"}, "result": "", "bytes": 100},
        {"name": "Bash", "input": {"command": "rg drainSignal internal/"}, "result": "", "bytes": 50},
        {"name": "Read", "input": {"file_path": "x.go"}, "result": "", "bytes": 500},
    ]
    m = adoption_metrics(ledger)
    denied_ledger = [{"name": "Bash", "input": {"command": "grep -rn x ."}, "bytes": 90,
                      "result": "PreToolUse:Bash hook error: Text search from the shell is not available"},
                     {"name": "mcp__graphin__search_keyword", "input": {"pattern": "x"},
                      "result": "", "bytes": 10}]
    dm = adoption_metrics(denied_ledger)
    if (dm["search_denied"], dm["search_calls"], dm["first_retriever"]) != (1, 0, "graphin"):
        raise SystemExit(f"denied calls leak into adoption: {dm}")
    want = {"nav_calls": 1, "search_bash": 1, "read_calls": 1, "fallbacks": 1,
            "same_intent_fallbacks": 1, "adopt_call_share": 0.5, "first_retriever": "graphin",
            "toolsearch_graphin": True}
    wrong = {k: (m[k], v) for k, v in want.items() if m[k] != v}
    if wrong:
        raise SystemExit(f"adoption_metrics wrong: {wrong}")
    print("ok — adoption_metrics on a fallback ledger")
    return 0


# ---------------------------------------------------------------- probe

# Phase 0 (design §6). The reasoning prompt must make thinking visible without
# any tool, so effort is the only thing that moves.
# The first draft asked an inclusion–exclusion count; every effort below xhigh
# answered it with zero thinking tokens, so it could not tell low from medium
# from the inherited setting. This one has no closed form to recall and must
# be worked (answer 140), so thinking scales with effort and the answer's
# correctness is a second, coarser signal.
PROBE_REASON = ("Without using any tools, count the integers n with 1 <= n <= 2000 "
                "such that n is odd and the sum of the decimal digits of n is divisible "
                "by 7. Answer with the number only.")
PROBE_ANSWER = "140"

PROBE_EFFORT = [  # (label, model, effort or None, user settings on?)
    ("sonnet-low", "sonnet", "low", False),
    ("sonnet-medium", "sonnet", "medium", False),
    ("sonnet-high", "sonnet", "high", False),
    ("sonnet-xhigh", "sonnet", "xhigh", False),
    ("sonnet-default", "sonnet", None, False),
    ("sonnet-inherit-user", "sonnet", None, True),
    ("sonnet-low-user", "sonnet", "low", True),
    ("opus-low", "opus", "low", False),
    ("opus-medium", "opus", "medium", False),
]


class _SkipSurface(Exception):
    pass


def probe(args):
    out = args.out
    if os.path.exists(os.path.join(out, "probe.json")):
        raise SystemExit(f"{out} already holds a probe — pick a fresh --out")
    os.makedirs(os.path.join(out, "transcripts"), exist_ok=True)
    selftest(args)
    res = {"effort": [], "surface": {}}

    # 1–3: effort precedence, auth without user settings, alias resolution.
    empty = tempfile.mkdtemp(prefix="graphin-ea-probe-")
    try:
        for rep in range(args.reps if args.only != "surface" else 0):
            for label, model, effort, user in PROBE_EFFORT:
                c = ["claude", "-p", PROBE_REASON, "--strict-mcp-config", "--model", model,
                     "--output-format", "stream-json", "--verbose", "--max-turns", "2",
                     "--disallowedTools", "Bash,Read,Grep,Glob,Edit,Write,Agent,Task,"
                                          "WebFetch,WebSearch,Skill,ToolSearch"]
                if effort:
                    c += ["--effort", effort]
                if not user:
                    c += ["--setting-sources", "project,local"]
                tr = os.path.join(out, "transcripts", f"effort-{label}-r{rep}.jsonl")
                wall, err = run_claude(c, empty, tr, 300)
                x = parse_extra(tr)
                final = rag.parse_transcript(tr)[0]
                ans = (final or "").strip()
                row = {"label": label, "rep": rep, "error": err, "wall_s": wall,
                       "answer": ans[-40:],
                       "correct": bool(re.search(r"\b" + PROBE_ANSWER + r"\b", ans[-40:])),
                       **{k: x[k] for k in ("thinking_tokens", "tokens_out", "cost_usd",
                                            "duration_api_ms", "models")}}
                res["effort"].append(row)
                print(f"effort {label} r{rep}: thinking {row['thinking_tokens']} "
                      f"out {row['tokens_out']} models {row['models']}"
                      + (f" ERROR {err}" if err else ""), flush=True)
    finally:
        shutil.rmtree(empty, ignore_errors=True)

    # 4–6, 8: the B surface, on real snapshots.
    env = Env(out, ("mixed", "g-only", "s-only"), 1, args)
    try:
        if args.only == "effort":
            raise _SkipSurface
        env.build()
        surf = {}
        tr = os.path.join(out, "transcripts", "surface-mixed.jsonl")
        _, err = run_claude(env.cmd("C2", "mixed", "What module path does go.mod declare? "
                                    "Cite the file.", 0), env.cwd("mixed", 0), tr, 600)
        x = parse_extra(tr)
        init = x["init"] or {}
        _, ledger, _, _ = rag.parse_transcript(tr)
        surf["mixed"] = {
            "error": err,
            "plugins": [p.get("name") for p in init.get("plugins", [])],
            "skills": init.get("skills", []),
            "mcp_servers": init.get("mcp_servers", []),
            "graphin_tools_in_init": [t for t in init.get("tools", []) if t.startswith("mcp__graphin__")],
            "toolsearch_in_init": "ToolSearch" in init.get("tools", []),
            "permission_mode": init.get("permissionMode"),
            "calls": [e["name"] for e in ledger],
            "models": x["models"],
        }
        tr = os.path.join(out, "transcripts", "surface-sandbox.jsonl")
        cwd = env.cwd("s-only", 0)
        _, err = run_claude(env.cmd("C2", "s-only",
                                    "Run exactly this shell command in the working directory and "
                                    "report its full output verbatim: git clone --depth 1 "
                                    "https://github.com/octocat/Hello-World.git clone-probe", 0),
                            cwd, tr, 300)
        _, ledger, _, _ = rag.parse_transcript(tr)
        surf["sandbox"] = {"error": err,
                           "clone_landed": os.path.isdir(os.path.join(cwd, "clone-probe", ".git")),
                           "bash_results": [str(e["result"])[:200] for e in ledger if e["name"] == "Bash"]}
        tr = os.path.join(out, "transcripts", "surface-g-only.jsonl")
        _, err = run_claude(env.cmd("C2", "g-only",
                                    "Run exactly these two shell commands, one at a time, and report "
                                    "each output verbatim: first `grep -n module go.mod`, then "
                                    "`head -1 go.mod`.", 0), env.cwd("g-only", 0), tr, 300)
        _, ledger, _, _ = rag.parse_transcript(tr)
        bash = [(e["input"].get("command"), str(e["result"])[:160]) for e in ledger if e["name"] == "Bash"]
        surf["g_only_hook"] = {
            "error": err, "bash": bash,
            "search_denied": any(classify_bash(c or "")[0] and "not available" in r for c, r in bash),
            "read_allowed": any(not classify_bash(c or "")[0] and "module" in r for c, r in bash)}
        res["surface"] = surf
    except _SkipSurface:
        pass
    finally:
        env.cleanup()

    res["verdicts"] = probe_verdicts(res)
    with open(os.path.join(out, "probe.json"), "w") as f:
        json.dump(res, f, ensure_ascii=False, indent=2)
    print(json.dumps(res["verdicts"], ensure_ascii=False, indent=2))
    return 0 if all(v["ok"] for v in res["verdicts"].values()) else 1


def probe_verdicts(res):
    by = {}
    for r in res["effort"]:
        by.setdefault(r["label"], []).append(r)

    def mt(label):
        return med([r["thinking_tokens"] for r in by.get(label, []) if not r["error"]])
    v = {}
    errs = [f"{r['label']} r{r['rep']}" for r in res["effort"] if r["error"]]
    v["auth_without_user_settings"] = {"ok": not errs, "detail": errs or "all runs answered"}
    models = {r["label"]: sorted(set(r["models"])) for r in res["effort"]}
    alias_ok = all(all("sonnet-5-5" in m for m in models[l]) for l in models if l.startswith("sonnet")) \
        and all(all("opus-5-5" in m for m in models[l]) for l in models if l.startswith("opus"))
    v["alias_resolution"] = {"ok": alias_ok, "detail": models}
    lo, md_, hg, hi = (mt("sonnet-low"), mt("sonnet-medium"), mt("sonnet-high"),
                       mt("sonnet-xhigh"))
    acc = {l: f"{sum(r['correct'] for r in rs)}/{len(rs)}" for l, rs in by.items()}
    # Thinking tokens alone are too noisy rep to rep to order low and medium
    # (Phase 0: 643 vs 613); the flag counts as effective when xhigh thinks
    # more than low AND low is not more accurate than xhigh.
    def ncorrect(label):
        return sum(r["correct"] for r in by.get(label, []))
    v["effort_monotone_sonnet"] = {
        "ok": None not in (lo, hi) and hi > lo and ncorrect("sonnet-low") <= ncorrect("sonnet-xhigh"),
        "detail": {"low": lo, "medium": md_, "high": hg, "xhigh": hi, "correct": acc}}
    olo, omd = mt("opus-low"), mt("opus-medium")
    v["effort_monotone_opus"] = {"ok": None not in (olo, omd) and olo <= omd,
                                 "detail": {"low": olo, "medium": omd}}
    inh, lou = mt("sonnet-inherit-user"), mt("sonnet-low-user")
    flag_wins = None not in (lou, lo, inh) and abs(lou - lo) <= abs(lou - inh)
    v["flag_beats_user_settings"] = {
        "ok": flag_wins, "detail": {"low+user": lou, "low": lo, "inherit-user": inh}}
    # Informational, not a gate: what the historical (no-flag) condition looks like.
    v["historical_condition"] = {"ok": True, "detail": {
        "inherit-user thinking": inh, "closest": min(
            ((abs(inh - x), n) for n, x in (("low", lo), ("medium", md_), ("high", hg),
                                             ("xhigh", hi))
             if None not in (inh, x)), default=(None, None))[1]}}
    if not res["surface"]:
        return v
    s = res["surface"].get("mixed", {})
    v["b_surface"] = {
        "ok": not s.get("error") and "graphin" not in s.get("plugins", [])
        and "graphin-guide" in s.get("plugins", [])
        and any("graphin" in k for k in s.get("skills", []))
        and any(m.get("name") == "graphin" and m.get("status") == "connected"
                for m in s.get("mcp_servers", [])),
        "detail": {k: s.get(k) for k in ("plugins", "mcp_servers", "toolsearch_in_init",
                                         "permission_mode", "calls")}
        | {"skills": [k for k in s.get("skills", []) if "graphin" in k or ":" in k][:12],
           "graphin_tools_in_init": len(s.get("graphin_tools_in_init", []))}}
    sb = res["surface"].get("sandbox", {})
    v["sandbox_blocks_clone"] = {"ok": not sb.get("clone_landed", True), "detail": sb}
    g = res["surface"].get("g_only_hook", {})
    v["g_only_hook"] = {"ok": bool(g.get("search_denied")) and bool(g.get("read_allowed")),
                        "detail": g.get("bash")}
    return v


# ---------------------------------------------------------------- main

def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("selftest", help="no LLM: contract lint + usage parity")

    def common(p):
        p.add_argument("--out", required=True)
        p.add_argument("--bin", default=os.path.join(REPO, "bin/graphin"))
        p.add_argument("--ref", default="HEAD")
        p.add_argument("--worktree", action="store_true")
        p.add_argument("--max-turns", type=int, default=40)
        p.add_argument("--index-timeout", type=float, default=600.0)

    pp = sub.add_parser("probe", help="Phase 0 probes")
    common(pp)
    pp.add_argument("--reps", type=int, default=2)
    pp.add_argument("--only", choices=["all", "effort", "surface"], default="all")

    rp = sub.add_parser("run", help="run one phase")
    common(rp)
    rp.add_argument("--exp", choices=["A", "B"], required=True)
    rp.add_argument("--cells", required=True, help="comma list, e.g. C0,C1,C2")
    rp.add_argument("--arms", default="", help="subset of the experiment's arms")
    rp.add_argument("--subset", choices=["all", "smoke"], default="all")
    rp.add_argument("--tasks", default="", help="comma list of task ids (overrides subset)")
    rp.add_argument("--runs", type=int, default=3, help="runs per task for arm rag")
    rp.add_argument("--mixed-runs", type=int, default=5)
    rp.add_argument("--anchor-runs", type=int, default=3)
    rp.add_argument("--seed", type=int, default=20260930)
    rp.add_argument("--jobs", type=int, default=3)
    rp.add_argument("--run-timeout", type=int, default=900)
    rp.add_argument("--resume", action="store_true")
    rp.add_argument("--agent-variant", default="",
                    help="agent .md for arm rag-x (the skill stays the shipped one)")
    rp.add_argument("--detach", action="store_true")

    sp = sub.add_parser("score", help="grade a finished phase")
    sp.add_argument("--out", required=True)
    ap_ = sub.add_parser("analyze", help="decision rule (A) or choice regret (B) over a scored phase")
    ap_.add_argument("--out", required=True)

    args = ap.parse_args()
    return {"selftest": selftest, "probe": probe, "run": run, "score": score,
            "analyze": analyze}[args.cmd](args)


if __name__ == "__main__":
    sys.exit(main())
