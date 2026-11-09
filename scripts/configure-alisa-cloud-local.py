#!/usr/bin/env python3
"""Provision only localhost:9091 via real Alisa OAuth and authorization APIs."""
import urllib.request,urllib.parse,http.cookiejar,html.parser,json,pathlib
root=pathlib.Path(__file__).resolve().parents[1];private=root/'deploy/local/real-auth';state=private/'runtime.json';runtime=json.loads(state.read_text());account=json.loads((root/'deploy/local/alisa-account.json').read_text());base='http://127.0.0.1:9091'
class Parser(html.parser.HTMLParser):
 def __init__(self):super().__init__();self.hidden={};self.codes=[];self.incode=False
 def handle_starttag(self,tag,attrs):
  a=dict(attrs)
  if tag=='input' and a.get('type')=='hidden':self.hidden[a.get('name')]=a.get('value','')
  if tag=='code':self.incode=True;self.codes.append('')
 def handle_endtag(self,tag):
  if tag=='code':self.incode=False
 def handle_data(self,data):
  if self.incode:self.codes[-1]+=data

def api(path,body=None,method=None,token=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request(base+path,method=method or ('POST' if body is not None else 'GET'),headers=headers,data=json.dumps(body).encode() if body is not None else None)
 with urllib.request.urlopen(req,timeout=15) as r:return json.load(r)
def save():state.write_text(json.dumps(runtime,ensure_ascii=False,indent=2));state.chmod(0o600)
login=api('/api/v1/user/signin',{k:account[k] for k in ['username','email','password']});token=login.get('data');assert isinstance(token,str) and token,'Local real account sign-in failed'
profile=api('/api/v1/user/profile',token=token);assert profile['data']['username']=='alisa'
role='/api/v1/authorization/applications/portal/roles/portal-admin'
for path,body in [(role,{'description':'本地门户实例管理员'}),(role+'/grants',{'resourceType':'console','resourceId':'*','action':'manage'}),(role+'/bindings',{'subjectType':'USER','subjectId':account['userId']})]:
 result=api(path,body,'PUT',token);assert result.get('code')==200
if not runtime.get('resourceClientId'):
 resource=api('/api/v1/user/oauth/resource-clients',{'applicationId':'portal','name':'Alisa Portal local API'},token=token)
 runtime['resourceClientId']=resource['clientId'];runtime['resourceClientSecret']=resource['clientSecret'];save()
if not runtime.get('clientId'):
 jar=http.cookiejar.CookieJar();browser=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
 with browser.open(base+'/login',timeout=15) as r:page=r.read().decode()
 p=Parser();p.feed(page)
 form=urllib.parse.urlencode({'username':account['username'],'password':account['password'],'_csrf':p.hidden['_csrf']}).encode()
 with browser.open(urllib.request.Request(base+'/login',data=form,headers={'Content-Type':'application/x-www-form-urlencoded'}),timeout=15) as r:page=r.read().decode();assert '/oauth/apps' in r.url,'Local browser login failed'
 p=Parser();p.feed(page)
 form=urllib.parse.urlencode({'applicationId':'portal','name':'Alisa Portal local','redirectUri':'http://127.0.0.1:3000/api/auth/callback','scope':'portal:console:manage','publicClient':'false','_csrf':p.hidden['_csrf']}).encode()
 with browser.open(urllib.request.Request(base+'/oauth/apps',data=form,headers={'Content-Type':'application/x-www-form-urlencoded'}),timeout=15) as r:page=r.read().decode()
 p=Parser();p.feed(page)
 if len(p.codes)!=2:
  f=private/'registration-response.html';f.write_text(page);f.chmod(0o600);raise SystemExit('Client registration response requires inspection in private file')
 runtime['clientId'],runtime['clientSecret']=p.codes;save()
with urllib.request.urlopen(base+'/.well-known/oauth-authorization-server',timeout=15) as r:metadata=json.load(r)
assert metadata['issuer']==base and 'S256' in metadata['code_challenge_methods_supported']
print(json.dumps({'realAuthorizationServer':base,'account':'alisa','localConsoleRole':'granted','confidentialClient':'registered','introspectionClient':'registered','pkceS256':True}))
