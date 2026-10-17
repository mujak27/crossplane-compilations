# Istio secure egress

`XIstioSecureEgress` maps one exact DNS hostname and logical `gatewayProfile` to
four `networking.istio.io/v1` resources in the application's namespace:
ServiceEntry, Gateway, gateway-Service DestinationRule and VirtualService.
One generic Composition serves every profile; it has no environment-specific input.

Applications send **HTTP on port 80**. The sidecar uses `ISTIO_MUTUAL` and the
external hostname as SNI to reach an existing platform gateway. The gateway-bound
HTTP route targets external port 443. External HTTP authority is not rewritten.
The platform must separately configure verified upstream TLS origination in the
gateway namespace. No external-host TLS rule, Secret, Service or Deployment is
composed here. This is routing configuration, **not application TLS, enforced
egress isolation, or evidence of traffic health**.

## Local development

Requires Go 1.26+ (repository toolchain 1.27.1), Python 3.9+, Make, and a Crossplane
CLI with `render --required-resources`. Kubectl is used only for local Kustomize.
No Docker, registry or cluster is needed:

```sh
python3 -m venv function/.venv
function/.venv/bin/python -m pip install -r function/requirements.txt
make generate
make test
(cd function && go test -race ./... && go vet ./...)
make render
kubectl kustomize .
```

`make render` starts a native function on localhost:9443 and refuses an occupied
port. It supplies `examples/environmentconfig.yaml` through `--required-resources`
and renders the `team-a/istio-secure-egress-demo` XR. `XR` and `PROFILE` environment
variables can select alternate fixture paths; `RENDER_ARGS` passes additional CLI
flags. `render.sh` alone expects a running local function.

Exercise the same Composition with another namespace, selector, exposed port and
non-default Service DNS suffix:

```sh
XR=examples/xr-regional.yaml PROFILE=examples/environmentconfig-regional.yaml make render
```

Exercise pending (missing/drifted) and matching mock observations serially:

```sh
make render RENDER_ARGS='--observed-resources examples/observed-pending.yaml'
make render RENDER_ARGS='--observed-resources examples/observed-matching.yaml'
```

The first remains `Ready=False`; the second demonstrates provisioning `Ready=True`
from observed configuration, not successful traffic. These fixtures use the nondefault
application and gateway namespaces in the default XR/profile pair.

## Provisioning readiness

Each stable resource key (`service-entry`, `gateway`, `gateway-destination-rule`,
`virtual-service`) is explicitly unready until an observation has the intended API
version, kind, name, namespace and complete spec, without deletion in progress.
XR/profile updates reset readiness for each changed resource until its new
configuration is observed. Incoming composite status and unrelated pipeline state
are preserved; the function does not force the composite Ready.

Controller metadata and status are ignored, including any Istio conditions.
Only these extra spec defaults are tolerated: ServiceEntry port `targetPort: 0`,
Gateway server TLS `httpsRedirect: false`, subset port TLS `insecureSkipVerify: false`,
and VirtualService single-destination route `weight: 0` or `100`. All other extra,
missing or changed spec fields remain unready, including injected routing/security
policies. Lists are compared in order. This deliberately conservative allowlist may
need extending for a future API/controller's harmless defaults; unknown changes
never silently count as matching.

Provisioning readiness means **observed application-owned configuration only**.
It does not establish xDS acceptance/convergence, functioning gateway endpoints,
platform TLS-rule or trust-Secret existence, runtime upstream certificate validity,
successful HTTPS, gateway traversal, network-policy enforcement, bypass denial or
allowed-client authorization. There are no active probes or platform-object
observations, and no assumed universal Istio Ready or Gateway API Programmed condition.

Go types/validation markers in `function/api/v1alpha1/types.go` are the source of
truth. Commit generated `xrd.yaml` and deepcopy output with source changes. Generate
twice to check determinism. `make check-generated` compares against **committed HEAD**;
it intentionally fails for untracked or merely staged outputs. Generation uses pinned
controller-gen/Python requirements and does not tidy or change module dependencies.

Native build (no package publication):

```sh
(cd function && go build -o build/function .)
```

## Profile contract and installation boundaries

`spec.hostname` is one lowercase DNS hostname, at most 253 characters with labels
at most 63; no IP, URL, wildcard or port. `spec.gatewayProfile` is a DNS label of
1–50 characters, resolving only to `istio-egress-<profile>` (at most 63 characters).
XR name and namespace are required. Resource names equal the XR name; reserve these
names across owners of each kind. A deterministic namespace/name hash distinguishes
gateway subsets; the platform still must enforce unique hostname/listener ownership.

The function reads that exact, platform-owned, cluster-scoped
`apiextensions.crossplane.io/v1beta1` EnvironmentConfig using required resources:

- `data.gateway.namespace`: gateway namespace (DNS label).
- `data.gateway.serviceFQDN`: `<service>.<namespace>.svc.<cluster DNS suffix>`;
  non-default suffixes are supported.
- `data.gateway.selector`: nonempty valid Kubernetes pod labels.
- `data.gateway.port`: exposed Service port, 1–65535, not container targetPort.

Initial lookup preserves pipeline desired state and requests the profile; an empty
resolved result, deletion, invalid profile, or explicitly unsupported required-resource
capability is fatal. Versions predating capability advertisement can still perform the
handshake. Crossplane needs read access to EnvironmentConfigs. Selecting a profile is
not authorization to use that gateway. Profiles contain no credentials or certificates.

Kustomize includes **only** the v2 Namespaced XRD and generic Composition. Install
the shared Function separately with a real published package; `functions.yaml` uses
an **unpublished placeholder**, not an installable release. Apply profiles and XRs
separately. The fixture profile is illustrative, not actual environment configuration.
Actual profiles, TLS/trust policy and GitOps wiring belong to `k8s-crossplane`, which
this example does not modify or read at runtime.

Before any installation, the platform must provide Istio sidecar capture, compatible
Service/listener mapping, API support/RBAC, intended selector labels and
`PILOT_SCOPE_GATEWAY_TO_NAMESPACE=false` for cross-namespace Gateway selection.
Discovery/Sidecar imports must allow visibility. ServiceEntry/VirtualService export
only locally and to the selected gateway namespace; the gateway-Service
DestinationRule is private. Platform policy/trust must enable verified upstream TLS;
independent network controls must prevent bypass. This function changes none of these
prerequisites.

## Profile updates and migration

Profile edits are independent of Composition revisions and may affect every consumer
when it reconciles. Updating a profile or selecting another replaces all managed
gateway routing, selector, port and export settings while keeping application-local
resource names and subsets stable. ServiceEntry/VirtualService exports explicitly
name the application and gateway namespaces, deduplicated when they are the same.
The gateway-Service DestinationRule remains private with `exportTo: ["."]`.

Use a new profile name for staged migration. Prepare the target listeners and
platform TLS/trust policy first, move consumers and verify their reconciliation,
then retire the old profile. Rendering the new profile does not prove traffic
health, platform TLS/trust availability, atomic migration or immediate consumer
reconciliation. Operational trust examples and onboarding are follow-up #25.
