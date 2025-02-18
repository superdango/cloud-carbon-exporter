
FROM golang:1.24-bookworm AS build-env
RUN  go env -w GOCACHE=/go-cache
RUN  go env -w GOMODCACHE=/gomod-cache

WORKDIR /go/src/cloud-carbon-exporter

COPY . .

RUN --mount=type=cache,target=/gomod-cache --mount=type=cache,target=/go-cache \
    go install honnef.co/go/tools/cmd/staticcheck@latest \
    && go mod download      \
    && go vet ./...         \
    && staticcheck ./...    \
    && go test -v ./...        \
    && go build -o /cloud-carbon-exporter -ldflags="-s -w" github.com/superdango/cloud-carbon-exporter/cmd

# Final stage
FROM gcr.io/distroless/cc-debian12:nonroot
ARG TARGET_NAME=cloud-carbon-exporter
COPY --from=build-env /cloud-carbon-exporter /usr/local/bin/cloud-carbon-exporter
ENTRYPOINT ["/usr/local/bin/cloud-carbon-exporter"]