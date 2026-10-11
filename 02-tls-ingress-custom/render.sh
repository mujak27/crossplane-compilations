#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
crossplane render examples/xr.yaml composition.yaml functions.yaml "$@"
