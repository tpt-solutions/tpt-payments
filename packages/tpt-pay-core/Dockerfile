# Production image for tpt-pay-core.
#
# Build (from the repository root):
#   docker build -f packages/tpt-pay-core/Dockerfile \
#     --build-arg VERSION=v0.1.0 --build-arg COMMIT=$(git rev-parse HEAD) \
#     -t tpt-pay-core:local packages/tpt-pay-core
#
# The final image is distroless: no shell, no package manager, non-root.

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/tpt-pay-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tpt-pay-server /tpt-pay-server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/tpt-pay-server"]
