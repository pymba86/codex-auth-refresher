FROM node:22-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY README.md ./README.md
COPY --from=web-builder /src/internal/httpapi/webdist/index.html /src/internal/httpapi/webdist/index.html
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/codex-auth-refresher ./cmd/codex-auth-refresher

FROM alpine:3.20
RUN addgroup -g 1000 -S app && adduser -S -D -H -u 1000 -G app app
WORKDIR /app
COPY --from=builder /out/codex-auth-refresher /usr/local/bin/codex-auth-refresher
USER app
EXPOSE 8080
ENV CODEX_AUTH_DIR=/data/auth
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/codex-auth-refresher"]
