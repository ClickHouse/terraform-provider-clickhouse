#!/usr/bin/env bash
# Re-imports everything this example created and checks the imported state is
# usable as-is. The rest of the e2e run only covers create and destroy, and most
# of the bugs these resources have shipped were import bugs (#654, #659, #663).
#
# Run from the example directory, after `terraform apply`, with the same
# -var-file. Leaves state populated so the caller can still destroy.
#
# Run it once per apply. It finishes with the dashboard imported, whose state
# then holds the server's body rather than the authored one, so the next plan
# reports a dashboard change until an apply reconciles it. The e2e run does
# exactly that (import test, then the update apply, then destroy); a second
# back-to-back run of this script would fail on that self-inflicted diff.
set -euo pipefail

VAR_FILE="${1:-variables.tfvars}"
tf() { terraform "$@" -no-color; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Every check here removes a resource from state before importing it back. If the
# import fails in between, the object still exists on the shared ClickStack
# service but Terraform no longer knows about it, so destroy cannot clean it up
# and nothing else sweeps it. Restore the pre-run state on any non-zero exit.
STATE_BACKUP=$(mktemp)

restore_state_on_failure() {
  local rc=$?
  if [ "$rc" -ne 0 ] && [ -s "$STATE_BACKUP" ]; then
    echo "restoring pre-import state so destroy can still tear down" >&2
    cp "$STATE_BACKUP" terraform.tfstate
  fi
  rm -f "$STATE_BACKUP"
  exit "$rc"
}
trap restore_state_on_failure EXIT

cp terraform.tfstate "$STATE_BACKUP"

resource_id() {
  tf state show "$1" | awk '$1=="id"{gsub(/"/,"",$3); print $3; exit}'
}

reimport() {
  local addr id
  addr="$1"
  id=$(resource_id "$addr")
  if [ -z "$id" ]; then fail "$addr: no id in state"; fi
  echo "--- re-importing $addr ($id)"
  tf state rm "$addr" >/dev/null
  tf import -var-file="$VAR_FILE" "$addr" "$id" >/dev/null
}

# Attribute-based resources: config and imported state should agree exactly, so
# a plan after importing them must be empty. This is what regressed in #663 and
# #659 — the API echoes an unset expression back as "", and without the
# empty-string-equals-null handling an imported source plans an endless no-op
# update.
#
# They are imported together and checked with a single plan, because a plan
# covers the whole configuration: planning after each import would report the
# same diff against whichever resource happened to be checked first.
reimport clickhouse_clickstack_source.logs
reimport clickhouse_clickstack_saved_search.errors
reimport clickhouse_clickstack_webhook.alerts
reimport clickhouse_clickstack_alert.too_many_errors
reimport clickhouse_clickstack_role.readonly

echo "--- plan after import (expecting no changes)"
set +e
PLAN_OUT=$(mktemp)
tf plan -detailed-exitcode -var-file="$VAR_FILE" >"$PLAN_OUT" 2>&1
plan_rc=$?
set -e

case "$plan_rc" in
  0) ;;
  2)
    echo "the imported state does not match the configuration:" >&2
    cat "$PLAN_OUT" >&2
    fail "import produced a dirty plan"
    ;;
  *)
    cat "$PLAN_OUT" >&2
    fail "plan errored after import (exit $plan_rc)"
    ;;
esac

# The dashboard is checked differently, and last — its config is a hand-authored
# JSON body while the import synthesizes dashboard_json from what the server
# returns (defaults applied), so the two legitimately differ and "plan is empty"
# is the wrong assertion. Importing it before the plan above would make that plan
# dirty for a reason that is not a bug.
#
# What must hold is that the synthesized body is *writable*: the API rejects a
# top-level id and rejects ids on filters, but tile ids have to survive or
# UI-created tile alerts lose their binding. That is #654.
reimport clickhouse_clickstack_dashboard.e2e

body=$(tf show -json | jq -r '
  .values.root_module.resources[]
  | select(.address == "clickhouse_clickstack_dashboard.e2e")
  | .values.dashboard_json')
if [ -z "$body" ] || [ "$body" = "null" ]; then
  fail "dashboard: dashboard_json empty after import"
fi

echo "$body" | jq -e 'has("id") | not' >/dev/null \
  || fail "dashboard: imported body kept the top-level id (the gateway 400s on write)"
echo "$body" | jq -e '[(.filters // [])[] | has("id")] | any | not' >/dev/null \
  || fail "dashboard: imported body kept filter ids (/dashboards/validate rejects them)"
echo "$body" | jq -e '[(.tiles // [])[] | has("id")] | all' >/dev/null \
  || fail "dashboard: imported body lost tile ids (UI tile alerts would be orphaned)"

echo "import checks passed"
