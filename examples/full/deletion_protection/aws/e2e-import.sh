#!/usr/bin/env bash
# Checks that a protected service survives `terraform destroy` and that an
# import shows the protection. The e2e update pass then turns protection off,
# and the final destroy proves it can be deleted again.
#
# Run from the example directory, after `terraform apply`, with the same
# -var-file. Leaves state populated so the caller can still destroy.
set -euo pipefail

VAR_FILE="${1:-variables.tfvars}"
ADDR="clickhouse_service.service"
tf() { terraform "$@" -no-color; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# A failed import below would leave the service out of state. Restoring the
# pre-run state keeps it tracked so the update pass can turn protection off;
# if that pass never runs, the cleanup sweep unprotects and deletes it.
STATE_BACKUP=$(mktemp)
DESTROY_OUT=$(mktemp)
PLAN_OUT=$(mktemp)
PLAN_FILE=$(mktemp)
CMD_OUT=$(mktemp)

restore_state_on_failure() {
  local rc=$?
  if [ "$rc" -ne 0 ] && [ -s "$STATE_BACKUP" ]; then
    echo "restoring pre-import state so Terraform still tracks the protected service" >&2
    cp "$STATE_BACKUP" terraform.tfstate
  fi
  rm -f "$STATE_BACKUP" "$DESTROY_OUT" "$PLAN_OUT" "$PLAN_FILE" "$CMD_OUT"
  exit "$rc"
}
trap restore_state_on_failure EXIT

cp terraform.tfstate "$STATE_BACKUP"

protection_in_state() {
  tf show -json | jq -r --arg addr "$ADDR" '
    .values.root_module.resources[]
    | select(.address == $addr)
    | .values.deletion_protection'
}

[ "$(protection_in_state)" = "true" ] || fail "deletion_protection is not true in state after apply"

# Read before the destroy: a destroy run can drop outputs from state even when the delete is refused.
id=$(tf output -raw service_id)
[ -n "$id" ] || fail "no service_id output"

# Terraform wraps diagnostic text and prefixes each line with a box border; join it back into one line.
destroy_text() { sed 's/^[│ ]*//' "$DESTROY_OUT" | tr '\n' ' ' | tr -s ' '; }

echo "--- destroy (expecting it to be refused)"
# The rest of the e2e job retries each Terraform call; retry only a failure that isn't the protection refusal.
for attempt in 1 2 3; do
  set +e
  tf destroy -auto-approve -var-file="$VAR_FILE" >"$DESTROY_OUT" 2>&1
  destroy_rc=$?
  set -e
  if [ "$destroy_rc" -eq 0 ] || grep -q "Protected From Deletion" <<<"$(destroy_text)"; then
    break
  fi
  if [ "$attempt" -lt 3 ]; then
    echo "destroy failed for another reason (attempt $attempt), retrying" >&2
    sleep 30
  fi
done

if [ "$destroy_rc" -eq 0 ]; then
  # The service is gone, so restoring state for it would only make the update pass recreate it.
  rm -f "$STATE_BACKUP"
  cat "$DESTROY_OUT" >&2
  fail "destroy succeeded on a protected service"
fi

if ! grep -q "Protected From Deletion" <<<"$(destroy_text)"; then
  cat "$DESTROY_OUT" >&2
  fail "destroy failed, but not because of deletion protection"
fi
# The refusal must come from the provider's check before the stop, not from the API after stopping the service.
if grep -q "is now stopped" <<<"$(destroy_text)"; then
  cat "$DESTROY_OUT" >&2
  fail "destroy stopped the protected service before it was refused"
fi
tf state show "$ADDR" >/dev/null || fail "service was removed from state by the refused destroy"

echo "--- re-importing $ADDR"
tf state rm "$ADDR" >"$CMD_OUT" 2>&1 || { cat "$CMD_OUT" >&2; fail "state rm failed"; }
tf import -var-file="$VAR_FILE" "$ADDR" "$id" >"$CMD_OUT" 2>&1 || { cat "$CMD_OUT" >&2; fail "import failed"; }

[ "$(protection_in_state)" = "true" ] || fail "deletion_protection is not true in state after import"

echo "--- plan after import (expecting no deletion_protection change and no replacement)"
tf plan -var-file="$VAR_FILE" -out="$PLAN_FILE" >"$PLAN_OUT" 2>&1 || { cat "$PLAN_OUT" >&2; fail "plan errored after import"; }
changed=$(terraform show -json "$PLAN_FILE" | jq -r --arg addr "$ADDR" '
  .resource_changes[]?
  | select(.address == $addr)
  | select((.change.actions | index("delete")) or (.change.before.deletion_protection != .change.after.deletion_protection))
  | .address')
if [ -n "$changed" ]; then
  cat "$PLAN_OUT" >&2
  fail "plan after import wants to change deletion_protection or replace the service"
fi

echo "deletion protection checks passed"
