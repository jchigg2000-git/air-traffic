# syntax=docker/dockerfile:1
# Multi-target build: `--target server` (control plane, bakes the SPA) and
# `--target gateway` (inference data plane). Both are CGO-free static Go
# binaries on alpine (busybox wget serves the compose healthchecks).

FROM node:22-alpine AS webbuild
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS gobuild
WORKDIR /src
# stdlib-only module: go.mod is the entire dependency manifest (no go.sum)
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/air-traffic-server ./cmd/air-traffic-server && \
    CGO_ENABLED=0 go build -trimpath -o /out/air-traffic-gateway ./cmd/air-traffic-gateway

FROM alpine:3.24 AS server
RUN adduser -D -u 10001 airtraffic
WORKDIR /app
COPY --from=gobuild /out/air-traffic-server /usr/local/bin/air-traffic-server
# the binary serves web/dist relative to its working directory
COPY --from=webbuild /src/web/dist /app/web/dist
# data/ is the harness's durable state; owned by the runtime user so the
# named volume inherits writable permissions on first mount
RUN mkdir -p /app/data && chown -R airtraffic /app/data
USER airtraffic
EXPOSE 8122
ENTRYPOINT ["air-traffic-server"]

FROM alpine:3.24 AS gateway
RUN adduser -D -u 10001 airtraffic
WORKDIR /app
COPY --from=gobuild /out/air-traffic-gateway /usr/local/bin/air-traffic-gateway
USER airtraffic
EXPOSE 8125
ENTRYPOINT ["air-traffic-gateway"]
