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
then retire the old profile and prerequisites only after all consumers have moved.
Rendering the new profile does not prove traffic health, platform TLS/trust
availability, atomic migration, immediate fan-out reconciliation or zero downtime.
External profiles are not snapshots pinned by Composition revisions; verify each
consumer actually reconciles after a profile edit.

## Separately owned upstream TLS and trust

These are **portable examples, not actual platform configuration**. All hostnames,
namespaces, Service FQDNs/DNS suffixes, labels and ports must be replaced together
with approved installation values. The two pairs select the same generic Composition:

| Application fixture | External profile | Gateway namespace / Service port | Platform prerequisite |
| --- | --- | --- | --- |
| `examples/xr.yaml`: `team-a`, `api.example.com` | `istio-egress-east-west` | `istio-gateway-east-west` / 80 | `examples/platform/public-ca.yaml` |
| `examples/xr-regional.yaml`: `team-b`, `regional.example.com` | `istio-egress-regional` | `gateway-regional` / 8443 | `examples/platform/custom-trust.yaml` |

Each external-host DestinationRule belongs in the **selected gateway proxy's
namespace**, not the XR namespace. It is private (`exportTo: ["."]`), uses SIMPLE
TLS on upstream 443, and explicitly sets external SNI, the exact DNS SAN and
`insecureSkipVerify: false`. SNI selects the upstream virtual host; SAN validation
checks its certificate identity. Never disable verification to repair a mismatch.
The example workloadSelector limits policy to the same gateway labels as the
profile. Such a selector is optional for gateway credentialName support, but only
selects workloads in the rule's own namespace. It is not a cross-namespace shortcut.

- **Public CA:** `public-ca.yaml` relies on the gateway proxy's system/OS CA roots.
  There is no custom Secret, credentialName or CA-file path. Verify those roots are
  installed and suitable for the actual server chain.
- **Custom trust:** `custom-trust.yaml` contains an Opaque Secret with **only
  `ca.crt`** and a corresponding external-host rule. Its replacement text is
  deliberately **invalid PEM and not install-ready**. Replace it in the platform
  repository with an approved public PEM trust certificate/bundle before use.
  This requires no client certificate, client private key, upstream server key or
  CA signing key. Do not add `tls.crt`/`tls.key` or change SIMPLE to MUTUAL.
  Both Secret and rule stay in `gateway-regional` for this fixture.
- `credentialName: istio-secure-egress-upstream-trust` references the ordinary
  Secret name. Do not append SDS's internal `-cacert` suffix, create a competing
  suffixed Secret, or reference a Secret in the application namespace. Do not also
  set file-based CA/client certificate/key options. Custom trust selects the
  supplied bundle; do not assume it automatically merges with system public roots.
- For a **private-CA-issued leaf**, trust the approved root/appropriate CA bundle;
  the server must supply the required intermediate chain. For a **directly trusted
  self-signed leaf**, the approved public leaf certificate itself can be the trust
  anchor. It is not a reusable issuing CA: replacing that leaf generally requires
  updating trust too. In both cases verify DNS SAN, validity/expiry, intended usage,
  algorithms, chain and compatibility with the deployed proxy. Trusting a
  certificate never repairs a missing/mismatched SAN or expired certificate.

