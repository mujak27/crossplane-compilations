SHELL := /bin/bash
.DEFAULT_GOAL := render

RENDER_ARGS ?=

.PHONY: render
render:
	@set -euo pipefail; \
	if (echo > /dev/tcp/localhost/9443) 2>/dev/null; then \
		echo "Port 9443 is already in use. Stop the existing function before running make render." >&2; \
		exit 1; \
	fi; \
	tmp=$$(mktemp -d); \
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
	echo "Building TLS ingress function..." >&2; \
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
