#!/bin/bash
# Runs Impostor and the front; if either exits, the container exits and the runtime restarts both.
set -m
cd /app && ./Impostor.Server &
/usr/local/bin/among-us &
wait -n
exit 1
