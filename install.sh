#!/usr/bin/env bash
# paymint installer — downloads the latest release tarball from GitHub,
# verifies it against checksums.txt, and installs the binary into PREFIX.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/vanducng/paymint/main/install.sh | bash
#   curl -fsSL .../install.sh | VERSION=v0.1.0 bash
#   curl -fsSL .../install.sh | PREFIX="$HOME/.local/bin" bash

set -euo pipefail

REPO="vanducng/paymint"
BIN="paymint"
VERSION="${VERSION:-latest}"
PREFIX="${PREFIX:-/usr/local/bin}"

err()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '\033[36m::\033[0m %s\n' "$*"; }

command -v curl >/dev/null || err "curl is required"
command -v tar  >/dev/null || err "tar is required"

# --- Detect OS/arch -----------------------------------------------------------
uname_s=$(uname -s)
uname_m=$(uname -m)

case "$uname_s" in
  Linux)  os=linux  ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS: $uname_s (linux and darwin only)" ;;
esac

case "$uname_m" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported arch: $uname_m (amd64 and arm64 only)" ;;
esac

# --- Resolve version ----------------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  info "resolving latest release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || err "could not resolve latest release tag"
fi

# Strip leading "v" for filename matching: tarball is paymint_0.1.0_<os>_<arch>.tar.gz
ver_nov="${VERSION#v}"
tarball="${BIN}_${ver_nov}_${os}_${arch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$VERSION"

info "installing $BIN $VERSION for $os/$arch"

# --- Download + verify --------------------------------------------------------
tmp=$(mktemp -d) && trap 'rm -rf "$tmp"' EXIT
cd "$tmp"

info "downloading $tarball"
curl -fSL --progress-bar -o "$tarball"      "$base_url/$tarball"
curl -fsSL                  -o checksums.txt "$base_url/checksums.txt" \
  || err "failed to fetch checksums.txt — release may be incomplete"

info "verifying checksum"
if command -v sha256sum >/dev/null; then
  grep " $tarball\$" checksums.txt | sha256sum -c - >/dev/null \
    || err "checksum mismatch for $tarball"
elif command -v shasum >/dev/null; then
  grep " $tarball\$" checksums.txt | shasum -a 256 -c - >/dev/null \
    || err "checksum mismatch for $tarball"
else
  err "neither sha256sum nor shasum found — cannot verify checksum"
fi

info "extracting"
tar -xzf "$tarball"
[ -x "$BIN" ] || err "extracted archive missing executable '$BIN'"

# --- Install ------------------------------------------------------------------
sudo=""
if [ ! -w "$PREFIX" ] && [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null; then sudo=sudo; else
    err "$PREFIX is not writable and sudo is unavailable; set PREFIX=\$HOME/.local/bin and re-run"
  fi
fi

info "installing to $PREFIX/$BIN"
$sudo install -m 0755 "$BIN" "$PREFIX/$BIN"

# --- Done ---------------------------------------------------------------------
hash -r 2>/dev/null || true
if command -v "$BIN" >/dev/null && [ "$(command -v "$BIN")" = "$PREFIX/$BIN" ]; then
  printf '\n\033[32m✓\033[0m installed: %s\n' "$($BIN version 2>/dev/null || echo "$PREFIX/$BIN")"
else
  # shellcheck disable=SC2016  # $PATH must stay literal in the printed instruction
  printf '\n\033[33m!\033[0m installed at %s but not in PATH — add it with:\n  export PATH="%s:$PATH"\n' \
    "$PREFIX/$BIN" "$PREFIX"
fi
