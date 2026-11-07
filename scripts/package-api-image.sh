#!/bin/sh
# Package a static Linux Go binary using an existing Alpine image in your Registry.
set -eu
if [ "$#" -ne 2 ]; then
  echo "Usage: DOCKER_CONFIG=... CRANE=crane sh scripts/package-api-image.sh REGISTRY TAG" >&2
  exit 2
fi
registry_host="$1"
release_tag="$2"
crane_bin="${CRANE:-crane}"
project_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
build_tmp="$(mktemp -d)"
trap 'rm -rf "$build_tmp"' EXIT HUP INT TERM
(cd "$project_root/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$build_tmp/portal" ./cmd/server)
python3 - "$build_tmp" <<'PY'
import io
import sys
import tarfile
from pathlib import Path
root = Path(sys.argv[1])
with tarfile.open(root / 'runtime.tar', 'w') as archive:
    directory = tarfile.TarInfo('data')
    directory.type, directory.mode, directory.uid, directory.gid = tarfile.DIRTYPE, 0o700, 10001, 10001
    archive.addfile(directory)
    content = (root / 'portal').read_bytes()
    binary = tarfile.TarInfo('usr/local/bin/portal')
    binary.size, binary.mode, binary.uid, binary.gid = len(content), 0o755, 10001, 10001
    archive.addfile(binary, io.BytesIO(content))
PY
"$crane_bin" mutate "$registry_host/alpine:3.22" --platform linux/amd64 \
  --append "$build_tmp/runtime.tar" --entrypoint /usr/local/bin/portal --cmd= \
  --user 10001:10001 --env LISTEN_ADDR=0.0.0.0:8080 --env DATA_FILE=/data/portal.json \
  --exposed-ports 8080/tcp --set-platform linux/amd64 \
  --tag "$registry_host/alisa-portal-api:$release_tag"
