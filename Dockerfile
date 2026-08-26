# syntax=docker/dockerfile:1
#
# Grok Build WebUI — container image
#
# Multi-stage build:
#   1. Build the static Go binary (modernc.org/sqlite => CGO_ENABLED=0).
#   2. Runtime image with the tools grok sessions expect on hand
#      (git, ripgrep, bash, node, python, ssh ...).
#
# The container is meant to behave like a native service: it runs as your
# host user and bind-mounts your home directory at the SAME path, so project
# paths (and ~/.grok/bin/grok) resolve identically inside the container.

# ---------- stage 1: build ----------
FROM golang:1.25-alpine AS build

# pass e.g. --build-arg VERSION=v1.2.3 when building from a tag
ARG VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY web/ web/

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/grok-webui ./cmd/server

# ---------- stage 2: runtime ----------
FROM alpine:3.21

LABEL org.opencontainers.image.title="grok-build-webui" \
      org.opencontainers.image.description="Grok Build CLI session multiplexer (tabs + splits) with auth" \
      org.opencontainers.image.source="https://github.com/karutoil/grok-build-webui"

# Tooling available inside grok PTY sessions. Trim or extend to taste —
# anything under $HOME still comes from the host via the bind mount.
RUN apk add --no-cache \
        bash \
        ca-certificates \
        coreutils \
        curl \
        git \
        jq \
        nodejs \
        npm \
        openssh-client-default \
        python3 \
        ripgrep \
        tzdata \
        unzip \
        zip

COPY --from=build /out/grok-webui /app/grok-webui

# data dir lives on a bind mount; just make sure the default path exists
RUN mkdir -p /app/data && chown -R nobody:nogroup /app

WORKDIR /app
EXPOSE 8080

ENTRYPOINT ["/app/grok-webui"]
CMD ["--addr", ":8080", "--data", "/app/data"]
