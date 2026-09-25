#!/usr/bin/env bash
set -euo pipefail

# Credential-free exeuntu bootstrap. All archives are checked before extraction.
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'Linux/amd64 required' >&2; exit 1; }
root="${XDG_CACHE_HOME:-$HOME/.cache}/tmp"
mkdir -p "$root" "$HOME/.local/bin" "$HOME/.local/share"
tmp=$(mktemp -d "$root/scry-setup.XXXXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
fetch() {
  local url=$1 checksum=$2 dest=$3
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --retry 3 --output "$dest" "$url"
  printf '%s  %s\n' "$checksum" "$dest" | sha256sum --check --strict >/dev/null
}
install_tar() {
  local name=$1 url=$2 checksum=$3 top=$4 destination=$5
  if [[ ! -d "$destination" ]]; then
    fetch "$url" "$checksum" "$tmp/$name.tar.gz"
    mkdir -p "$tmp/$name"
    tar -xzf "$tmp/$name.tar.gz" -C "$tmp/$name"
    mv "$tmp/$name/$top" "$destination"
  fi
}
install_tar go https://go.dev/dl/go1.27.1.linux-amd64.tar.gz \
  63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445 go "$HOME/.local/share/go-1.27.1"
install_tar node https://nodejs.org/dist/v22.22.0/node-v22.22.0-linux-x64.tar.gz \
  c33c39ed9c80deddde77c960d00119918b9e352426fd604ba41638d6526a4744 \
  node-v22.22.0-linux-x64 "$HOME/.local/share/node-22.22.0"
export PATH="$HOME/.local/share/go-1.27.1/bin:$HOME/.local/share/node-22.22.0/bin:$HOME/.local/bin:$PATH"
if [[ ! -x "$HOME/.local/bin/bun-1.4.2" ]]; then
  fetch https://github.com/oven-sh/bun/releases/download/bun-v1.4.2/bun-linux-x64-baseline.zip \
    c678040f14fe0440eb839d37cbd0ce4c051a32da72806ac97de6a6aab6bf728f "$tmp/bun.zip"
  unzip -q "$tmp/bun.zip" -d "$tmp"
  install -m 755 "$tmp/bun-linux-x64-baseline/bun" "$HOME/.local/bin/bun-1.4.2"
fi
ln -sfn bun-1.4.2 "$HOME/.local/bin/bun"
if [[ ! -x "$HOME/.local/bin/gitleaks-8.30.1" ]]; then
  fetch https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz \
    551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb "$tmp/gitleaks.tar.gz"
  tar -xzf "$tmp/gitleaks.tar.gz" -C "$tmp" gitleaks
  install -m 755 "$tmp/gitleaks" "$HOME/.local/bin/gitleaks-8.30.1"
fi
ln -sfn gitleaks-8.30.1 "$HOME/.local/bin/gitleaks"
profile_line='export PATH="$HOME/.local/share/go-1.27.1/bin:$HOME/.local/share/node-22.22.0/bin:$HOME/.local/bin:$PATH"'
touch "$HOME/.profile"
if ! grep -Fxq "$profile_line" "$HOME/.profile"; then printf '\n%s\n' "$profile_line" >> "$HOME/.profile"; fi

walk_modules="$HOME/.local/share/scry-walk"
if [[ ! -f "$walk_modules/node_modules/playwright/package.json" ]] ||
   [[ "$(node -p "require('$walk_modules/node_modules/playwright/package.json').version" 2>/dev/null || true)" != 1.63.0 ]]; then
  PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install --prefix "$walk_modules" --no-save --no-audit --no-fund --loglevel=error playwright@1.63.0
fi
# The Playwright package pins the Chromium revision and installs it under HOME.
# Its install-deps command uses apt only when noninteractive sudo is available.
if ! node -e "const p=require('$walk_modules/node_modules/playwright');require('fs').accessSync(p.chromium.executablePath(),require('fs').constants.X_OK)" 2>/dev/null; then
  if ! sudo -n true 2>/dev/null; then echo 'Chromium needs apt dependencies; noninteractive sudo unavailable' >&2; exit 1; fi
  sudo -n env PATH="$PATH" node "$walk_modules/node_modules/playwright/cli.js" install-deps chromium
  node "$walk_modules/node_modules/playwright/cli.js" install chromium
fi
chromium_binary=$(node -p "require('$walk_modules/node_modules/playwright').chromium.executablePath()")
[[ -x "$chromium_binary" ]]
ln -sfn "$chromium_binary" "$HOME/.local/bin/chromium"
[[ "$(go version)" == 'go version go1.27.1 linux/amd64' ]]
[[ "$(node --version)" == v22.* ]]
[[ "$(bun --version)" == '1.4.2' ]]
[[ "$(gitleaks version)" == '8.30.1' ]]
[[ -x "$HOME/.local/bin/chromium" ]]
echo 'Scry VM toolchain ready (Go 1.27.1, Node 22.22.0, Bun 1.4.2, Gitleaks 8.30.1, Playwright 1.63.0 Chromium).'
