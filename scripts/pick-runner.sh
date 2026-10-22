#!/usr/bin/env bash
# Chooses where the Linux CI and release jobs run and prints the value for
# `runs-on` as JSON: "ubuntu-latest" (GitHub-hosted) or the local runner labels.
# The local runner is used only when it has announced itself recently and the
# event is trusted. Anything unexpected, such as an unset or malformed
# heartbeat, means hosted. This script never fails.
#
# Environment:
#   SEEN          heartbeat of the local runner, epoch seconds (repo variable LOCAL_RUNNER_SEEN)
#   NOW           current epoch seconds (default: now)
#   EVENT         github.event_name
#   HEAD_REPO     head repository of a pull request (empty otherwise)
#   REPO          this repository (owner/name)
#   DISABLED      "true" turns the local runner off (repo variable LOCAL_RUNNER_DISABLED)
#   FORCE_HOSTED  "true" forces hosted (workflow_dispatch input runner=hosted)
#   MAX_AGE       seconds a heartbeat stays fresh (default 180)
# The reason goes to stderr, the JSON to stdout.

hosted='"ubuntu-latest"'
local_runner='["self-hosted","linux","sie-local"]'

pick_hosted() {
  echo "Using hosted runner: $1" >&2
  echo "$hosted"
  exit 0
}

now=${NOW:-$(date +%s 2>/dev/null)}
max_age=${MAX_AGE:-180}

[ "${FORCE_HOSTED:-}" = true ] && pick_hosted "forced"
[ "${DISABLED:-}" = true ] && pick_hosted "local runner disabled"
case ${SEEN:-} in
'' | *[!0-9]*) pick_hosted "no valid heartbeat from the local runner" ;;
esac
case $now in
'' | *[!0-9]*) pick_hosted "cannot read the clock" ;;
esac

age=$((now - SEEN))
if [ "$age" -lt 0 ] || [ "$age" -gt "$max_age" ]; then
  pick_hosted "heartbeat is ${age}s old"
fi

case ${EVENT:-} in
push | workflow_dispatch) ;;
pull_request)
  [ -n "${HEAD_REPO:-}" ] && [ "${HEAD_REPO:-}" = "${REPO:-}" ] || pick_hosted "pull request from a fork"
  ;;
*) pick_hosted "event ${EVENT:-unknown} is not trusted" ;;
esac

echo "Using local runner (heartbeat ${age}s old)" >&2
echo "$local_runner"
