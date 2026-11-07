#!/usr/bin/env python3
"""Generate credential-free Argo build inputs from tracked application source."""
import base64
import gzip
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile
import argparse

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--registry', required=True, help='Registry hostname without scheme')
parser.add_argument('--output', required=True, type=Path, help='Output directory outside Git')
args = parser.parse_args()
tracked = subprocess.check_output(['git', 'ls-files', '-z', 'backend', 'frontend'], cwd=root).decode().split('\0')
archive = io.BytesIO()
with tarfile.open(fileobj=archive, mode='w') as tar:
    for name in sorted(filter(None, tracked)):
        path = root / name
        if path.is_symlink() or not path.is_file():
            raise SystemExit(f'Unexpected build input: {name}')
        info = tarfile.TarInfo(name)
        content = path.read_bytes()
        info.size, info.mode, info.mtime = len(content), 0o644, 0
        tar.addfile(info, io.BytesIO(content))
bundle = gzip.compress(archive.getvalue(), mtime=0)
tag = 'src-' + hashlib.sha256(bundle).hexdigest()[:12]
cm = {'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'portal-build-context-' + tag, 'namespace': 'alisa-portal'}, 'binaryData': {'source.tar.gz': base64.b64encode(bundle).decode()}}
if len(json.dumps(cm)) > 900000:
    raise SystemExit('Build context exceeds ConfigMap size budget; use CI/object storage instead')
resources = [cm]
for component, directory in [('api', 'backend'), ('web', 'frontend')]:
    job = {'apiVersion': 'batch/v1', 'kind': 'Job', 'metadata': {'name': f'portal-build-{component}-{tag}', 'namespace': 'alisa-portal'}, 'spec': {'backoffLimit': 0, 'activeDeadlineSeconds': 2400, 'template': {'spec': {'automountServiceAccountToken': False, 'restartPolicy': 'Never', 'initContainers': [{'name': 'unpack', 'image': 'busybox:1.37.0', 'command': ['sh', '-c', 'tar -xzf /bundle/source.tar.gz -C /workspace'], 'volumeMounts': [{'name': 'bundle', 'mountPath': '/bundle', 'readOnly': True}, {'name': 'workspace', 'mountPath': '/workspace'}]}], 'containers': [{'name': 'build', 'image': 'ghcr.io/osscontainertools/kaniko:v1.29.0', 'args': [f'--context=dir:///workspace/{directory}', f'--dockerfile=/workspace/{directory}/Dockerfile', f'--destination={args.registry}/alisa-portal-{component}:{tag}', '--build-arg=API_URL=http://api:8080', '--cache=false', '--compressed-caching=false', '--reproducible'], 'resources': {'requests': {'cpu': '250m', 'memory': '512Mi'}, 'limits': {'cpu': '2', 'memory': '2Gi'}}, 'volumeMounts': [{'name': 'workspace', 'mountPath': '/workspace'}, {'name': 'registry-auth', 'mountPath': '/kaniko/.docker', 'readOnly': True}]}], 'volumes': [{'name': 'bundle', 'configMap': {'name': cm['metadata']['name']}}, {'name': 'workspace', 'emptyDir': {'sizeLimit': '4Gi'}}, {'name': 'registry-auth', 'secret': {'secretName': 'portal-registry-auth', 'items': [{'key': '.dockerconfigjson', 'path': 'config.json'}]}}]}}}}
    resources.append(job)
args.output.mkdir(parents=True, exist_ok=True)
(args.output / 'build-manifests.json').write_text(json.dumps(resources))
(args.output / 'release.json').write_text(json.dumps({'tag': tag, 'registry': args.registry}))
print(json.dumps({'tag': tag, 'context_bytes': len(bundle), 'output': str(args.output)}))
