#!/usr/bin/env python3
"""Run the portal API against the real loopback Alisa Cloud server."""
import os,json,pathlib
root=pathlib.Path(__file__).resolve().parents[1]
c=json.loads((root/'deploy/local/real-auth/runtime.json').read_text())
for key in ['clientId','clientSecret','resourceClientId','resourceClientSecret']:
    if not c.get(key):raise SystemExit('Run configure-alisa-cloud-local.py first')
env={k:v for k,v in os.environ.items() if not k.startswith('ALISA_OAUTH_') and k!='ALISA_PORTAL_ORIGIN'}
env.update(ALISA_OAUTH_ISSUER='http://127.0.0.1:9091',ALISA_PORTAL_ORIGIN='http://127.0.0.1:3000',ALISA_OAUTH_ALLOW_LOOPBACK='true',ALISA_OAUTH_CLIENT_ID=c['clientId'],ALISA_OAUTH_CLIENT_SECRET=c['clientSecret'],ALISA_OAUTH_RESOURCE_CLIENT_ID=c['resourceClientId'],ALISA_OAUTH_RESOURCE_CLIENT_SECRET=c['resourceClientSecret'],ALISA_OAUTH_ALLOW_API_TOKEN='false',DATA_FILE=str(root/'deploy/local/portal-state.json'),LISTEN_ADDR='127.0.0.1:8080')
env.setdefault('GOCACHE',str(root/'deploy/local/go-cache'))
os.chdir(root/'backend')
os.execvpe('go',['go','run','./cmd/server'],env)
