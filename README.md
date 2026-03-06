# codex-auth-refresher

`codex-auth-refresher` is a Go service that keeps Codex / OpenAI auth JSON files up to date for `cliproxyapi` by refreshing tokens before they expire.

## What it does
- reads `auth/*.json` created via `./cliproxyapi --codex-login`
- supports both flat and nested auth JSON formats
- derives expiry from JWT `exp` or explicit expiry fields
- refreshes tokens ahead of expiry using the OAuth refresh flow
- rewrites the same JSON files atomically
- exposes `GET /healthz`, `GET /readyz`, `GET /metrics`, and `GET /v1/status`

## Security
- Do **not** commit `auth/*.json` to git.
- Do **not** commit `auth/*.bak-*` backup files.
- Auth files are runtime data and must be copied to servers outside git.
- Tokens that were exposed in chats, logs, screenshots, or pasted into tickets should be rotated before production use.

## Repository and container targets
- GitHub repository: `https://github.com/pymba86/codex-auth-refresher`
- GHCR image: `ghcr.io/pymba86/codex-auth-refresher:latest`
- Release tags: `ghcr.io/pymba86/codex-auth-refresher:vX.Y.Z`
- Target runtime: Debian `amd64`

## Local development
Run tests:

```bash
go test ./...
```

Run locally against your auth directory:

```bash
go run ./cmd/codex-auth-refresher --auth-dir ./auth
```

Useful endpoints:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/v1/status
```

## Local Docker build
Build the image locally:

```bash
docker build -t codex-auth-refresher .
```

For bind-mounted auth files, the container writes as UID/GID `1000:1000`. Make sure the host directory is writable by that user before starting the container.

Run it locally with Docker:

```bash
mkdir -p auth
chmod 755 auth
sudo chown -R 1000:1000 auth

docker run --rm \
  -p 8080:8080 \
  -v "$(pwd)/auth:/data/auth" \
  -e CODEX_AUTH_DIR=/data/auth \
  codex-auth-refresher
```

Or use the local compose example:

```bash
docker compose -f docker-compose.example.yml up -d
```

## GitHub Actions and publishing
Two workflows are included:

- `CI` runs on pull requests, pushes to `main`, and version tags `v*`
- `Docker Publish` builds and publishes the GHCR image on `main`, version tags `v*`, and manual `workflow_dispatch`

Published tags:
- `latest` on `main`
- `sha-<shortsha>` on `main`
- `vX.Y.Z`, `vX.Y`, and `vX` on release tags

After the first publish, verify that the GHCR package visibility is set to `public` if GitHub created it as private.

## Remote deployment on Debian amd64
1. Install Docker Engine and the Docker Compose plugin.
2. Create a deployment directory, for example:

```bash
sudo mkdir -p /opt/codex-auth-refresher/auth
cd /opt/codex-auth-refresher
```

3. Copy `docker-compose.ghcr.yml` to the server.
4. Copy your auth JSON files into `/opt/codex-auth-refresher/auth` outside git.
5. Make the auth directory writable for the container user:

```bash
sudo chmod 755 /opt/codex-auth-refresher/auth
sudo chown -R 1000:1000 /opt/codex-auth-refresher/auth
```

6. Pull and start the service:

```bash
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

7. Verify the service:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/v1/status
```

## Updating the server
To update the remote machine after a new image is published:

```bash
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

## Configuration
| Env | Flag | Default | Purpose |
| --- | --- | --- | --- |
| `CODEX_AUTH_DIR` | `--auth-dir` | required | Auth JSON directory |
| `CODEX_LISTEN_ADDR` | `--listen-addr` | `:8080` | HTTP listen address |
| `CODEX_REFRESH_BEFORE` | `--refresh-before` | `6h` | Early refresh threshold |
| `CODEX_SCAN_INTERVAL` | `--scan-interval` | `1m` | Periodic scan interval |
| `CODEX_MAX_PARALLEL` | `--max-parallel` | `4` | Max concurrent refresh jobs |
| `CODEX_HTTP_TIMEOUT` | `--http-timeout` | `15s` | HTTP client timeout |
| `CODEX_TOKEN_ENDPOINT` | `--token-endpoint` | `https://auth.openai.com/oauth/token` | OAuth token endpoint |
| `CODEX_CLIENT_ID` | `--client-id` | from JWT | Fallback client id |
| `CODEX_CA_FILE` | `--ca-file` | — | Extra CA PEM file |
| `CODEX_LOG_FORMAT` | `--log-format` | `json` | `json` or `text` |
| `CODEX_STATUS_ENABLE` | `--status-enable` | `true` | Enable `/v1/status` |

## First push checklist
Before the first public push:

```bash
git init
git branch -M main
git add .
git status --short
```

Verify that `auth/*.json` and `auth/*.bak-*` are **not** staged, then commit and push:

```bash
git commit -m "Prepare project for GitHub and GHCR"
git remote add origin https://github.com/pymba86/codex-auth-refresher.git
git push -u origin main
```
