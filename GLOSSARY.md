# Glossary

- **Provisioning readiness**: whether Crossplane's composed resources have
  reached the desired infrastructure configuration. For the custom TLS ingress
  composite, this is derived from current, unexpired Certificate readiness and
  an Ingress whose relevant observed configuration matches the desired spec.
- **Application health**: whether traffic can reach and successfully use the
  backend over HTTPS. This is distinct from provisioning readiness and is not
  inferred from Certificate readiness or Ingress configuration alone.
- **Composed resource**: a namespaced managed or Kubernetes resource created by
  a composite resource through its Composition.
