# This Dockerfile is used by GoReleaser to build multi-arch images.
# The binary is built by GoReleaser and copied into the image.
FROM gcr.io/distroless/static-debian12:nonroot

COPY observability-federation-proxy /proxy

USER nonroot:nonroot

ENTRYPOINT ["/proxy"]
