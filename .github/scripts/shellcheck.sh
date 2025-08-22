#!/usr/bin/env bash
#
# Run shellcheck on list of received files

set -eo pipefail

if [[ -z "${ALL_CHANGED_FILES}" ]]; then
	echo "==> No Changed Files received"
	exit 0
fi

for FILE in ${ALL_CHANGED_FILES}; do
	echo "==> Running shellcheck for ${FILE}"
	shellcheck "${FILE}"
	echo -e "Exit code: $?\n"
done
