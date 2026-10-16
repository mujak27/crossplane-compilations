# Template integration

Repository paths below are relative to the Git root. Read
`function-template/README.md`, especially **Manual customization checklist**, and
inspect its Makefile and scripts before running commands. That checklist is the
source of truth for adapting the starter; this reference adds acceptance ordering.

## Validate the merged template

Use `function-template/` from the task's freshly fetched `origin/main` baseline,
not another session's worktree. In the dedicated task worktree, run from that
directory:

```sh
python3 -m venv function/.venv
function/.venv/bin/python -m pip install -r function/requirements.txt
make generate
make check-generated
make test
(cd function && go test -race ./... && go vet ./...)
make render
kubectl kustomize .
```

Require unchanged committed generated outputs and dependency manifests. Kustomize
must emit only the XRD and Composition. Render must produce the expected ConfigMap
in the XR namespace. The starter intentionally leaves readiness unspecified:
`Ready=False` / `Unready resources: configmap` is expected, not a failed render.
A function promising readiness needs deliberate behavior and tests.

## Scaffold the approved function

Copy tracked template files into the approved project directory, leaving the
source unchanged. Exclude virtual environments, intermediate CRDs, caches, and
build outputs. Follow the manual customization checklist; no initialization
generator is required. Replace module, API, resource, and package placeholders
consistently across code, tests, manifests, examples, and generation paths.
Generate XRD and deepcopy files from the adapted Go types.

Repeat the checks above in the new directory with this ordering:

1. Generate twice; require deterministic output and unchanged dependency manifests.
2. Run tests, race/vet, render, and Kustomize against the new API and behavior.
   Exercise an explicit non-default namespace and verify composed resources remain
   there. Preserve unrelated desired state and test chosen readiness semantics.
3. Review and commit source and generated XRD/deepcopy files locally as part of
   authorized implementation. On a new scaffold, `make check-generated` intentionally
   fails before this commit: it compares against HEAD and requires tracked outputs;
   staging alone is insufficient.
4. Run `make check-generated` against that committed baseline and resolve drift
   before accepting the ticket. Report unavailable or failed checks explicitly.

Serialize render checks across worktrees because they use localhost:9443. If the
port is busy, report the blocker rather than terminating another session's process.
Local rendering needs no Docker, registry, or cluster. Package builds, publishing,
and installation are separate work, governed by the approved scope and authorization.
