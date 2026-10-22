#!/usr/bin/env bash
# Tests scripts/pick-runner.sh with every combination that matters.
set -u
cd "$(dirname "$0")" || exit 1

hosted='"ubuntu-latest"'
local_runner='["self-hosted","linux","sie-local"]'
fail=0

# check <name> <expected> [VAR=value ...]
check() {
  name=$1 want=$2
  shift 2
  got=$(env -i PATH="$PATH" REPO=avero-it/sie NOW=1000 "$@" bash ./pick-runner.sh 2>/dev/null)
  code=$?
  if [ "$got" != "$want" ] || [ "$code" -ne 0 ]; then
    echo "FAIL $name: got '$got' (exit $code), want '$want'"
    fail=1
  fi
}

check "no heartbeat"            "$hosted"       EVENT=push
check "empty heartbeat"         "$hosted"       EVENT=push SEEN=
check "malformed heartbeat"     "$hosted"       EVENT=push SEEN=abc
check "negative heartbeat"      "$hosted"       EVENT=push SEEN=-5
check "heartbeat in the future" "$hosted"       EVENT=push SEEN=2000
check "stale heartbeat"         "$hosted"       EVENT=push SEEN=700
check "fresh heartbeat"         "$local_runner" EVENT=push SEEN=900
check "heartbeat at the limit"  "$local_runner" EVENT=push SEEN=820
check "just over the limit"     "$hosted"       EVENT=push SEEN=819
check "custom max age"          "$local_runner" EVENT=push SEEN=700 MAX_AGE=400
check "dispatch"                "$local_runner" EVENT=workflow_dispatch SEEN=900
check "same-repo pull request"  "$local_runner" EVENT=pull_request HEAD_REPO=avero-it/sie SEEN=900
check "fork pull request"       "$hosted"       EVENT=pull_request HEAD_REPO=someone/sie SEEN=900
check "pull request, no head"   "$hosted"       EVENT=pull_request SEEN=900
check "pull_request_target"     "$hosted"       EVENT=pull_request_target HEAD_REPO=avero-it/sie SEEN=900
check "schedule"                "$hosted"       EVENT=schedule SEEN=900
check "no event"                "$hosted"       SEEN=900
check "forced hosted"           "$hosted"       EVENT=push SEEN=900 FORCE_HOSTED=true
check "force_hosted false"      "$local_runner" EVENT=push SEEN=900 FORCE_HOSTED=false
check "kill switch"             "$hosted"       EVENT=push SEEN=900 DISABLED=true
check "bad clock"               "$hosted"       EVENT=push SEEN=900 NOW=x

[ "$fail" -eq 0 ] && echo "pick-runner: all cases pass"
exit "$fail"
