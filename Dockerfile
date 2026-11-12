# syntax=docker/dockerfile:1
# Copyright 2026 Jason Hall
# SPDX-License-Identifier: Apache-2.0
#
# Multi-stage build (docs/DESIGN.md §10.1): Node builds the SPA, Go embeds it
# into a static binary, and the final image is distroless: no shell, no
# package manager, runs as non-root. Base images are pinned by digest.
#
# The build stages run on the build machine's own architecture
# ($BUILDPLATFORM) and Go cross-compiles, so multi-arch builds don't need
# slow emulation.

FROM --platform=$BUILDPLATFORM node:24.22.2-alpine@sha256:a34ab6d7f6f4813f4d0c5469d74685604b962fbdbf5793a7bc7b4998ecb867c8 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" -o /out/chrysalis ./cmd/chrysalis \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=build /out/chrysalis /chrysalis
COPY --from=build --chown=65532:65532 /out/data /data
ENV CHRYSALIS_DATA_DIR=/data \
    CHRYSALIS_LISTEN_ADDR=:8080
USER 65532:65532
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/chrysalis"]
CMD ["serve"]
