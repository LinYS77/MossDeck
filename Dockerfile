# Multi-stage Dockerfile for the homepage project.
#
# Stage 1 – build React frontend
# Stage 2 – build Go server (CGO_ENABLED=0, static binary)
# Stage 3 – minimal Alpine runtime with the binary + static assets
#
# Build:  docker build -t homepage .
# Run:    docker run -p 8080:8080 -v ./data:/data --env-file .env homepage

# =============================================================================
# Stage 1: Build frontend
# =============================================================================
FROM node:22-alpine AS web
WORKDIR /src

COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# =============================================================================
# Stage 2: Build Go server
# =============================================================================
FROM golang:1.25-alpine AS gobuild
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
COPY --from=web /src/dist ./web/dist

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

# =============================================================================
# Stage 3: Minimal runtime
# =============================================================================
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=gobuild /server /usr/local/bin/server
COPY --from=gobuild /src/web/dist /srv/web

ENV APP_STATIC_DIR=/srv/web
ENV APP_DATABASE_PATH=/data/homepage.db
ENV APP_ENV=production

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/server"]
