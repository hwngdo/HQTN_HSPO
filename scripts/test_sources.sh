#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ "${HSPO_SKIP_INTEGRITY:-0}" != "1" ]; then
  python3 "$ROOT/scripts/verify_package.py"
fi
for dir in "$ROOT/apps/core-recovered" "$ROOT/apps/qa-gateway" "$ROOT/apps/uat-preview-8970"; do
  if [ -f "$dir/go.mod" ]; then
    echo "TEST: $dir"
    (cd "$dir" && go test ./... && go vet ./...)
  fi
done
python3 "$ROOT/scripts/check_policy_snapshot.py"
python3 "$ROOT/scripts/extract_api_contract.py" >/dev/null
