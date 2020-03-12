#!/usr/bin/env bash
set -e

# Set www-data uid & gid
usermod -u $(stat -c %u /app) node || true
groupmod -g $(stat -c %g /app) node || true

# change to user node
gosu node "$@"
