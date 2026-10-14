# TLS Ingress with a custom Go function

Same composition behavior as `../01-tls-ingress`, without inline Go templates.
`function/fn.go` builds the Certificate and Ingress directly using the Crossplane
Go function SDK. This example uses a separate namespaced `XIngressTLSCustom` XRD
with the same input schema. Its Composition has a distinct name, selected
explicitly by `examples/xr.yaml`.

The example XR is `default/tls-ingress-custom-demo` with hostname `app-custom.mujak.com`.
It creates Certificate and Ingress `default/tls-ingress-custom-demo`, using TLS Secret
`default/tls-ingress-custom-demo-tls`, so it can coexist with example 01's
`default/tls-ingress-demo`.
For additional XRs, use distinct names within the namespace (even across XR
kinds) and distinct ingress hostnames. Configure DNS for the custom hostname
before requesting a certificate. The backend service and issuer are shared
dependencies, not resources created by this example.

`kubectl kustomize .` renders only the XRD and Composition. Install Functions and
RBAC separately. See the root [naming convention](../README.md#naming-and-installation).

## Local rendering (no cluster required)

Requires Go 1.26+ (and a toolchain compatible with the dependencies) and the
Crossplane CLI. Current CLI versions also use Docker for the render engine (or
accept `--crossplane-binary` pointing to a compatible Crossplane server binary).
The function itself runs locally. From this directory, run both with:

```sh
make render
make render RENDER_ARGS="--verbose --observed-resources examples/observed/healthy.yaml"
```

The Makefile builds a temporary binary, starts it, waits for port 9443, renders,
and stops the function on completion or failure. Port 9443 must be free.

Alternatively, start it manually in one terminal:

```sh
cd function
go run . --insecure --debug
```

In another terminal, from this directory:

```sh
./render.sh
./render.sh --observed-resources examples/observed/healthy.yaml
./render.sh --observed-resources examples/observed/expired.yaml
./render.sh --observed-resources examples/observed/pending.yaml
```

The CLI connects to `localhost:9443` using the Development runtime annotations
in `functions.yaml`. These annotations affect rendering only; they do not install
the local process in a cluster. The CLI adds `crossplane.io/composition-resource-name`
annotations from the SDK resource keys `certificate` and `ingress`.

## Behavior

- Certificate name and Ingress name match the XR name and namespace.
- The Certificate requests the hostname using the configured Issuer/ClusterIssuer;
  both resources reference `<xr-name>-tls`.
- The Ingress routes `/` with `Prefix` to the configured service and port.
  An omitted ingress class is left unset.
- Observed Certificate `status.notAfter`, when present, is copied to the Ingress
  annotation `platform.mujak.com/cert-not-after` and XR certificate status.
- Certificate provisioning readiness requires `Ready=True` with
  `observedGeneration` equal to the Certificate's current `metadata.generation`,
  and a valid `status.notAfter` strictly in the future. Missing, stale, invalid,
  or expired data keeps the composed Certificate unready.
- `daysRemaining` is `(expiry.Unix() - now.Unix()) / 86400`, truncated toward zero.
- Health precedence: expired or invalid expiry → `Expired`; at most 15 whole
  days remaining → `Warning`; otherwise ready → `Healthy`; otherwise → `Pending`.
- Near-expiry certificates remain provisioned and ready while health reports
  `Warning`. Missing expiry yields `Pending`, zero days, and no `notAfter`.
- Ingress provisioning readiness requires its observed host, TLS host and Secret,
  paths/path types, backend service/port, and any explicitly requested class to
  match the desired configuration. Empty load-balancer status is acceptable;
  controller metadata/status and an admission-defaulted class are ignored.
- Backend Service/endpoints availability and HTTPS/application health are
  separate from composite provisioning readiness.
- Unrelated incoming desired resources and composite status fields are preserved.

The function explicitly reports readiness for the Certificate and Ingress;
Crossplane derives composite provisioning readiness from those composed
resources. It does not force the XR ready. Application health is a separate
signal and is not inferred from an empty Ingress load balancer or Certificate
condition alone.

## Tests

```sh
cd function
go test -race ./...
go vet ./...
```

Tests use a fixed clock for exact expiry/warning boundaries and exercise the
function request/response, readiness transitions, rendered resources, status,
and desired-state preservation.
The observed YAML fixtures use distant dates so normal rendering stays useful;
the warning boundary is covered deterministically by the Go tests.

## Build and publish

### GitHub Actions and GHCR

The workflow in `../.github/workflows/function-tls-ingress.yaml` tests, vets,
builds the runtime image, and embeds it in an installable Crossplane xpkg.
It targets **linux/amd64**; ARM64 clusters need a separate ARM64 build.
Go is pinned to 1.27.1 and the Crossplane CLI to v2.5.0. When upgrading Go,
update both the workflow and the Dockerfile default; when upgrading the CLI,
update its version and linux/amd64 SHA256 in the workflow.

- Pull requests changing this example or its workflow build without publishing.
- Manual runs from GitHub's Actions tab also build without publishing.
- Pushing `function-tls-ingress-vMAJOR.MINOR.PATCH` publishes the tested package
  to `ghcr.io/mujak27/function-tls-ingress:vMAJOR.MINOR.PATCH`.

After merging the workflow, release from the repository root:

```sh
git switch main
git pull --ff-only
git tag function-tls-ingress-v0.1.0
git push origin function-tls-ingress-v0.1.0
```

Publishing uses the repository's automatic `GITHUB_TOKEN` with `packages: write`
only in the release job; no PAT or additional repository secret is required.
The runtime image is embedded, not published separately. The final package
carries a source-repository annotation and is published from this repository's
workflow so GitHub can associate the package with the repository.

**One-time setup after the first release:** open your GitHub profile's Packages
page, select `function-tls-ingress`, and change its visibility to **Public** in
Package settings. New GHCR packages are private by default, even for public
repositories. Confirm the package is linked to `mujak27/crossplane-compilations`;
if a package with this name already exists, grant this repository Actions access
before publishing. Public visibility lets Crossplane pull without credentials.
GHCR container storage and bandwidth are currently free; standard GitHub-hosted
Actions runners are free for this public repository.

`functions.yaml` references `v0.1.0`; it cannot be installed until that release
has been published and made public. Update the reference for later releases.
The Development annotations remain for local rendering only. For a cluster
smoke test after publishing:

```sh
kubectl apply -f functions.yaml
kubectl wait --for=condition=Installed function/function-tls-ingress --timeout=120s
kubectl wait --for=condition=Healthy function/function-tls-ingress --timeout=120s
```

See [GHCR authentication and visibility](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
and [GitHub Packages billing](https://docs.github.com/en/packages/learn-github-packages/introduction-to-github-packages).

### Manual publishing

From this directory (replace the registry with one you own):

```sh
docker build -t function-tls-ingress:runtime ./function
crossplane xpkg build --package-root function/package \
  --embed-runtime-image function-tls-ingress:runtime \
  --package-file function/function-tls-ingress.xpkg
crossplane xpkg push --package-files function/function-tls-ingress.xpkg \
  registry.example.com/your-org/function-tls-ingress:v0.1.0
```

Update `spec.package` in `functions.yaml` to that published package before
installing. Then apply `functions.yaml`, `xrd.yaml`, `composition.yaml`, and the
example XR to your suitably configured cluster. Like the original, this example
assumes support for composing native Kubernetes resources, cert-manager and its
issuer, the ingress controller and backend service, and appropriate Crossplane
RBAC for Certificates and Ingresses.

The XRD is independent of `01-tls-ingress`; installing it does not modify the
original example's XRD. Both use namespaced Crossplane v2 composites.
Both XRD health enums include `Expired`, matching the values emitted by the
implementations. Consumers of these definitions no longer need a local enum patch.
