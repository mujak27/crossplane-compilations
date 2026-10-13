# function-tls-ingress

Go composition function generated from Crossplane's `function-template-go`.
See [the example README](../README.md) for usage and packaging instructions.

The function has no pipeline input: its contract is the `XIngressTLSCustom` schema
in `../xrd.yaml`. It uses observed certificate state, never desired state, to
project expiry and readiness onto the composite and Ingress.
