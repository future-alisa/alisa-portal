#!/usr/bin/env python3
"""Run the real alisa-user with an isolated, loopback-only development database."""
import os,pathlib,json,subprocess,xml.etree.ElementTree as ET,secrets,argparse
os.umask(0o077)
# Do not let a caller's production datasource, profile, mail or -D settings
# override the generated isolated development configuration.
blocked=('SPRING_','ALISA_','MYSQL_','JWT_','OAUTH_','AUTHORIZATION_','TENANCY_','SERVER_','MAIL_','REDIS_','NACOS_')
child_env={k:v for k,v in os.environ.items() if not k.startswith(blocked) and k not in {'JAVA_TOOL_OPTIONS','JDK_JAVA_OPTIONS','_JAVA_OPTIONS'}}
root=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--cloud-root',type=pathlib.Path,default=root.parent/'alisa-cloud');parser.add_argument('--csrf-debug',action='store_true',help='Log CSRF failure type and session presence only');args=parser.parse_args();cloud=args.cloud_root.resolve()
private=root/'deploy/local/real-auth';private.mkdir(parents=True,exist_ok=True);private.chmod(0o700)
account=json.loads((root/'deploy/local/alisa-account.json').read_text())
if account.get('username')!='alisa' or not account.get('userId') or not account.get('password'):raise SystemExit('Expected the explicitly configured alisa account in deploy/local/alisa-account.json')
report=cloud/'alisa-user/target/surefire-reports/TEST-com.alisa.user.oauth.OAuthFlowTest.xml'
if not report.exists():raise SystemExit('Run mvn -pl alisa-user -am test in the chosen cloud checkout first')
classpath=next(x.attrib['value'] for x in ET.parse(report).findall('.//property') if x.attrib['name']=='java.class.path')
classpath=os.pathsep.join(x for x in classpath.split(os.pathsep) if not x.endswith('/test-classes'))
state=private/'runtime.json'
if state.exists():runtime=json.loads(state.read_text())
else:runtime={'jwtSecret':secrets.token_urlsafe(48),'dbPassword':secrets.token_urlsafe(24)};state.write_text(json.dumps(runtime));state.chmod(0o600)
def literal(s):return "'"+s.replace("'","''")+"'"
new=not (private/'authorization.mv.db').exists()
if new:
 source=private/'HashPassword.java'
 source.write_text('import java.io.*; import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder; class HashPassword { public static void main(String[] a)throws Exception { System.out.print(new BCryptPasswordEncoder().encode(new BufferedReader(new InputStreamReader(System.in)).readLine())); }}')
 result=subprocess.run(['java','--class-path',classpath,str(source)],input=account['password']+'\n',text=True,capture_output=True,check=True,env=child_env)
 hashed=result.stdout.strip();assert hashed.startswith('$2')
 schema='CREATE TABLE IF NOT EXISTS tb_user(user_id VARCHAR(64) PRIMARY KEY,user_name VARCHAR(45) NOT NULL UNIQUE,user_password VARCHAR(255) NOT NULL,user_email VARCHAR(255) NOT NULL UNIQUE);\n'
 schema+=(cloud/'alisa-user/src/main/resources/sql/oauth.sql').read_text()
 schema+=(cloud/'alisa-authorization/src/main/resources/db/authorization-schema.sql').read_text()
 schema+='\nINSERT INTO tb_user(user_id,user_name,user_password,user_email) VALUES('+','.join(literal(x) for x in [account['userId'],account['username'],hashed,account['email']])+');\n'
 (private/'schema.sql').write_text(schema)
cfg={'server':{'port':9091,'address':'127.0.0.1','servlet':{'session':{'cookie':{'name':'ALISA_LOCAL_AUTH_SESSION','same-site':'lax','http-only':True}}}},'spring':{'cloud':{'nacos':{'discovery':{'enabled':False},'config':{'enabled':False,'import-check':{'enabled':False}}},'discovery':{'enabled':False}},'datasource':{'driver-class-name':'org.h2.Driver','url':'jdbc:h2:file:'+str(private/'authorization')+';MODE=MySQL;DB_CLOSE_ON_EXIT=FALSE','username':'sa','password':runtime['dbPassword']},'sql':{'init':{'mode':'always' if new else 'never','schema-locations':'file:'+str(private/'schema.sql')}}},'jwt':{'secret':runtime['jwtSecret']},'alisa':{'oauth':{'enabled':True,'issuer':'http://127.0.0.1:9091','allow-loopback-redirects':True,'applications':{'portal':{'[portal:console:manage]':'管理门户实例'}}},'authorization':{'enabled':True,'applications':{'portal':'local'},'administrators':[account['userId']]},'tenancy':{'enabled':False}},'logging':{'level':{'root':'INFO','org.springframework.security':'INFO','org.springframework.jdbc':'INFO'}}}
# Tomcat keeps the saved OAuth request and its CSRF token together across
# orderly local restarts. Store sessions beside the private H2 database.
sessions=private/'sessions'
sessions.mkdir(mode=0o700,exist_ok=True)
sessions.chmod(0o700)
cfg['server']['servlet']['session'].update(persistent=True, **{'store-dir':str(sessions)})
if args.csrf_debug:cfg['logging']['level']['com.alisa.user.oauth.OAuthConfiguration']='DEBUG'
config=private/'application.yaml';config.write_text(json.dumps(cfg,ensure_ascii=False));config.chmod(0o600)
print('Starting real alisa-user on http://127.0.0.1:9091; independent persistent local H2 database',flush=True)
args=['java','-cp',classpath,'com.alisa.user.AlisaUserApplication','--spring.profiles.active=portal-local','--spring.config.additional-location=file:'+str(config)]
raise SystemExit(subprocess.call(args,cwd=cloud,env=child_env))
