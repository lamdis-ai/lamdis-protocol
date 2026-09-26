# syntax=docker/dockerfile:1
# Build the node as a static binary, then ship it on nothing.
#
# The result is a single executable with no libc, no shell, and no package
# manager, which is both small and a much smaller thing to keep patched.
FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies come from vendor/ when ops/deploy-app.sh has filled it on the
# host: the builder's own route to the module proxy has proved unreliable, and
# a deploy should not hinge on it. Without vendor/, download with retries.
COPY . .
# CGO off matters here: the sqlite driver is a pure-Go WASM build, so the whole
# binary links statically and needs nothing from the base image.
RUN --mount=type=cache,target=/go/pkg/mod \
    if [ -d vendor ]; then FLAGS=-mod=vendor; else \
      for i in 1 2 3 4; do go mod download && break; sleep 10; done; FLAGS=; fi; \
    CGO_ENABLED=0 GOOS=linux go build $FLAGS -trimpath \
      -ldflags="-s -w" -o /out/lamdis ./cmd/lamdis

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lamdis /lamdis
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/lamdis"]
CMD ["exchange"]
