# api.getaced.io

Scans a Google Drive folder for PNG/HTML files, feeds them into GPT, extracts answers, and syncs them as Chrome bookmarks in an LXD container.

## Quick Commands

```bash
# Run locally
go run src/main.go

# Build
go build -o getaced ./src/...

# Kill port
kill $(lsof -ti :80)
```

## LXD Container Commands

```bash
# List containers
lxc list

# Shell into a container
lxc exec getaced-1 -- bash

# Check container bookmarks
lxc file pull getaced-1/root/.config/google-chrome/Default/Bookmarks -

# Start/stop container
lxc start getaced-1
lxc stop getaced-1

# Delete a user container
lxc stop getaced-1 --force && lxc delete getaced-1

# View base container files
lxc file pull getaced-base/root/start.sh -
```

## Database

SQLite at `./getaced.db`

```bash
# Open db
sqlite3 getaced.db

# Useful queries
SELECT id, email, onboarding_step, subscription_status, usage_count FROM users;
SELECT * FROM processed_files ORDER BY created_at DESC LIMIT 10;
```

## Deploy

```bash
# On VPS
cd /root/getaced && git pull && go build -o getaced ./src/... && systemctl restart getaced
```

## Env

Copy `.env.example` to `.env`. See `.env.example` for all required vars.

## API

Import `getaced.postman_collection.json` into Postman/Bruno for all endpoints.