These objects are independently applied platform prerequisites, **never composed**
by the application XR and never included in the definitions Kustomization. The
function neither reads their Secrets nor verifies their existence. See the
[Istio TLS settings](https://istio.io/v1.31/docs/reference/config/networking/destination-rule/#ClientTLSSettings)
and [egress origination task](https://istio.io/v1.31/docs/tasks/traffic-management/egress/egress-gateway-tls-origination/).
Research used Istio 1.31; check actual installed CRDs/control-plane/proxy versions.

## Platform onboarding and installation order

Gateway Helm/Argo CD ownership is separate from this Composition. Actual profiles,
XRs, upstream rules/trust and GitOps wiring belong to the platform's
`k8s-crossplane` convention; this repository does not modify it. Operators must
coordinate these independently reconciled units before enabling application traffic:

1. Verify Crossplane **v2** supports namespaced composites, composing native
   namespaced Kubernetes resources, and the required-resources protocol. Install
   `apiextensions.crossplane.io/v1beta1` EnvironmentConfig and
   `networking.istio.io/v1` ServiceEntry, Gateway, DestinationRule and VirtualService
   APIs. This uses Istio networking Gateway, not Kubernetes Gateway API; no provider
   wrapper or mandatory environment-loading Function is used.
2. Before XRs reconcile, grant the **Crossplane controller** read access
   (`get`, `list`, `watch`) to EnvironmentConfigs for required-resource resolution,
   and the native composition permissions to get/list/watch/create/update/patch/delete
   the four Istio resource kinds in participating application namespaces. Follow
   the installed Crossplane version's RBAC aggregation mechanism. The function
   uses supplied resources, not a direct Kubernetes client or new Secret-reading
   credentials. Platform profile/trust mutation rights must remain restricted.
3. Independently provision/verify the existing gateway Deployment and Service.
   Service endpoints and profile selector must target the same intended pods.
   Verify the exposed **Service port**, its targetPort mapping and the compatible
   HTTPS/ISTIO_MUTUAL listener that terminates mTLS and routes HTTP. Service port
   80 can map to targetPort 8080; regional 8443 is illustrative, not a claim about
   an installed listener. Adding a Gateway does not open a Service port. An
   east-west profile/name does not make an east-west-only gateway compatible.
4. Check the governance/security prerequisites below. Arrange DNS, gateway upstream
   connectivity and verified TLS trust. Install the approved custom trust Secret
   before its external-host DestinationRule; public trust needs no custom Secret.
   Make the platform rule available before sending application traffic.
5. Install the shared Function once, independently from definitions. Replace the
   **unpublished package placeholder** in `functions.yaml` with a real, separately
   published compatible package and adapt/remove local Development render
   annotations for the installation. This project supplies no package build/publish
   automation or release. Local native renders do not pull a package.
6. Install the definitions-only base (local preview: `kubectl kustomize .`), then
   apply the selected platform EnvironmentConfig **before** its application XR.
   Prepare namespaces beforehand; none are created by the function. Keep profiles,
   Function, RBAC, application instances and platform TLS/trust out of that base.
   Definitions releases follow immutable `istio-secure-egress-vMAJOR.MINOR.PATCH`
   remote-base refs as described in the [root README](../README.md); no such tag
   or runtime package is published by this example.
7. Observe each consumer's configuration and perform the operational checks below
   before considering the traffic path verified. Application provisioning readiness
   does not gate on independently owned platform rules or successful requests.

### Governance and security prerequisites

The function does not discover/enforce global ownership conflicts or change these settings:

- Enforce **one owning XR per exact hostname on each shared gateway/listener**, even
  across application namespaces. Reserve generated names and subsets. Audit duplicate
  host ownership, overlapping wildcard/listener ports/binds, incompatible TLS/plaintext
  servers, conflicting mesh or gateway VirtualServices, duplicate subset definitions,
  and exact/wildcard/root-namespace DestinationRules that shadow intended policy.
  Reject competing top-level gateway-Service traffic policies. Each application
  rule contributes only its own label-less subset, referenced explicitly by its
  route; subset uniqueness is not listener/hostname ownership enforcement.
- Set/check **`PILOT_SCOPE_GATEWAY_TO_NAMESPACE=false`**: application-local Gateway
  objects must select proxy pods in another namespace. Use unique, specific gateway
  labels across namespaces to avoid selecting unintended gateways under this global
  setting. Neither exports nor qualified Gateway references override it.
- Verify functioning **sidecar injection and source traffic capture**, selected
  Service endpoints/ports and actual cluster DNS suffix. The application must send
  HTTP on port 80 with the external authority, not application HTTPS. Redirects to
  HTTPS can move traffic onto a different path; this is not TLS passthrough or
  end-to-end application encryption, and is not an ambient/waypoint example.
- Verify the gateway Service's export visibility, application/gateway Sidecar
  imports for required services/ports, discovery selectors and any mesh
  ServiceEntry visibility restrictions (including `serviceEntryVisibility` where
  enabled). ServiceEntry/VirtualService explicitly export to application and gateway
  namespaces; the gateway-Service DestinationRule remains application-private.
  Imports cannot restore resources excluded by exports or discovery settings.
- Profile selection and `exportTo` are **not authorization**. Restrict profile
  selection through platform admission/RBAC if required. Separately own any
  AuthorizationPolicy restricting authenticated client identities: mTLS authenticates
  a client but does not itself allowlist that client.
- Enforce direct-bypass denial with independent NetworkPolicy/CNI/firewall and
  administrative controls. `REGISTRY_ONLY` is not a firewall and `exportTo` is not
  packet filtering. Permit necessary DNS/control-plane dependencies and intended
  gateway paths without permitting application direct external egress. Restrict
  changes to gateway labels, capture exclusions, privileged/host-network workloads,
  network policies and trust. Validate actual CNI/port behavior separately.

### Changes, trust rotation and removal

App and platform owners must jointly approve hostname -> gateway -> trust ownership.
For hostname changes, provision the new host's gateway-side rule/trust and resolve
listener/route ownership first, update the XR, then verify reconciliation and traffic
before retiring old prerequisites. For profile edits/migrations, prepare target
infrastructure, policies/trust and visibility first. Prefer a **new profile name**
for staged migration; move/reconcile consumers individually and retain old profile,
listeners, rules and trust until no consumers depend on them. In-place profile edits
can affect every consumer independently of its Composition revision. No immediate
fan-out, atomic change, automatic rollback or zero-downtime guarantee is provided.

Trust Secret updates propagate dynamically through **SDS**, not a manually copied
private key. Where appropriate, distribute an approved overlapping old/new trust
bundle first, verify every selected gateway proxy has adopted it, rotate the upstream
certificate, verify new handshakes, then remove retired trust after the agreed
operational window. Direct self-signed leaf replacement needs the new leaf anchor
distributed before switching servers. Overlap broadens trust temporarily and must be
bounded by platform policy. SDS propagation is not bounded or atomic; existing
connections may survive changes, removal is not immediate revocation, and updates
can cause outages. Provisioning readiness cannot prove proxy adoption.

On removal, stop/migrate application use, remove the XR and verify composed routing
cleanup, then retire separately owned external rules/trust/profile only after checking
all consumers (including other namespaces). XR deletion neither deletes nor atomically
updates those platform objects; never delete a shared trust Secret still in use.

## Operational verification — manual, NOT executed locally

These require separately authorized cluster access and real installation details.
Local generation, fixture renders and provisioning readiness **do not execute or
prove** any of them:

- [ ] Inspect installed API/control-plane/proxy versions, Crossplane native-resource
  and EnvironmentConfig read RBAC, profile contents and each consumer's reconciliation.
  Verify gateway pods, selectors, Service endpoints/ports/targetPorts, cross-namespace
  selection setting, DNS, exports/imports/discovery and ownership/conflict controls.
- [ ] Run `istioctl analyze` for application/gateway configuration; inspect
  `istioctl proxy-status` for source and gateway xDS synchronization. Use
  `istioctl proxy-config routes`, `listeners`, `clusters` and `secret` on the
  relevant proxies; inspect detailed cluster TLS/validation contexts for the
  gateway subset's ISTIO_MUTUAL/SNI and upstream SIMPLE/exact SNI/SAN/trust. Check
  intended routes, listener bindings and endpoints, not just API-object existence.
- [ ] From a captured application, request HTTP port 80 for each approved public-CA
  and custom-trust hostname. Correlate gateway access logs to demonstrate traversal
  and inspect upstream TLS verification; a successful request alone is insufficient.
- [ ] In controlled negative cases, verify **hostname/SAN mismatch** and **untrusted
  issuer/leaf** each fail. Check expiry handling too; do not enable insecure
  verification as a workaround or test shortcut.
- [ ] Update the approved trust Secret and verify gateway SDS adoption and actual
  fresh TLS handshakes before rotating/removing trust. Inspect active trust/expiry
  state and account for existing connections and propagation delay.
- [ ] Attempt direct/bypass egress and prove independent controls deny it; separately
  test any allowed-client identity AuthorizationPolicy. Record passed/failed checks
  and environment evidence without treating observed-config Ready as traffic health.
