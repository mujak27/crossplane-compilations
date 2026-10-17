SHELL := /bin/bash
.DEFAULT_GOAL := render

RENDER_ARGS ?=
PYTHON ?= function/.venv/bin/python
CONTROLLER_GEN_VERSION := v0.22.0

.PHONY: generate check-generated test render
generate:
	cd function && GOFLAGS=-mod=readonly go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION) object crd paths=./api/... output:crd:artifacts:config=generated/crds
	$(PYTHON) function/scripts/crd_to_xrd.py function/generated/crds/platform.mujak.com_xistiosecureegresses.yaml xrd.yaml

check-generated:
	$(PYTHON) function/scripts/check_generated.py

test:
	cd function && go test ./...
	$(PYTHON) -m unittest discover -s function/scripts -p 'test_*.py'

render:
	@set -euo pipefail; \
	if (echo > /dev/tcp/localhost/9443) 2>/dev/null; then \
		echo "Port 9443 is already in use. Stop the existing function before running make render." >&2; \
		exit 1; \
	fi; \
	tmp=$$(mktemp -d "$${TMPDIR:-/tmp}/istio-secure-egress-render.XXXXXX"); \
	pid=""; \
	cleanup() { \
		if [[ -n "$$pid" ]]; then \
			kill "$$pid" 2>/dev/null || true; \
			wait "$$pid" 2>/dev/null || true; \
		fi; \
		rm -f "$$tmp/function"; \
		rmdir "$$tmp"; \
	}; \
	trap cleanup EXIT; \
	trap 'exit 130' INT; \
	trap 'exit 143' TERM; \
	echo "Building secure-egress function..." >&2; \
	(cd function && go build -o "$$tmp/function" .); \
	"$$tmp/function" --insecure --debug & \
	pid=$$!; \
	ready=false; \
	for ((attempt = 0; attempt < 100; attempt++)); do \
		if ! kill -0 "$$pid" 2>/dev/null; then \
			echo "Function exited before becoming ready." >&2; \
			exit 1; \
		fi; \
		if (echo > /dev/tcp/localhost/9443) 2>/dev/null; then \
			ready=true; \
			break; \
		fi; \
		sleep 0.1; \
	done; \
	if [[ "$$ready" != true ]]; then \
		echo "Function did not listen on port 9443 within 10 seconds." >&2; \
		exit 1; \
	fi; \
	./render.sh $(RENDER_ARGS)
