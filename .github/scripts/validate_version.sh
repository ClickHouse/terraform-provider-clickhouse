#!/bin/bash

set -e

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 3.6.4"
    exit 1
fi

version="$1"

echo "Validating version: $version"

if [[ ! $version =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$ ]]; then
    echo "Error: Invalid semver format: \"$version\". Must be something like 1.2.3 without leading \"v\""
    exit 1
fi

echo "✓ Valid semver format"

existing_tags=$(git tag --sort=-version:refname)

if [ -z "$existing_tags" ]; then
    echo "No existing versions found, allowing version $version"
    exit 0
fi

if echo "$existing_tags" | grep -q "^v$version$"; then
    echo "Error: Version $version already exists as a release"
    exit 1
fi

latest_version=$(echo "$existing_tags" | head -1 | sed 's/^v//')
echo "Latest existing version: $latest_version"
echo "Proposed version: $version"

if [ "$(printf '%s\n%s\n' "$latest_version" "$version" | sort -V | tail -1)" = "$version" ] && [ "$latest_version" != "$version" ]; then
    echo "✓ Version $version is higher than latest version $latest_version"
    exit 0
else
    echo "Error: Version $version is not higher than the latest version $latest_version"
    exit 1
fi
