# codex-auth-refresher

`codex-auth-refresher` is a Go service that keeps Codex / OpenAI auth JSON files up to date for `cliproxyapi` by refreshing tokens before they expire. It now also ships with an embedded React dashboard at `GET /` for operational visibility.

## What it does
- reads `auth/*.json` created via `./cliproxyapi --codex-login`
- supports both flat and nested auth JSON formats
- derives expiry from JWT `exp` or explicit expiry fields
- refreshes tokens ahead of expiry using the OAuth refresh flow
- can force refresh at least once every `CODEX_REFRESH_MAX_AGE`
- rewrites the same JSON files atomically
- exposes `GET /`, `GET /healthz`, `GET /readyz`, `GET /metrics`, `GET /v1/status`, and `GET /v1/dashboard`

## Security
- Do **not** commit `auth/*.json` to git.
- Do **not** commit `auth/*.bak-*` backup files.
- Auth files are runtime data and must be copied to servers outside git.
- Tokens that were exposed in chats, logs, screenshots, or pasted into tickets should be rotated before production use.
- The web dashboard is read-only and never exposes token values or the auth directory path.

## Repository and container targets
- GitHub repository: `https://github.com/pymba86/codex-auth-refresher`
- GHCR image: `ghcr.io/pymba86/codex-auth-refresher:latest`
- Release tags: `ghcr.io/pymba86/codex-auth-refresher:vX.Y.Z`
- Target runtime: Debian `amd64`

## Local development
Run backend tests:

```bash
go test ./...
```

Run the backend locally against your auth directory:

```bash
go run ./cmd/codex-auth-refresher --auth-dir ./auth
```

### React dashboard development
Install frontend dependencies:

```bash
npm ci --prefix web
```

Run the Vite dev server:

```bash
npm run dev --prefix web
```

The Vite dev server proxies the backend routes:
- `/v1/dashboard`
- `/v1/status`
- `/healthz`
- `/readyz`
- `/metrics`

Build the embedded dashboard bundle manually:

```bash
npm run build --prefix web
```

Useful endpoints once the backend is running:

```bash
curl http://127.0.0.1:8080/
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/v1/dashboard
curl http://127.0.0.1:8080/v1/status
```

## Local Docker build
Build the image locally:

```bash
docker build -t codex-auth-refresher .
```

For bind-mounted auth files, the container writes as UID/GID `1000:1000` unless you override the user in compose. Make sure the host directory is writable by that user before starting the container.

Run it locally with Docker and the dashboard enabled:

```bash
mkdir -p auth
chmod 755 auth
sudo chown -R 1000:1000 auth

docker run --rm \
  -p 8080:8080 \
  -v "$(pwd)/auth:/data/auth" \
  -e CODEX_AUTH_DIR=/data/auth \
  -e CODEX_WEB_ENABLE=true \
  codex-auth-refresher
```

Or use the local compose example:

```bash
docker compose -f docker-compose.example.yml up -d
```

## Web dashboard
The embedded dashboard is a React single-page app inspired by the visual style of `Cli-Proxy-API-Management-Center`, adapted for a read-only operational view.

When `CODEX_WEB_ENABLE=true`, open:

```bash
http://127.0.0.1:8080/
```

The dashboard shows:
- service readiness and uptime
- refresh policy and current runtime settings
- summary cards for tracked files and failures
- a filterable table of auth files
- operational metrics and endpoint shortcuts

The dashboard fetches data from `GET /v1/dashboard` every 10 seconds.

## GitHub Actions and publishing
Two workflows are included:

- `CI` runs on pull requests, pushes to `main`, and version tags `v*`
- `Docker Publish` builds and publishes the GHCR image on `main`, version tags `v*`, and manual `workflow_dispatch`

Both workflows now build the React frontend before running Go tests and Docker image validation.

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

To enable the dashboard in this public-facing compose, add `CODEX_WEB_ENABLE=true` explicitly and then open `http://HOST:8080/`.

## Running alongside cli-proxy-api
If your server already runs `cli-proxy-api`, use `docker-compose.cliproxyapi.yml` as the starting point for a shared stack. It mounts the same host `./auth` directory into both containers so the refresher updates the files that `cli-proxy-api` already uses.

If `cli-proxy-api` tends to demand a fresh `--codex-login` about every 24 hours even though the auth JSON shows a much longer JWT expiry, enable the max-age mode to force periodic refreshes from the stored `refresh_token`.

Recommended sidecar defaults:
- `CODEX_REFRESH_BEFORE=6h` refreshes comfortably before honest token expiry.
- `CODEX_REFRESH_MAX_AGE=20h` forces one refresh roughly once per day even when the JWT expiry looks much longer.
- `CODEX_SCAN_INTERVAL=5m` keeps token checks cheap while still reacting quickly enough.
- `CODEX_MAX_PARALLEL=1` avoids unnecessary concurrent refreshes for a single shared auth directory.
- `CODEX_HTTP_TIMEOUT=30s` is more forgiving on remote hosts and filtered networks.
- `CODEX_LOG_FORMAT=json` keeps logs ready for Docker and log shippers.
- `CODEX_STATUS_ENABLE=true` keeps `/v1/status` available for local diagnostics.
- `CODEX_WEB_ENABLE=true` enables the embedded dashboard at `GET /`.

Open the local-only sidecar dashboard at:

```bash
http://127.0.0.1:18081/
```

Useful checks:

```bash
curl http://127.0.0.1:18081/
curl http://127.0.0.1:18081/healthz
curl http://127.0.0.1:18081/readyz
curl http://127.0.0.1:18081/v1/dashboard
curl http://127.0.0.1:18081/v1/status
```

Because `cli-proxy-api` usually writes the shared auth directory as `root`, the sidecar example uses `user: "0:0"` for compatibility. If you later make `./auth` consistently writable by UID/GID `1000:1000`, you can remove that override and run the refresher with its default non-root image user.

Important: container `/etc/hosts` entries are isolated, so the refresher needs the same `extra_hosts` mappings as `cli-proxy-api` when you rely on those custom OpenAI DNS overrides.

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
| `CODEX_REFRESH_MAX_AGE` | `--refresh-max-age` | disabled | Force refresh after a maximum token age, even if JWT expiry is still far away |
| `CODEX_SCAN_INTERVAL` | `--scan-interval` | `1m` | Periodic scan interval |
| `CODEX_MAX_PARALLEL` | `--max-parallel` | `4` | Max concurrent refresh jobs |
| `CODEX_HTTP_TIMEOUT` | `--http-timeout` | `15s` | HTTP client timeout |
| `CODEX_TOKEN_ENDPOINT` | `--token-endpoint` | `https://auth.openai.com/oauth/token` | OAuth token endpoint |
| `CODEX_CLIENT_ID` | `--client-id` | from JWT | Fallback client id |
| `CODEX_CA_FILE` | `--ca-file` | — | Extra CA PEM file |
| `CODEX_LOG_FORMAT` | `--log-format` | `json` | `json` or `text` |
| `CODEX_STATUS_ENABLE` | `--status-enable` | `true` | Enable `GET /v1/status` |
| `CODEX_WEB_ENABLE` | `--web-enable` | `false` | Enable the embedded React dashboard at `GET /` |

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
