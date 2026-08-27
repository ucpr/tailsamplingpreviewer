# Multi-target Dockerfile for the Tail Sampling Preview stack. Each
# runnable piece (tailpreview server, otelcol-tailpreview collector,
# loadgen) is built as its own final stage, selected in
# examples/compose.yaml via `build.target`.

FROM node:22-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY exporter/ exporter/
COPY examples/loadgen/ examples/loadgen/
ENV CGO_ENABLED=0

FROM go-build AS build-tailpreview
RUN go build -trimpath -ldflags="-s -w" -o /out/tailpreview ./cmd/tailpreview

FROM go-build AS build-otelcol
RUN go build -trimpath -ldflags="-s -w" -o /out/otelcol-tailpreview ./cmd/otelcol-tailpreview

FROM go-build AS build-loadgen
RUN go build -trimpath -ldflags="-s -w" -o /out/loadgen ./examples/loadgen

FROM alpine:3.22 AS tailpreview
RUN apk add --no-cache wget
COPY --from=build-tailpreview /out/tailpreview /usr/local/bin/tailpreview
COPY --from=web-build /web/dist /web/dist
ENTRYPOINT ["/usr/local/bin/tailpreview", "--web-dir=/web/dist"]

FROM alpine:3.22 AS otelcol
COPY --from=build-otelcol /out/otelcol-tailpreview /usr/local/bin/otelcol-tailpreview
ENTRYPOINT ["/usr/local/bin/otelcol-tailpreview"]

FROM alpine:3.22 AS loadgen
COPY --from=build-loadgen /out/loadgen /usr/local/bin/loadgen
ENTRYPOINT ["/usr/local/bin/loadgen"]
