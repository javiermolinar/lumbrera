#!/bin/sh
set -eu

repo="${LUMBRERA_REPO:-javiermolinar/lumbrera}"
version="${LUMBRERA_VERSION:-latest}"
install_dir="${INSTALL_DIR:-/usr/local/bin}"
bin_name="lumbrera"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: $1 is required" >&2
    exit 1
  fi
}

need curl
need tar
need uname
need mktemp
need install

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin|Linux) ;;
  *) echo "error: unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch="x86_64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "error: unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="lumbrera_${os}_${arch}.tar.gz"
if [ "$version" = "latest" ]; then
  url="https://github.com/${repo}/releases/latest/download/${asset}"
else
  case "$version" in
    v*) ;;
    *) version="v${version}" ;;
  esac
  url="https://github.com/${repo}/releases/download/${version}/${asset}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$url" -o "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

if mkdir -p "$install_dir" 2>/dev/null && [ -w "$install_dir" ]; then
  install -m 0755 "$tmp/$bin_name" "$install_dir/$bin_name"
elif command -v sudo >/dev/null 2>&1; then
  sudo install -d "$install_dir"
  sudo install -m 0755 "$tmp/$bin_name" "$install_dir/$bin_name"
else
  echo "error: $install_dir is not writable and sudo is not available" >&2
  echo "set INSTALL_DIR to a writable directory, for example:" >&2
  echo "  curl -fsSL https://raw.githubusercontent.com/${repo}/main/scripts/install.sh | INSTALL_DIR=\$HOME/.local/bin sh" >&2
  exit 1
fi

"$install_dir/$bin_name" version
