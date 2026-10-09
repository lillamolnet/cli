#!/usr/bin/env bash
# Updates api/openapi.json from the public API. The tests check the CLI's test
# responses against it, so run the tests afterwards.
#
#   scripts/update-openapi.sh
#
# LILLAMOLNET_API_URL selects another environment. Requires curl and jq.

set -euo pipefail

cd "$(dirname "$0")/.."

base="${LILLAMOLNET_API_URL:-https://lillamolnet.se}"

curl -fsS "${base%/}/api/v1/openapi.json" | jq --sort-keys . > api/openapi.json.tmp
mv api/openapi.json.tmp api/openapi.json

git diff --stat -- api/openapi.json
