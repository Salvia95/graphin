#!/usr/bin/env bash
# Installs the pinned flatc binary used to generate internal/graph/fbsgen.
# Generated code is committed; this script is only needed when schema/graph.fbs changes.
set -euo pipefail

TAG="v25.12.19-2026-02-06-03fffb2"
ASSET="Linux.flatc.binary.g++-13.zip"
SHA256="c5e0adf44ffc20556620427eb6f183022d1e94d64f751a6b67e2ebd9cec76c9a"
EXPECT_VERSION="25.12.19"
DEST="${HOME}/.local/bin"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -sL -o "$tmp/flatc.zip" "https://github.com/google/flatbuffers/releases/download/${TAG}/${ASSET}"
echo "${SHA256}  $tmp/flatc.zip" | sha256sum -c -
unzip -o -d "$tmp" "$tmp/flatc.zip" >/dev/null
chmod +x "$tmp/flatc"

got="$("$tmp/flatc" --version)"
case "$got" in
  *"$EXPECT_VERSION"*) ;;
  *) echo "flatc version mismatch: got '$got', expected ${EXPECT_VERSION}" >&2; exit 1 ;;
esac

mkdir -p "$DEST"
mv "$tmp/flatc" "$DEST/flatc"
echo "installed $("$DEST/flatc" --version) to $DEST/flatc"
