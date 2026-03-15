#!/bin/bash
set -e

REMOTE="talon@151.247.22.11"
BIN="getaced"
KEY="$(dirname "$0")/ssh-key"

CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc go build -ldflags '-s -w -linkmode external -extldflags "-static"' -o "$BIN" ./src
echo "built"

scp -i "$KEY" "$BIN" "$REMOTE":~/getaced.new
echo "uploaded"

ssh -i "$KEY" "$REMOTE" "killall $BIN 2>/dev/null || true; sleep 1; mv -f ~/getaced.new ~/$BIN; chmod +x ~/$BIN"
echo "deployed — starting and streaming logs (ctrl+c to stop)..."

# Run the binary and tail its output; ctrl+c kills both
ssh -t -i "$KEY" "$REMOTE" "~/$BIN 2>&1 | tee ~/getaced.log"
