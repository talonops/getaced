#!/bin/bash
source /etc/getaced.conf

MODE=${1:-setup}

ip addr add 10.246.29.${IP_SUFFIX}/24 dev eth0 2>/dev/null
ip route add default via 10.246.29.1 2>/dev/null

if [ "$MODE" = "setup" ]; then
    Xvfb :1 -screen 0 1280x720x24 &
    sleep 2
    export DISPLAY=:1
    x11vnc -display :1 -forever -nopw &
    websockify --web /usr/share/novnc 6080 localhost:5900 &
    google-chrome --no-sandbox --disable-gpu --disable-dev-shm-usage --no-first-run "https://accounts.google.com" &
    wait
elif [ "$MODE" = "sync" ]; then
    Xvfb :1 -screen 0 1280x720x24 &
    sleep 2
    export DISPLAY=:1
    google-chrome --no-sandbox --disable-gpu --disable-dev-shm-usage --no-first-run &
    sleep 10
    pkill chrome
    pkill Xvfb
fi
