#!/bin/sh
# toktop installer — downloads the latest release binary for your OS/arch.
# Usage: curl -fsSL https://raw.githubusercontent.com/furkanalp41/toktop/main/install.sh | sh
set -eu

REPO="furkanalp41/toktop"
BIN="toktop"

info() { printf '\033[0;34m==>\033[0m %s\n' "$1"; }
err() { printf '\033[0;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac
case "$os" in
  linux | darwin) ;;
  *) err "unsupported OS: $os (try the Releases page or 'go install')" ;;
esac

# Resolve the latest tag from the GitHub API.
info "Finding the latest release of $REPO..."
tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep -m1 '"tag_name"' | cut -d'"' -f4)
[ -n "${tag:-}" ] || err "could not determine the latest release tag"

asset="${BIN}_${tag#v}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$asset"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
info "Downloading $asset..."
curl -fsSL "$url" -o "$tmp/$asset" || err "download failed: $url"
tar -xzf "$tmp/$asset" -C "$tmp"

# Pick an install dir we can write to.
if [ -w /usr/local/bin ]; then
  dest=/usr/local/bin
elif command -v sudo >/dev/null 2>&1 && [ "$os" != "windows" ]; then
  dest=/usr/local/bin
  SUDO="sudo"
else
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
fi

info "Installing to $dest/$BIN"
${SUDO:-} install -m 0755 "$tmp/$BIN" "$dest/$BIN"

if ! command -v "$BIN" >/dev/null 2>&1; then
  printf '\033[0;33mNote:\033[0m %s is not on your PATH. Add this to your shell profile:\n' "$dest"
  printf '  export PATH="%s:$PATH"\n' "$dest"
fi

info "Done. Run '$BIN --demo' to take it for a spin."
