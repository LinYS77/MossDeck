# Mossdeck

Mossdeck is a personal, self-hosted browser homepage for bookmarks and read-later items. It uses a Go + SQLite backend and a React/Vite frontend with a light neo-brutalist UI.

## Features

- Personal login with cookie sessions and CSRF protection
- Bookmark categories, tags, search, archive/trash, favorites and pinned items
- Browser bookmark HTML import
- Read-later queue with tags, states, priorities and domain filtering
- JSON backup/export and restore/import
- Chinese / English UI switching
- Optional wallpaper layer with film-grain texture
- Docker Compose + Caddy example for self-hosting

## Stack

- **Backend:** Go, `net/http`, SQLite, embedded migrations
- **Frontend:** React, TypeScript, Vite, CSS Modules
- **Storage:** SQLite database file
- **Auth:** bcrypt passwords, server-side sessions, CSRF double-submit cookie

## Quick Start

```bash
# Backend
cp .env.example .env   # optional in development
make run
```

```bash
# Frontend dev server
cd web
npm install
npm run dev
```

Open the Vite URL, usually `http://localhost:5173`.

The dev frontend proxies `/api` to the Go server on `http://localhost:8080`.

## First Login

In development, setup is enabled by default:

```bash
APP_ENV=development APP_SETUP_ENABLED=true go run ./cmd/server
```

Create the first admin through the UI, or call:

```bash
curl -i -c cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me-now","displayName":"Admin"}' \
  http://localhost:8080/api/v1/auth/setup
```

For public deployment, keep setup disabled and use an already-initialized database or restore from a trusted backup.

## Build

```bash
# Backend checks
go test ./...
go build ./...

# Frontend build
cd web
npm run build
```

## Deployment

A minimal self-hosting setup is included:

- `Dockerfile`
- `compose.yaml`
- `Caddyfile`

Important production settings:

```bash
APP_ENV=production
APP_SETUP_ENABLED=false
APP_SESSION_SECRET=<at-least-32-bytes>
APP_DATABASE_PATH=/data/mossdeck.db
APP_COOKIE_SECURE=true
```

If running behind a reverse proxy, set `APP_TRUSTED_PROXY_CIDRS` to the proxy CIDR and ensure the proxy overwrites forwarded headers.

## Data

Runtime data is stored in SQLite and ignored by Git:

```text
/data/
*.db
*.db-wal
*.db-shm
```

Use Settings → Backup to export/import bookmarks, tags, categories and read-later data as JSON.

## Repository Notes

Ignored local-only assets include:

- `/data/` runtime database
- `/backups/` local backups
- `/erro/` screenshots/reference images
- `/wallpaper/` large original wallpaper sources
- `web/node_modules/` and `web/dist/`

The optimized frontend wallpapers live in `web/public/wallpaper/` and are tracked.
