# Custom Go function template

A provider-independent starter: `XExample.spec.message` becomes `data.message` in
one core/v1 ConfigMap named after the XR, in the XR's namespace. The stable
desired-resource key is `configmap`; unrelated pipeline state is preserved.
This demonstrates generation, tests, and local rendering, not application health.
The ConfigMap's readiness is left unspecified; there is no readiness detector.
Consequently local render reports the ConfigMap as unready and the XR's Ready
condition as false. This is not a provisioning-readiness or application-health
implementation; add deliberate readiness behavior when adapting it.

## Quick start

Requires Go 1.26+ (the repository currently uses 1.27.1), Python 3.9+ with venv,
Make, and the Crossplane CLI supporting `crossplane render`. Kubectl is needed
only to build the definitions-only Kustomize base. No cluster or provider is
needed for rendering.

From this directory:

```sh
python3 -m venv function/.venv
function/.venv/bin/python -m pip install -r function/requirements.txt
make generate
make test
cd function && go test -race ./... && go vet ./... && cd ..
make render
kubectl kustomize .
```

`make generate` runs pinned controller-gen, then the Python converter. Go types
and validation markers in `function/api/v1alpha1/types.go` are the schema source
of truth. Commit `xrd.yaml` and `zz_generated.deepcopy.go`, never edit them by
hand. Intermediate CRDs and the local Python environment are ignored.
Generation does not update the module manifests.

`make check-generated` regenerates outputs and fails on changes relative to HEAD
(including staged changes), missing outputs, or untracked XRD/deepcopy files.
It requires these generated artifacts to be committed and may rewrite them while
checking; inspect the result before committing. `make test` also checks manifest
identities and converter boundaries using the pinned Python environment.
On a newly copied or still-untracked template, the check intentionally fails
until the generated outputs have been committed. Staging them alone is not
sufficient. The checker does not stage, commit, or reset any files.

`make render` checks port 9443, builds and starts the function, waits for its
listener, runs `crossplane render`, and cleans up on exit. It prints the XR and
the ConfigMap `default/function-template-example` with the sample message.
`./render.sh` can also be used with an already running local function.

Kustomize includes **only** the XRD and Composition. Functions and example XRs
are separate resources. Do **not** install `functions.yaml` as-is: its package
reference is an intentionally unpublished placeholder. Local rendering uses the
development runtime instead and needs no package registry.

For an eventual cluster installation, first customize, build, and publish your
own Function package outside this starter workflow. Install that Function
separately and wait for it to become healthy, install the definitions with
`kubectl apply -k .`, then apply `examples/xr.yaml` separately in an existing
namespace. These commands change a cluster and are not needed for local render.

## Build locally (no publishing)

Docker with a running daemon and BuildKit is required for the runtime image;
the native Go binary and local render do not require Docker. From this directory:

```sh
mkdir -p function/build
(cd function && go build -o build/function .)
docker build --platform linux/arm64 -t function-template:local function
crossplane xpkg build --package-root function/package \
  --embed-runtime-image function-template:local \
  --package-file function/build/function-template.xpkg
```

This builds only local artifacts, not an installable registry reference. The
example selects `linux/arm64`; use `linux/amd64` for an amd64 runtime instead.
Only one architecture is built at a time. Base-image downloads need network
access. The package embeds the runtime image; do not mistake a metadata-only
package for a runnable Function. Build outputs are ignored. No push, release
tag, or automated publishing is included.

## Before adapting

`example.org/function-template` is a placeholder Go module path; replace it and
its imports with your own module. The API group `template.example.org` and the
package `registry.example.org/your-org/function-template:replace-me` are also
placeholders. Rename these consistently across Go types, generated schemas,
Composition, Function metadata, example, and generation paths. This starter
does not publish packages or provision cloud resources.

## Manual customization checklist

- [ ] Copy/rename `function-template/` and choose a unique project ID. Update
  documentation and project-owned resource names; leave the numbered examples
  unchanged.
- [ ] Replace `example.org/function-template` in `function/go.mod` and all Go
  imports (including tests). Run `go mod tidy` only when intentionally changing
  dependencies, not as a generation step.
- [ ] Choose API group, version, kind/list kind, plural and singular in
  `function/api/v1alpha1/types.go`. Rename the API directory and imports if the
  version changes. Keep the typed resource handling and controller-gen markers aligned.
- [ ] Replace the XRD identity `xexamples.template.example.org` and Composition
  identity `function-template.template.example.org` consistently. Update
  `composition.yaml`'s composite type reference and the example's API version,
  kind, and `spec.crossplane.compositionRef.name`.
- [ ] Rename Function metadata in `functions.yaml` and
  `function/package/crossplane.yaml`, and its pipeline reference in
  `composition.yaml`. Replace the unpublished
  `registry.example.org/your-org/function-template:replace-me` only with your
  actual package reference before cluster use. Keep Development runtime
  annotations for local render; remove them if no longer developing locally.
- [ ] Choose the example XR name and namespace in `examples/xr.yaml`. Create the
  namespace separately for cluster use. Keep composed resources in that namespace
  and avoid naming collisions with other workloads.
- [ ] Update Makefile API paths and converter input
  `function/generated/crds/template.example.org_xexamples.yaml` for the new
  group/plural. Adjust the generated-checker's required deepcopy path if the API
  directory moves. Keep `render.sh` paths and `kustomization.yaml` aligned if
  moving manifests. Do not hand-edit generated `xrd.yaml` or deepcopy output.
- [ ] Replace ConfigMap behavior in `function/fn.go` and its tests. Preserve
  unrelated desired state and stable resource keys, validate inputs, and choose
  readiness semantics explicitly. Update schema fields/markers, examples,
  manifest/converter tests and any hard-coded identity assertions together.
- [ ] Rerun generation twice, tests/race/vet, render, Kustomize and local builds.
  Review and commit generated outputs with the source changes when authorized,
  then run `make check-generated` against that committed baseline.
