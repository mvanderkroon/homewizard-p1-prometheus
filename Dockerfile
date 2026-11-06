FROM golang:1.23@sha256:60deed95d3888cc5e4d9ff8a10c54e5edc008c6ae3fba6187be6fb592e19e8c0 as builder
WORKDIR /go/src/github.com/chrisdoc/homewizard-p1-prometheus/

# Copy go mod files first to cache dependencies layer
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build with cache mount for build cache
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /go/src/github.com/chrisdoc/homewizard-p1-prometheus/app

FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=builder /go/src/github.com/chrisdoc/homewizard-p1-prometheus/app .
CMD ["./app"]
