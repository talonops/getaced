#!/bin/bash
set -e

REMOTE="talon@151.247.22.11"
BIN="getaced"
KEY="$(dirname "$0")/ssh-key"

GOOS=linux GOARCH=amd64 go build -o "$BIN" ./src
echo "built"

scp -i "$KEY" "$BIN" "$REMOTE":~/getaced.new
echo "uploaded"

ssh -i "$KEY" "$REMOTE" "killall $BIN 2>/dev/null || true; mv ~/getaced.new ~/$BIN"
echo "deployed — starting and streaming logs (ctrl+c to stop)..."

# Run the binary and tail its output; ctrl+c kills both
ssh -t -i "$KEY" "$REMOTE" "~/$BIN 2>&1 | tee ~/getaced.log"
