#!/usr/bin/env sh

set -eu

repo="viicslen/discord-progress-agent"
bin_name="session-agent"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'install.sh: missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

need_cmd uname
need_cmd mktemp
need_cmd chmod
need_cmd mkdir
need_cmd mv

pick_asset() {
  os=$(uname -s)
  arch=$(uname -m)

  case "$os/$arch" in
    Linux/x86_64)
      printf 'discord-progress-agent-linux-amd64\n'
      ;;
    Darwin/arm64)
      printf 'discord-progress-agent-macos-arm64\n'
      ;;
    MINGW64_NT-*/x86_64|MSYS_NT-*/x86_64|CYGWIN_NT-*/x86_64)
      printf 'discord-progress-agent-windows-amd64.exe\n'
      ;;
    *)
      printf 'install.sh: unsupported platform %s/%s\n' "$os" "$arch" >&2
      printf 'Supported release assets: linux-amd64, macos-arm64, windows-amd64.exe\n' >&2
      exit 1
      ;;
  esac
}

pick_install_dir() {
  if [ -n "${INSTALL_DIR:-}" ]; then
    printf '%s\n' "$INSTALL_DIR"
    return
  fi

  if [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
    printf '/usr/local/bin\n'
    return
  fi

  printf '%s/.local/bin\n' "$HOME"
}

download() {
  url=$1
  out=$2

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi

  printf 'install.sh: need curl or wget to download releases\n' >&2
  exit 1
}

asset=$(pick_asset)
install_dir=$(pick_install_dir)
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT INT TERM HUP

mkdir -p "$install_dir"

download_url="https://github.com/${repo}/releases/latest/download/${asset}"
tmpbin="$tmpdir/$bin_name"

download "$download_url" "$tmpbin"
chmod +x "$tmpbin"
mv "$tmpbin" "$install_dir/$bin_name"

printf 'Installed %s to %s/%s\n' "$asset" "$install_dir" "$bin_name"

case ":$PATH:" in
  *":$install_dir:"*)
    ;;
  *)
    printf 'Add %s to PATH if it is not already there.\n' "$install_dir"
    ;;
esac

printf 'Run `%s --version` to verify the install.\n' "$bin_name"
