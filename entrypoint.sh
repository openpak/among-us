#!/bin/bash
# Runs Impostor, the front and the DTLS front; if any exits, the container exits and the runtime restarts all three.
set -m
cd /app && ./Impostor.Server &
/usr/local/bin/among-us &
dotnet /opt/dtls-front/dtls-front.dll &
wait -n
exit 1
