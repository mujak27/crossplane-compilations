# Adding a function

This is the authoritative workflow for planning, scaffolding, implementing, and
validating a custom Crossplane function in this repository. It is harness-neutral:
use it directly, or through the `create-crossplane-function` skill.

[`function-template/README.md`](../function-template/README.md) remains the source
of truth for the starter's mechanics and its **Manual customization checklist**;
this guide links to that checklist instead of repeating it. The root
[`README.md`](../README.md#naming-and-installation) owns naming and installation
conventions, and [`GLOSSARY.md`](../GLOSSARY.md) plus relevant decisions in
`docs/adr/` (when present) own the domain vocabulary.

Planning a function, invoking a skill, or approving a plan is not authorization
to publish issues or tickets, start or dispatch implementation, build or publish
packages, or change a cluster. Obtain explicit user approval for each of those
actions.

## 1. Decide what the function does

Read repository and ancestor `AGENTS.md`, [`docs/agents/domain.md`](agents/domain.md),
the root README's naming conventions, the glossary, and relevant ADRs when
present. If the work is tracked in GitHub Issues, also read
[`docs/agents/issue-tracker.md`](agents/issue-tracker.md) and
[`docs/agents/triage-labels.md`](agents/triage-labels.md). Inspect the existing
numbered examples and `function-template/` without modifying them.

Resolve and record:

- the purpose and a short, filesystem-safe project slug and directory;
- the API identity: group, version, kinds, plural and singular, and owning XRD;
- the typed XR contract: inputs, defaults, validation, and status;
- each composed resource, its stable desired-resource key, and its namespace;
- naming and collision rules from the root README;
- which unrelated desired pipeline state must be preserved;
- error behavior for invalid input and failed composition;
- readiness semantics, chosen explicitly (see provisioning readiness and
  application health in the glossary);
- test seams for generation, behavior, namespace isolation, and readiness;
- explicit non-goals.

Design is complete when these decisions are recorded and the requested scope is
clear. Planning and design may proceed while the template or environment is
blocked. Mark scaffolding and validation as blocked rather than implying they are
complete.

## 2. Verify the template baseline

Use `function-template/` from the freshly fetched task baseline, normally
`origin/main` in a dedicated worktree. Do not copy another session's uncommitted
template.

A template change is ready only when its implementation is merged into `main` and
generation, tests, drift checks, and local render pass. Verify the implementation
commits and the actual checks; issue or ticket closure alone does not prove
readiness. If the template is not ready, planning and issue preparation may
continue, but scaffolding stays explicitly blocked and the evidence is reported.

## 3. Scaffold the new function

Copy tracked template files into the approved project directory, leaving the
source unchanged. Exclude virtual environments, intermediate CRDs, caches, and
build outputs. Follow the template's
[Manual customization checklist](../function-template/README.md#manual-customization-checklist);
no initialization generator is required. Replace module, API, resource, package,
example, and generation-path placeholders consistently across code, tests,
manifests, examples, and generation paths.

Start with a minimal working path: the scaffold plus one composed resource
end-to-end, then broaden behavior.

## 4. Keep schema generation authoritative

Go types and validation markers in `function/api/<version>/types.go` are the
schema source of truth. Generation is:

1. pinned `controller-gen` emits the namespaced CRD and deepcopy code together;
2. `function/scripts/crd_to_xrd.py` converts the CRD to an
   `apiextensions.crossplane.io/v2` XRD with `spec.scope: Namespaced`.

Commit `xrd.yaml` and `zz_generated.deepcopy.go`; never hand-edit them. Generation
does not update dependency manifests. Run `go mod tidy` only when intentionally
changing dependencies, not as a generation step.

## 5. Implement the behavior

- Keep composed-resource keys stable and preserve unrelated desired pipeline state.
- Validate inputs and report clear errors.
- Choose readiness semantics deliberately. Provisioning readiness is whether
  composed resources reached the desired configuration; application health is
  whether traffic succeeds. Neither Certificate status nor successful rendering
  alone proves both. A function that promises readiness needs deliberate behavior
  and tests.
- Add tests for behavior, an explicit non-default namespace, preservation of
  unrelated desired state, and the chosen readiness behavior.

Keep the template and existing numbered examples unchanged unless the approved
scope includes them.

## 6. Apply Crossplane v2 namespaced rules

- XRDs use `apiextensions.crossplane.io/v2` with `spec.scope: Namespaced`.
- Managed resources use namespaced APIs.
- Set `metadata.namespace` on resource instances, and keep composed resources in
  the composite's namespace.
- Keep XRDs, Compositions, and Functions unnamespaced: they are inherently
  cluster-scoped.
- Do not introduce legacy claims or cluster-scoped composite, managed, or workload
  resources.

## 7. Validate

Run the following from `function-template/` in the implementation worktree, after
inspecting its Makefile and scripts. This block validates the already-committed
merged template:

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

A new scaffold follows the acceptance order below, where `make check-generated`
runs only after the first commit.

For the merged template, committed generated files and dependency manifests must
remain unchanged. Kustomize emits only the XRD and Composition. Render produces
the expected ConfigMap in the XR namespace. The starter intentionally leaves
readiness unspecified: `Ready=False` / `Unready resources: configmap` is expected,
not a failed render.

### New scaffold acceptance order

1. Generate twice; require deterministic output and unchanged dependency
   manifests.
2. Run tests, race/vet, render, and Kustomize for the new API and behavior.
   Exercise a non-default namespace, unrelated desired-state preservation, and
   the chosen readiness behavior. Include applicable local build checks from the
   template checklist within the authorized scope.
3. Review and commit source plus generated XRD/deepcopy outputs locally when the
   implementation is authorized. On a new scaffold, `make check-generated`
   intentionally fails before that commit because it compares against HEAD and
   requires tracked outputs; staging alone is insufficient.
4. Run `make check-generated` against the committed baseline and resolve drift
   before acceptance. Report failed or unavailable checks explicitly.

Render checks bind `localhost:9443`. Serialize them across worktrees; a busy port
is a blocker, not permission to stop another process. Local rendering needs no
Docker, registry, or cluster.

Validation reporting is complete when every applicable check is accounted for as
passed, failed, or unavailable. Acceptance requires every applicable check to
pass; any missing check remains a blocker.

## 8. Keep external actions separate

Local generation, tests, and render do not authorize package builds, publication,
or cluster installation. Do those only with explicit approval for those actions.
Do not install the template's placeholder Function package as-is: its registry
reference is intentionally unpublished. For cluster use, customize, build, and
publish your own package first (see the template README), then install the
definitions and examples separately.

## Skill portability

The editable skill lives at `.agents/skills/create-crossplane-function/`.
OpenCode V2 discovers `.agents/skills` directly. Claude Code discovers project
skills under `.claude/skills`, so this repository provides
`.claude/skills` as a symlink to `../.agents/skills`. A checkout that cannot
materialize symlinks, such as some Windows configurations, will not load the
Claude alias; recreate the link or point the harness at `.agents/skills`. This
repository keeps one editable copy rather than synchronizing a second one.

## Reporting

Report the checks actually run, their results, and every blocked or unavailable
check. Do not claim acceptance from issue or ticket state, plan approval, or an
unavailable environment.
