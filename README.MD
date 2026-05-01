# getaced

A self-hosted Go service that watches a Google Drive folder, sends each new
PNG / HTML / MHTML file to OpenAI for answer extraction, and writes the
results as Chrome bookmarks inside a per-user LXD container that the user
can reach over noVNC in their browser.

Originally built as a SaaS at `api.getaced.io`. It now runs as a private tool
for myself and a friend — access is gated by an email allowlist. The source
is published as-is for transparency. No support, no roadmap, no guarantees.

## How it works

1. User signs in with Google.
2. User links a Google Drive account and picks a folder to watch.
3. A dedicated LXD container is provisioned for the user, running headless
   Chrome behind a noVNC proxy. The user signs into Chrome inside that
   container once, via the browser, so bookmarks sync to their real account.
4. Google Drive push notifications hit `/v1/webhooks/drive` whenever the
   watched folder changes. New files are downloaded, run through OpenAI,
   and the extracted answers are written to a Chrome bookmark folder inside
   the user's container — which Chrome syncs back out to their devices.

## Stack

- Go 1.x with [Fiber v3](https://github.com/gofiber/fiber)
- SQLite + GORM for persistence (`getaced.db` next to the binary)
- LXD for per-user containers running Chrome + noVNC
- Google OAuth + Drive API (push notifications, not polling)
- OpenAI for the answer extraction step (model in `src/config/defaults.go`)

## Prerequisites

- Go installed
- LXD installed and initialised on the host
- A base LXD container called `getaced-base` that boots Chrome via
  `scripts/start.sh` (see that script for what's expected)
- A Google Cloud OAuth client with Drive scope enabled
- An OpenAI API key

## Setup

```bash
git clone <this repo>
cd backend
cp .env.example .env
# fill in .env — pay attention to ALLOWED_EMAILS, that's the only access gate
go run src/main.go
```

SQLite is created automatically on first boot.

`ALLOWED_EMAILS` is a comma-separated allowlist of Google emails that are
allowed to sign in. Anyone else gets a `403 access denied` at the OAuth
callback.

## Operations

### Local

```bash
# Run
go run src/main.go

# Build
go build ./src/...

# Free port 80 if needed
kill $(lsof -ti :80)
```

### LXD containers

```bash
# List
lxc list

# Shell into a user container
lxc exec getaced-1 -- bash

# Inspect a user's bookmarks
lxc file pull getaced-1/root/.config/google-chrome/Default/Bookmarks -

# Start / stop
lxc start getaced-1
lxc stop getaced-1

# Wipe a user container
lxc stop getaced-1 --force && lxc delete getaced-1

# View base container start script
lxc file pull getaced-base/root/start.sh -
```

### Database

SQLite at `./getaced.db`.

```bash
sqlite3 getaced.db

# Useful queries
SELECT id, email, onboarding_step, usage_count FROM users;
SELECT * FROM processed_files ORDER BY created_at DESC LIMIT 10;
```

### Deploy

```bash
# On the VPS
cd /root/getaced && git pull && go build -o getaced ./src/... && systemctl restart getaced

# Logs
sudo journalctl -u getaced -f
sudo journalctl -u getaced -n 100
sudo journalctl -u getaced -b

# Service status
sudo systemctl status getaced
```

## API

Import `docs/getaced.postman_collection.json` into Postman or Bruno for the
full endpoint surface and example payloads.
