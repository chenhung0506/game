#!/bin/bash
# Deploy the game to linode-server: push what is on origin/main, rebuild and restart the container.
#
#   ./deploy.sh          test, then deploy
#   ./deploy.sh -s       skip the tests
#   ./deploy.sh -n       dry run: run the local checks and print the remote steps without running them
#
# It deploys what is on origin/main, so commit and push first. Works for code and for config.json alike:
# the container is recreated every time, because a restart would keep reading the old config.json (the
# file is bind-mounted, and git replaces it rather than editing it in place).
set -euo pipefail

HOST=linode-server
URL=https://game.linch.uk
COMPOSE=/usr/local/bin/docker-compose

skip_tests=false
dry=false
while getopts 'sn' opt; do
  case $opt in
    s) skip_tests=true ;;
    n) dry=true ;;
    *) echo "usage: $0 [-s] [-n]" >&2; exit 2 ;;
  esac
done

cd "$(dirname "${BASH_SOURCE[0]}")"

# ---- local checks: deploy only what is committed and pushed ----
branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$branch" != main ]; then
  echo "on branch '$branch', not main: the server deploys origin/main" >&2
  exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "uncommitted changes (they would not be deployed):" >&2
  git status --short >&2
  exit 1
fi
git fetch -q origin main
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
  echo "local main and origin/main differ: git push (or git pull) first" >&2
  exit 1
fi
commit=$(git rev-parse --short HEAD)

if $skip_tests; then
  echo "skipping tests"
else
  echo "== go vet / go test -race"
  go vet ./...
  go test -race ./...
fi

# ---- remote: pull, rebuild, recreate ----
remote="set -e
cd ~/game
git pull --ff-only
cd ~/reverse-proxy/docker
$COMPOSE up -d --build --force-recreate game
sleep 4
docker ps --filter name=^game\$ --format '{{.Names}} {{.Status}}'
docker logs --tail 2 game 2>&1"

echo "== deploying $commit to $HOST"
if $dry; then
  echo "[dry run] ssh $HOST"
  echo "$remote" | sed 's/^/    /'
  echo "[dry run] curl $URL/health"
  exit 0
fi
# BatchMode: never stop to ask for a password; the key must already work.
ssh -o BatchMode=yes "$HOST" "$remote" 2> >(grep -v 'bashrc' >&2)

# ---- check it from the outside ----
echo "== $URL/health"
for _ in 1 2 3 4 5; do
  if curl -fsS --max-time 5 "$URL/health"; then
    echo
    echo "deployed $commit"
    exit 0
  fi
  sleep 2
done
echo "deployed $commit, but $URL/health did not answer: check 'ssh $HOST docker logs game'" >&2
exit 1
