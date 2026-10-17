# syntax=docker/dockerfile:1
# Aligned with the repository's Go toolchain.
ARG GO_VERSION=1.27.1
FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS build
WORKDIR /fn
ENV CGO_ENABLED=0
RUN --mount=target=. --mount=type=cache,target=/go/pkg/mod go mod download
ARG TARGETOS
ARG TARGETARCH
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /function .
FROM gcr.io/distroless/static-debian12:nonroot AS image
WORKDIR /
COPY --from=build /function /function
EXPOSE 9443
USER nonroot:nonroot
ENTRYPOINT ["/function"]
