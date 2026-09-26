# syntax=docker/dockerfile:1
# Build the node as a static binary, then ship it on nothing.
#
# The result is a single executable with no libc, no shell, and no package
# manager, which is both small and a much smaller thing to keep patched.
FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first, so a code change does not re-download the module cache.
COPY go.mod go.sum ./
# The module cache is a BuildKit cache mount, so a no-cache rebuild does not
# re-fetch every dependency, and a dropped download is retried.
RUN --mount=type=cache,target=/go/pkg/mod \
    for i in 1 2 3 4; do go mod download && exit 0; sleep 10; done; exit 1

COPY . .
# CGO off matters here: the sqlite driver is a pure-Go WASM build, so the whole
# binary links statically and needs nothing from the base image.
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w" -o /out/lamdis ./cmd/lamdis

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lamdis /lamdis
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/lamdis"]
CMD ["exchange"]
