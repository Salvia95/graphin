#!/usr/bin/env python3
"""Verify that every knowledge-set entry still points at a real section.

A knowledge set names sections by anchor, and a heading rename silently breaks
that name — the same failure mode the plugin version guard exists for. This is
a pure text check: it re-derives GitHub-style slugs from the target files and
compares, so it needs neither graphin nor an index.

    scripts/check-knowledge.py [docs/knowledge/*.md]

Exit 1 with a per-entry report when anything dangles.
"""

import os
import re
import sys
import glob
import unicodedata

ENTRY = re.compile(r"^\s*-\s+\[([^\]]+)\]\(([^)\s]+)\)")
FENCE = re.compile(r"^\s{0,3}(`{3,}|~{3,})")
HEADING = re.compile(r"^(#{1,6})\s+(.*)$")


def slugify(text: str) -> str:
    """Mirror internal/parse/markdown.go: no hyphen collapsing, no trimming.

    The character test must be Go's, not Python's. `unicode.IsDigit` is the Nd
    category alone, while `str.isalnum()` also accepts Nl/No — so a heading
    numbered `①` slugs to `원인--단건` in the parser and `원인-①-단건` here.
    Using isalnum() made this guard both reject valid entries and, worse, pass
    an entry whose anchor graphin can never resolve.
    """
    out = []
    for ch in text.lower():
        cat = unicodedata.category(ch)
        if cat.startswith("L") or cat == "Nd" or ch in "_-":
            out.append(ch)
        elif ch.isspace():
            out.append("-")
    return "".join(out)


def anchors_of(path: str) -> set:
    """Every slug a file defines, disambiguated the way the parser does."""
    seen, out = {}, set()
    in_fence, fence_char = False, ""
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            m = FENCE.match(line)
            if m:
                ch = m.group(1)[0]
                if not in_fence:
                    in_fence, fence_char = True, ch
                elif ch == fence_char:
                    in_fence, fence_char = False, ""
                continue
            if in_fence:
                continue
            h = HEADING.match(line.rstrip("\n"))
            if not h:
                continue
            slug = slugify(h.group(2).strip().rstrip("#").strip()) or "section"
            n = seen.get(slug, 0)
            seen[slug] = n + 1
            out.add(slug if n == 0 else f"{slug}-{n}")
    return out


def main(argv):
    paths = argv[1:] or sorted(glob.glob("docs/knowledge/*.md"))
    if not paths:
        print("no knowledge sets found", file=sys.stderr)
        return 0

    anchor_cache = {}
    entries = broken = 0
    for setfile in paths:
        base = os.path.dirname(setfile)
        with open(setfile, encoding="utf-8") as fh:
            for lineno, line in enumerate(fh, 1):
                m = ENTRY.match(line)
                if not m:
                    continue
                entries += 1
                target = m.group(2)
                if target.startswith(("http://", "https://")):
                    continue
                path, _, anchor = target.partition("#")
                node = os.path.normpath(os.path.join(base, path)).replace(os.sep, "/")
                if not os.path.isfile(node):
                    print(f"::error file={setfile},line={lineno}::"
                          f"no such file: {node}")
                    broken += 1
                    continue
                if not anchor:
                    continue
                if node not in anchor_cache:
                    anchor_cache[node] = anchors_of(node)
                if anchor not in anchor_cache[node]:
                    print(f"::error file={setfile},line={lineno}::"
                          f"{node} has no section '{anchor}' — a heading was "
                          f"renamed and this entry now points at nothing")
                    broken += 1

    print(f"knowledge sets: {len(paths)} · entries: {entries} · broken: {broken}")
    return 1 if broken else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
