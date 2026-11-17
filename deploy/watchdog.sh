#!/bin/sh
# Run on the Docker host every minute. Docker restart policy handles exits;
# this complements it by restarting a running but unhealthy container.
set -eu
name=${1:-scry}
status=$(docker inspect --format '{{if .State.Running}}{{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}{{else}}stopped{{end}}' "$name")
case "$status" in
  unhealthy)
    printf '{"level":"ERROR","msg":"restarting unhealthy Scry container"}\n'
    docker restart --time 20 "$name"
    ;;
  healthy|starting|stopped) ;;
  *) printf '{"level":"ERROR","msg":"Scry health state unavailable"}\n' >&2; exit 1 ;;
esac
