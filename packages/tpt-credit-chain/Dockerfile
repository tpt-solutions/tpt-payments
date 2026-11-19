# Multi-stage build for TPT Credit chain binary.
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make build-base

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/tptcreditd ./cmd/tptcreditd

# Runtime stage.
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

COPY --from=builder /usr/local/bin/tptcreditd /usr/local/bin/tptcreditd

# Create default config directory.
RUN mkdir -p /root/.tptcredit/config /root/.tptcredit/data

# Expose ports:
# 26656 - P2P
# 26657 - RPC
# 1317  - REST API
# 9090  - gRPC
# 26660 - Prometheus metrics
EXPOSE 26656 26657 1317 9090 26660

ENTRYPOINT ["tptcreditd"]
CMD ["--help"]