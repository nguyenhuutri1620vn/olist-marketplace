# Build context is the repo root, so the Go workspace and shared modules resolve.
FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -o /out/olist-importer ./tools/olist-importer && mkdir /out/data

# The CSVs are not baked in: `make import` copies them into /data (Docker here
# cannot bind-mount paths under ~/Documents).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/olist-importer /olist-importer
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENTRYPOINT ["/olist-importer"]
