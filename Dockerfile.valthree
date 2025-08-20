# To minimize startup time in Antithesis, this Dockerfile uses a two-stage
# build to keep the final image small.
FROM docker.io/library/golang:1.25-bookworm AS builder
LABEL maintainer="Antithesis <support@antithesis.com>"

# We'll start by working with the code as it is in Github.
WORKDIR /workspace/uninstrumented

# Download Go dependencies in a separate layer to improve the hit rate of layer
# caching.
COPY go.mod go.sum /workspace/uninstrumented/
RUN go mod download

# Copy the remainder of the code, then add Antithesis instrumentation.
# Instrumentation is similar to `go test -cover`: it allows the Antithesis
# platform to detect which lines of code are exercised, which makes fuzzing
# more effective. It also catalogs all the assertions in the Valthree codebase.
COPY *.go /workspace/uninstrumented/
COPY internal /workspace/uninstrumented/internal
RUN mkdir -p /workspace/instrumented
RUN go run github.com/antithesishq/antithesis-sdk-go/tools/antithesis-go-instrumentor . /workspace/instrumented

# Compile the instrumented code into the final binary, being sure to enable
# Go's race detection.
WORKDIR /workspace/instrumented/customer
RUN go build -race -trimpath -buildvcs=false -o /go/bin/valthree .

# Base the deployable image on a slimmer Debian base. Musl-based distributions
# (like Alpine) don't work well with Antithesis.
FROM docker.io/library/debian:bookworm-slim
LABEL maintainer="Antithesis <support@antithesis.com>"

# In interactive debugging sessions, it's helpful to have redis-cli available.
# When installing it, take care not to leave frequently-changing apt files in
# the layer.
RUN apt update && apt install -y --no-install-recommends redis-tools && rm -rf /var/lib/apt/lists/*

# Copy the binary and the Antithesis symbol data into the final image.
COPY --from=builder /go/bin/valthree /usr/local/bin/valthree
RUN mkdir -p /symbols
COPY --from=builder /workspace/instrumented/symbols /symbols/

ENTRYPOINT ["/usr/local/bin/valthree", "serve", "--json"]
