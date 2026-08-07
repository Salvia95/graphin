#!/bin/sh
# Black-box check of the published release's install path: the manifest, the
# asset, and install.sh — the three things a user actually runs, and the only
# ones the Go test suite cannot reach.
#
# Two callers share this file on purpose:
#   ci.yml      install-smoke — every push, against the manifest on main
#   release.yml verify        — right after publishing, against the new tag
#
# They must not drift. A release that verified itself with a weaker check than
# CI would be worse than not verifying at all, because it would still report
# green.
#
# Requires curl, jq, tar and sha256sum, and is meant to run somewhere go and
# cc are ABSENT: install.sh falls back to `go install` when a download fails,
# and that fallback would quietly compile a broken manifest into a pass.
#
#   usage: install-smoke.sh <plugin-root> <expected-arch>

set -eu

ROOT="${1:?usage: install-smoke.sh <plugin-root> <expected-arch>}"
WANT_ARCH="${2:?usage: install-smoke.sh <plugin-root> <expected-arch>}"

MANIFEST="$ROOT/install/manifest.json"
if [ ! -r "$MANIFEST" ]; then
  echo "::error::no install manifest at $MANIFEST"
  exit 1
fi

want="$(jq -r '.version // empty' "$MANIFEST")"
if [ -z "$want" ]; then
  echo "::error::$MANIFEST has no version"
  exit 1
fi

DATA="$(mktemp -d)"
trap 'rm -rf "$DATA"' EXIT INT TERM HUP

echo "── installing $want ($WANT_ARCH) into an empty data dir"
CLAUDE_PLUGIN_ROOT="$ROOT" CLAUDE_PLUGIN_DATA="$DATA" sh "$ROOT/install/install.sh"

# A symlink, not a copy: overwriting a running binary in place is ETXTBSY on
# Linux, and the upgrade path depends on swapping this link.
if [ ! -L "$DATA/bin/graphin" ]; then
  echo "::error::$DATA/bin/graphin is not a symlink"
  exit 1
fi
echo "   symlink → $(readlink "$DATA/bin/graphin")"

# go-install would mean the download failed and the fallback covered for it.
src="$(jq -r '.source // empty' "$DATA/state/installed.json")"
if [ "$src" != "release" ]; then
  echo "::error::installed from '$src', not the published release asset"
  exit 1
fi

"$DATA/bin/graphin" version --json > "$DATA/v.json"
cat "$DATA/v.json"
if ! jq -e --arg v "$want" --arg a "$WANT_ARCH" \
  '.version == $v and .os == "linux" and .arch == $a and .semantic_supported == true' \
  "$DATA/v.json" > /dev/null; then
  echo "::error::version --json disagrees with the manifest (want $want / $WANT_ARCH, semantic on)"
  exit 1
fi

# An identical manifest means an identical install: the second run must print
# nothing and download nothing. Regressing this turns every session start into
# a 25MB fetch.
out="$(CLAUDE_PLUGIN_ROOT="$ROOT" CLAUDE_PLUGIN_DATA="$DATA" sh "$ROOT/install/install.sh" 2>&1)"
if [ -n "$out" ]; then
  echo "::error::second run was not a no-op"
  printf '%s\n' "$out"
  exit 1
fi

echo "── OK: $want from the release, ran on $(ldd --version 2>/dev/null | head -1), fast path clean"
