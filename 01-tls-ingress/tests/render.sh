#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

pending=$(crossplane composition render examples/xr.yaml composition.yaml functions.yaml)
grep -q 'ready: false' <<< "$pending"
grep -q 'health: Pending' <<< "$pending"

healthy=$(crossplane composition render examples/xr.yaml composition.yaml functions.yaml \
  --observed-resources tests/certificate.yaml)
grep -q 'ready: true' <<< "$healthy"
grep -q 'health: Healthy' <<< "$healthy"
grep -q 'notAfter: "2099-01-01T00:00:00Z"' <<< "$healthy"
grep -q 'platform.mujak.com/cert-not-after: "2099-01-01T00:00:00Z"' <<< "$healthy"

echo 'PASS: initial and observed-Certificate renders'
