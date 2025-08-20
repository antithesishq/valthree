FROM docker.io/library/golang:1.25-bookworm AS builder
LABEL maintainer="Antithesis <support@antithesis.com>"

WORKDIR /workspace

COPY go.mod go.sum /workspace/
RUN go mod download

COPY *.go /workspace/
COPY internal /workspace/internal

RUN go build -race -trimpath -buildvcs=false -o /go/bin/valthree .

FROM docker.io/library/debian:bookworm-slim
LABEL maintainer="Antithesis <support@antithesis.com>"

RUN apt update && apt install -y --no-install-recommends redis-tools && rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/bin/valthree /usr/local/bin/valthree

ENTRYPOINT ["/usr/local/bin/valthree", "serve", "--json"]
