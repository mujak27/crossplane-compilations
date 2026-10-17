#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
crossplane render "${XR:-examples/xr.yaml}" composition.yaml functions.yaml \
  --required-resources "${PROFILE:-examples/environmentconfig.yaml}" "$@"
