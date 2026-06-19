# Mossdeck

Mossdeck is a personal, self-hosted browser homepage for bookmarks and read-later items. It uses a Go + SQLite backend and a React/Vite frontend with a light neo-brutalist UI.

## Features

- Personal password-lock login with cookie sessions and CSRF protection
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
- **Auth:** single-owner password, bcrypt, server-side sessions, CSRF double-submit cookie

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

Mossdeck is personal-use only: there is no public registration and no username field.

In development, setup is enabled by default. Open the app and create one strong access password. The password must be at least 12 characters and include at least three of lowercase, uppercase, digits and symbols.

For public first-run setup, protect initialization with a setup token:

```bash
APP_ENV=production
APP_SETUP_ENABLED=true
APP_SETUP_TOKEN=<strong-one-time-token>
```

After creating the password, disable setup again. You can also initialize locally and copy/restore the SQLite database to the server.

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

The compose file runs the Mossdeck app container only. Bring your own reverse proxy / HTTPS layer, such as Caddy, Nginx, Traefik, Cloudflare Tunnel, 1Panel, or a VPS panel.

Important production settings:

```bash
APP_ENV=production
APP_SETUP_ENABLED=false
APP_SETUP_TOKEN=
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
- `web/node_modules/` and `web/dist/`
