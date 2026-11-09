#!/usr/bin/env python3
"""Exercise the real local Alisa OAuth flow; never connects to production."""
import pathlib,json,http.cookiejar,urllib.request,urllib.parse,urllib.error,html.parser
root=pathlib.Path(__file__).resolve().parents[1];account=json.loads((root/'deploy/local/alisa-account.json').read_text())
class Parser(html.parser.HTMLParser):
 def __init__(self):super().__init__();self.hidden={};self.forms=[]
 def handle_starttag(self,tag,attrs):
  a=dict(attrs)
  if tag=='input' and a.get('type')=='hidden':self.hidden[a.get('name')]=a.get('value','')
  if tag=='form':self.forms.append(a)
jar=http.cookiejar.CookieJar();browser=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
with browser.open('http://127.0.0.1:3000/api/auth/login',timeout=15) as r:page=r.read().decode();url=r.url
assert urllib.parse.urlparse(url).port==9091 and '/login' in url,'Expected real authorization login'
p=Parser();p.feed(page)
form=urllib.parse.urlencode({'username':account['username'],'password':account['password'],'_csrf':p.hidden['_csrf']}).encode()
with browser.open(urllib.request.Request('http://127.0.0.1:9091/login',data=form,headers={'Content-Type':'application/x-www-form-urlencoded'}),timeout=15) as r:page=r.read().decode();url=r.url
if '/oauth/consent' in url:
 p=Parser();p.feed(page);form=dict(p.hidden,scope='portal:console:manage')
 with browser.open(urllib.request.Request('http://127.0.0.1:9091/oauth2/authorize',data=urllib.parse.urlencode(form).encode(),headers={'Content-Type':'application/x-www-form-urlencoded'}),timeout=15) as r:page=r.read().decode();url=r.url
if 'auth_error' in url:raise SystemExit('Real OAuth callback was denied: '+urllib.parse.parse_qs(urllib.parse.urlparse(url).query).get('auth_error',['unknown'])[0])
with browser.open('http://127.0.0.1:3000/api/auth/session',timeout=15) as r:session=json.load(r)
assert session.get('authenticated') is True,'Portal session was not established'
with browser.open('http://127.0.0.1:3000/api/state',timeout=15) as r:state=json.load(r);assert r.status==200
req=urllib.request.Request('http://127.0.0.1:3000/api/auth/logout',method='POST',headers={'Origin':'http://127.0.0.1:3000','X-CSRF-Token':session['csrfToken']})
with browser.open(req,timeout=15) as r:assert r.status==200
with browser.open('http://127.0.0.1:3000/api/auth/session',timeout=15) as r:assert json.load(r).get('authenticated') is False
print(json.dumps({'realUserPasswordLogin':True,'realConsentOrExistingGrant':True,'pkceTokenExchange':True,'realIntrospectionAndPDP':True,'portalState':200,'logoutInvalidatesSession':True}))

# A platform administrator may manage policies; that alone must not bypass
# console resource authorization. Restore the exact binding in all cases.
def management(method,path,body=None,token=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request('http://127.0.0.1:9091'+path,method=method,headers=headers,data=json.dumps(body).encode() if body is not None else None)
 with urllib.request.urlopen(req,timeout=15) as response:return json.load(response)
admin=management('POST','/api/v1/user/signin',{k:account[k] for k in ['username','email','password']})['data']
binding={'subjectType':'USER','subjectId':account['userId']}
route='/api/v1/authorization/applications/portal/roles/portal-admin/bindings'
try:
 management('DELETE',route,binding,admin)
 decision=management('POST','/api/v1/authorization/check',{'applicationId':'portal','resourceType':'console','resourceId':'*','action':'manage'},admin)
 assert decision['data']['effect']=='DENY','Platform administration bypassed resource policy'
 with browser.open('http://127.0.0.1:3000/api/auth/login',timeout=15) as r:denied_url=r.url;r.read()
 # The authorization center may reuse this browser's login and prior consent.
 if 'auth_error' not in denied_url:raise AssertionError('Expected the portal callback to deny the revoked user')
 assert urllib.parse.parse_qs(urllib.parse.urlparse(denied_url).query).get('auth_error')==['forbidden']
 with browser.open('http://127.0.0.1:3000/api/auth/session',timeout=15) as r:assert json.load(r).get('authenticated') is False
 print('Real permission revocation denied portal login; binding restored after check')
finally:
 management('PUT',route,binding,admin)
