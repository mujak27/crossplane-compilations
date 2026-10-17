compilation of crossplane custom resources,
demonstrating how crossplane can abstract
complex configuration behind its custom resource

- [01-tls-ingress](01-tls-ingress): TLS Ingress using inline Go templates.
- [02-tls-ingress-custom](02-tls-ingress-custom): the same behavior in a custom Go composition function.
- [03-istio-secure-egress](03-istio-secure-egress): sidecar HTTP through an existing gateway to verified upstream HTTPS, with externally owned gateway profiles and trust.
- [function-template](function-template): an unnumbered, provider-independent custom Go function starter with generated schemas and a namespaced ConfigMap example.

## Creating a function

In OpenCode, invoke `@create-crossplane-function`. The project-local
[Crossplane skill](.opencode/skills/create-crossplane-function/SKILL.md) automatically
loads `plan-to-tickets` for worktree placement, approvals, tickets, and explicitly
requested implementation, while supplying domain, template, and validation
requirements. Install or update `plan-to-tickets` from `mujak27/skills` first;
older user-only versions cannot be loaded automatically. Approval gates remain
unchanged, including explicit invocation of user-only companion skills when prompted.

## Naming and installation

Each project has a stable ID without its numeric folder prefix:

| Project ID | XR kind | XRD name | Composition name |
| --- | --- | --- | --- |
| `tls-ingress` | `XIngressTLS` | `xingresstls.platform.mujak.com` | `xingresstls.platform.mujak.com` |
| `tls-ingress-custom` | `XIngressTLSCustom` | `xingresstlscustom.platform.mujak.com` | `xingresstls-custom.platform.mujak.com` |
| `istio-secure-egress` | `XIstioSecureEgress` | `xistiosecureegresses.platform.mujak.com` | `istio-secure-egress.platform.mujak.com` |
| `function-template` | `XExample` | `xexamples.template.example.org` | `function-template.template.example.org` |

- Independent APIs must have unique group/plural pairs and distinct kinds in this
  collection. An XRD name is always `<plural>.<group>`. Composition type references
  and XR API versions/kinds must match that XRD. Interchangeable implementations
  may intentionally share an API; they must share one owning XRD definition.
- Composition names must be unique cluster-wide. Preserve the existing names
  above; for new projects use `<project-id>.<group>`. Examples select their
  Composition explicitly through `spec.crossplane.compositionRef`.
- Use the project ID in project-owned GitOps Application names, dependency labels,
  and RBAC names. Shared Functions and RBAC have one installation owner; do not
  duplicate them per project. Keep existing shared Function names and package tags.
- Name new XRs `<project-id>-<instance>` and set `metadata.namespace`. Certificate
  and Ingress names equal the XR name; the TLS Secret is `<xr-name>-tls`. Reserve
  those names across XR kinds and other workloads in the same namespace, and avoid
  overlapping ingress hostnames. A new XR kind does not isolate composed resources.
- Keep XRDs at `apiextensions.crossplane.io/v2` with `scope: Namespaced` and compose
  resources in the XR namespace. Do not add namespaces to XRDs, Compositions or
  Functions, which are inherently cluster-scoped.

Secure egress uses `platform.mujak.com/v1alpha1`, Go module
`github.com/mujak27/function-istio-secure-egress`, and separately installed Function
`function-istio-secure-egress`. Its platform-owned EnvironmentConfigs are named
`istio-egress-<profile>`; selecting a profile is not gateway-use authorization.
All four composed Istio objects stay in the XR namespace. Gateway profiles and
gateway-namespace upstream TLS rules/trust are separate platform installation units.

Each project directory is a **definitions-only Kustomize base**, containing its
XRD and Composition. Functions, RBAC and example XRs are installed separately so
shared dependencies can have one owner:

```sh
kubectl kustomize 01-tls-ingress
kubectl kustomize 02-tls-ingress-custom
kubectl kustomize 03-istio-secure-egress
kubectl kustomize function-template
```

Definitions releases use immutable `<project-id>-vMAJOR.MINOR.PATCH` tags,
separate from runtime tags such as `function-tls-ingress-v0.1.0`. After a
definitions tag is published, GitOps can reference a single remote base:

```yaml
resources:
  - https://github.com/mujak27/crossplane-compilations//02-tls-ingress-custom?ref=tls-ingress-custom-v0.1.0
  - rbac.yaml
```

The URL above is a release convention, not an already published tag. Never move
an existing release tag. The older `function-tls-ingress-v0.1.0` source tag has a
custom XRD that shares the original API identity; do not use it as the custom
definitions base. Its runtime package can still be used with the distinct API.
