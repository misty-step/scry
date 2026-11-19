#!/usr/bin/env bash
# One-time host setup (run as root from the repo's deploy/host directory).
# Writes /etc/scry/scry.env from the caller's environment; values never print.
set -euo pipefail
id scry >/dev/null 2>&1 || useradd --system --home /srv/scry --shell /usr/sbin/nologin scry
mkdir -p /srv/scry/builds /srv/scry/data /etc/scry /usr/local/lib/scry
chown scry:scry /srv/scry /srv/scry/data
install -m 0644 scry-lib.sh /usr/local/lib/scry/scry-lib.sh
install -m 0755 scry-deploy scry-rollback scry-status /usr/local/bin/
install -m 0644 scry.service /etc/systemd/system/scry.service
umask 077
{ echo "PORT=${PORT:-8080}"; echo "SCRY_DB=/srv/scry/data/scry.db"
  echo "SCRY_BASE_URL=${SCRY_BASE_URL:?}"; echo "SCRY_MODE=${SCRY_MODE:?}"
  env | grep -E '^(CLERK|STRIPE|SENTRY|OPENAI|ANTHROPIC|ELEVENLABS|LITESTREAM)_[A-Z_]*=' || true
} > /etc/scry/scry.env
systemctl daemon-reload
systemctl enable scry >/dev/null

# Litestream (backup): binary, config and service.
if ! command -v litestream >/dev/null; then
  v=0.5.18
  curl -fsSL "https://github.com/benbjohnson/litestream/releases/download/v$v/litestream-$v-linux-x86_64.tar.gz" | tar -xz -C /tmp
  install -m 0755 "$(find /tmp -maxdepth 2 -name litestream -type f | head -1)" /usr/local/bin/litestream
fi
install -m 0644 litestream.yml /etc/scry/litestream.yml
install -m 0644 litestream.service /etc/systemd/system/litestream.service
systemctl daemon-reload
systemctl enable --now litestream >/dev/null
