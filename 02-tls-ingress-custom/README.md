# TLS Ingress with a custom Go function

Same composition behavior as `../01-tls-ingress`, without inline Go templates.
`function/fn.go` builds the Certificate and Ingress directly using the Crossplane
Go function SDK. The XRD and XR inputs are unchanged; the new Composition has a
distinct name, selected explicitly by `examples/xr.yaml`.

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
- `status.certificate.ready` is true only for Certificate `Ready=True`.
- `daysRemaining` is `(expiry.Unix() - now.Unix()) / 86400`, truncated toward zero.
- Health precedence: expired at or before now → `Expired`; at most 15 whole days
  remaining → `Warning`; otherwise ready → `Healthy`; otherwise → `Pending`.
- Missing expiry yields `Pending`, zero days, and no `notAfter`, even when ready.
- Malformed expiry preserves the template's Sprig `toDate` behavior: parsing with
  `2006-01-02T15:04:05Z` falls back to Go's zero time and yields `Expired`.
- Unrelated incoming desired resources and composite status fields are preserved.

There is deliberately no auto-ready pipeline step: the original example declares
that function but never calls it. Certificate status is not composite readiness.

## Tests

```sh
cd function
go test -race ./...
go vet ./...
```

Tests use a fixed clock for exact expiry/warning boundaries and exercise the
function request/response, rendered resources, status, and desired-state preservation.
The observed YAML fixtures use distant dates so normal rendering stays useful;
the warning boundary is covered deterministically by the Go tests.

## Build and publish

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

The XRD is shared with `01-tls-ingress`; do not delete it when switching examples.
This example does not change the original XRD's scope or compatibility assumptions.
It also preserves the original schema's known omission of `Expired` from the
health enum, even though both implementations emit that value. Local rendering
does not validate it; a cluster deployment needs that enum value added to the
shared XRD if it must accept expired certificate status.
