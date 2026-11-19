#!/usr/bin/env python3
"""Exercise the production recovery commands on two disposable storage stacks."""
import argparse
import datetime
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import shutil
import socket
import subprocess
import sys
import tarfile
import time
import urllib.parse
import urllib.request
import urllib.error
import uuid
import zipfile

ROOT=pathlib.Path(__file__).resolve().parents[1]
PROD=ROOT/"deploy/production"
sys.path.insert(0,str(PROD))
from checkpoint_roles import Checkpoint, write_json
from s3_archive import S3, file_hash


def command(args, *, env=None, stdin=None, binary=False):
    result=subprocess.run([str(a) for a in args],env=env,input=stdin,capture_output=True,timeout=500)
    if result.returncode:
        # Detailed commands/configurations can contain access paths. Retain error
        # codes privately and expose only the executable and operation here.
        with (WORK/"command-errors.log").open("ab") as output:
            output.write(result.stderr)
        raise ValueError(pathlib.Path(args[0]).name+" "+str(args[1])+" failed")
    return result.stdout if binary else result.stdout.decode().strip()


def docker(*args,**kw):
    return command(["docker",*args],**kw)


def pg(query,db):
    return docker("exec","-i",PG,"psql","-XqAt","-v","ON_ERROR_STOP=1","-U",USER,"-d",db,stdin=query.encode())


def uri(db):
    return urllib.parse.urlunsplit(DB._replace(path="/"+db))


def dump(db):
    return docker("exec",PG,"pg_dump","-U",USER,"-Fc","--no-owner","--no-acl",db,binary=True)


def private_network(owner):
    module=(ROOT/"scripts/architecture-http-network.mjs").as_uri()
    script=(
        "const [module,owner]=process.argv.slice(1);"
        "const {ensureOwnedDockerNetwork}=await import(module);"
        "process.stdout.write(ensureOwnedDockerNetwork({name:owner+'-data',owner,"
        "label:'infrapro.architecture.joint-recovery',prefix:'10.244'}));"
    )
    name=command(["node","--input-type=module","--eval",script,module,owner])
    network=json.loads(docker("network","inspect",name))[0]
    assert network["Labels"]["infrapro.architecture.joint-recovery"]==owner
    owned_networks.append({"owner":owner,"name":name,"id":network["Id"],
                           "subnet":network["IPAM"]["Config"][0]["Subnet"]})
    return name


def profile(side):
    name=TAG+"-"+side
    path=WORK/side
    path.mkdir()
    (path/"conf").mkdir()
    template=ROOT/"deploy/monitoring-restore"
    for file in ("clickhouse.xml","keeper.xml","s3.json"):
        shutil.copyfile(template/file,path/"conf"/file)
    s3cfg=json.loads((path/"conf/s3.json").read_text())
    s3cfg["identities"].append({"name":"joint-audit-content","credentials":[{"accessKey":"joint-audit-content","secretKey":"test-fixture-only"}],"actions":["Read:joint-audit"]})
    write_json(path/"conf/s3.json",s3cfg)
    write_json(path/"audit-content.json",{"access_key":"joint-audit-content","secret_key":"test-fixture-only"})
    shutil.copyfile(WORK/"tls.pem",path/"conf/tls.pem")
    shutil.copyfile(WORK/"tls.key",path/"conf/tls.key")
    (path/"conf/tls.conf").write_text("server { listen 443 ssl; ssl_certificate /cfg/tls.pem; ssl_certificate_key /cfg/tls.key; client_max_body_size 128m; access_log off; location / { proxy_http_version 1.1; proxy_set_header Host $http_host; proxy_request_buffering off; proxy_buffering off; proxy_pass http://s3:8333; } }")
    stack=json.loads(docker("compose","-f",template/"compose.yaml","config","--format","json"))
    keep={k:v for k,v in stack["services"].items() if k in ("keeper","s3","ch1")}
    for service in keep.values():
        for vol in service.get("volumes",[]):
            if vol["type"]=="bind":
                vol["source"]=str(path/"conf"/pathlib.Path(vol["source"]).name)
        if "ports" in service:
            service["ports"]=[{"target":9000,"host_ip":"127.0.0.1"}] if service is keep["ch1"] else []
        service["networks"]={"default":None}
    if side=="source":
        # Only the isolated maintenance identity installs the application users.
        # Each service's actual readiness and data-plane checks use its own login.
        permissions=path/"conf"/"access.xml"
        permissions.write_text("<clickhouse><users><default><access_management>1</access_management></default></users></clickhouse>")
        permissions.chmod(0o644)
        keep["ch1"]["volumes"].append({"type":"bind","source":str(permissions),"target":"/etc/clickhouse-server/users.d/arc-access.xml","read_only":True})
    keep["s3"]["command"]=[arg.replace("-volume.max=40","-volume.max=200") for arg in keep["s3"]["command"]]
    keep["tls"]={"image":"nginx:1.28-alpine","ports":[{"target":443,"host_ip":"127.0.0.1"}],"depends_on":["s3"],"volumes":[str(path/"conf")+":/cfg:ro",str(path/"conf/tls.conf")+":/etc/nginx/conf.d/default.conf:ro"]}
    conf=path/"compose.json"
    network=private_network(name)
    write_json(conf,{"name":name,"services":keep,
                     "networks":{"default":{"name":network,"external":True}}})
    stacks.append(conf)
    def compose(*args):
        return docker("compose","-p",name,"-f",conf,*args)
    compose("up","-d","--wait","--wait-timeout","160")
    sport=compose("port","tls","443").split(":")[-1]
    cport=compose("port","ch1","9000").split(":")[-1]
    endpoint="https://127.0.0.1:"+sport
    store=S3(endpoint,"restore-admin","restore-admin-test-only",WORK/"tls.pem")
    for n in range(100):
        try:
            store.request("GET");break
        except (OSError,RuntimeError):
            if n==99:raise
            time.sleep(.3)
    for bucket in ("restore-ch","restore-backups","restore-source","joint-files","joint-audit","joint-parsing"):
        store.request("PUT",bucket)
    for child in ("monitor","files","audit"): (path/child).mkdir(mode=0o700)
    common={"object_endpoint":endpoint,"access_key":"restore-admin","secret_key":"restore-admin-test-only","node_id":"joint-"+side}
    cfg={"database_url":uri(SOURCE if side=="source" else NEW),"redis_url":ctx["redis"],"redis_key_prefix":ctx["redis_prefix"],
         "support_contact":"隔离恢复演练","mfa_keyring_file":ctx["mfa"],"csrf_auth_key":ctx["csrf"],"trusted_origins":ctx["origin"],"public_console_url":ctx["origin"]+"/",
         "monitoring_clickhouse_replicas":"127.0.0.1:"+cport,"monitoring_clickhouse_database":"arc_joint","monitoring_clickhouse_user":"default","monitoring_clickhouse_policy":"monitoring_tiered",
         "monitoring_object_endpoint":endpoint,"monitoring_source_bucket":"restore-source","monitoring_object_access_key":"restore-source","monitoring_object_secret_key":"restore-source-test-only",
         "monitoring_backup_endpoint":endpoint,"monitoring_backup_bucket":"restore-backups","monitoring_backup_access_key":"restore-backup","monitoring_backup_secret_key":"restore-backup-test-only","monitoring_backup_named_collection":"monitoring_backups",
         "monitoring_temp_directory":str(path/"monitor"),"files":{**common,"bucket":"joint-files","temp_directory":str(path/"files")},"audit":{**common,"bucket":"joint-audit","temp_directory":str(path/"audit")},
         "parsing":{"enabled":"false","object_endpoint":endpoint,"bucket":"joint-parsing","object_access_key":"restore-admin","object_secret_key":"restore-admin-test-only"}}
    write_json(path/"maintenance.json",cfg)
    relay={"relay_id":RELAY,"database_url":uri(RSRC if side=="source" else RDST),"control_address":"127.0.0.1:18454","certificate_file":str(WORK/"tls.pem"),"key_file":str(WORK/"tls.key"),"client_ca_file":str(WORK/"tls.pem"),"principals_file":str(path/"principals.json"),"budget_bytes":"67108864","listener_port_min":1024,"listener_port_max":65535,"listen_addresses":["127.0.0.1"],"max_connections":16}
    write_json(path/"relay.json",relay)
    write_json(path/"principals.json",{})
    write_json(path/"events.json",{"mode":"pg","environment":"acceptance"})
    environ={k:v for k,v in os.environ.items() if not k.startswith(("PG","INFRAPRO_"))}
    env={**environ,"INFRAPRO_CONFIG":str(path/"maintenance.json"),"INFRAPRO_EVENTS_FILE":str(path/"events.json"),"SSL_CERT_FILE":str(WORK/"tls.pem")}
    return {"path":path,"store":store,"env":env,"compose":compose,"ch":compose("ps","-q","ch1")}


def app(stack,*args):
    value=command([BIN/"infrapro",*args],env=stack["env"])
    return json.loads(value) if value else {}


def accepted_part(stack, space, upload, data):
    object_id = str(uuid.uuid4())
    key = "files/v1/" + space + "/" + object_id
    sha = file_hash(data)
    size = data.stat().st_size
    stack["store"].request("PUT", "joint-files/" + key, file=data)
    pg(f"""BEGIN;
        INSERT INTO file_objects(id,space_id,kind,object_key,content_type,expected_size,actual_size,
            expected_sha256,actual_sha256,state,writer_kind,writer_id,writer_generation,writer_fence,
            writer_epoch,io_deadline,lease_until)
        SELECT '{object_id}','{space}','part','{key}','application/octet-stream',{size},{size},
            '{sha}','{sha}','available','upload','{upload}',1,1,restore_epoch,now(),now()
        FROM file_protocol WHERE id=1;
        INSERT INTO file_upload_parts(space_id,upload_id,part_no,expected_size,sha256,object_id,
            state,fence,lease_owner,lease_until,received_at)
        VALUES('{space}','{upload}',1,{size},'{sha}','{object_id}','received',1,gen_random_uuid(),now(),now());
        COMMIT""", SOURCE)
    return {"space": space, "object": object_id, "part": 1, "key": key, "sha256": sha, "size": size}


def sealed_audit_export(stack, actor):
    report = WORK/"audit-export.csv"
    report.write_bytes(b"event,outcome\njoint.restore,success\n")
    sha = file_hash(report)
    export_id, object_id = str(uuid.uuid4()), str(uuid.uuid4())
    key = "audit-exports/v1/" + export_id + "/1/" + object_id + ".csv"
    operation = pg("SELECT id FROM audit_operations ORDER BY created_at,id LIMIT 1", SOURCE)
    assert operation
    stack["store"].request("PUT", "joint-audit/" + key, file=report)
    pg(f"""BEGIN;
        INSERT INTO audit_exports(id,scope_kind,scope_key,owner_id,membership_key,filter,
            field_profile,reason,operation_id)
        VALUES('{export_id}','platform','platform','{actor}','platform','{{}}',
            'standard','joint restore fixture','{operation}');
        INSERT INTO audit_export_generations(export_id,generation,requested_by,operation_id,
            authorization_vector,projection_version,restore_epoch,state,phase,fence,attempt,object_id,finished_at)
        SELECT '{export_id}',1,'{actor}','{operation}','joint',1,restore_epoch,
            'succeeded','succeeded',1,1,'{object_id}',now() FROM audit_protocol WHERE id=1;
        INSERT INTO audit_export_objects(id,export_id,generation,object_key,fence,restore_epoch,
            state,size_bytes,sha256,expires_at) SELECT '{object_id}','{export_id}',1,'{key}',1,restore_epoch,
            'available',{report.stat().st_size},'{sha}',now()+interval '1 day' FROM audit_protocol WHERE id=1;
        COMMIT""", SOURCE)
    return {"id": export_id, "object": object_id, "key": key, "sha256": sha, "size": report.stat().st_size}


def presentation_facts(stack, tenant, project, actor):
    ids=[str(uuid.uuid4()) for _ in range(12)]
    screen,upload,job,package,config,release,terminal,subject,session,layer,blob,version=ids
    html=b"<!doctype html><title>Joint restore fixture</title>"
    scene=b'{"type":"FeatureCollection","features":[]}'
    sealed=WORK/"screen.html"
    sealed.write_bytes(html)
    geographic=WORK/"layer.geojson"
    geographic.write_bytes(scene)
    sealed_hash,geo_hash=file_hash(sealed),file_hash(geographic)
    manifest={"manifest_version":1,"app_key":"joint-restore","version":"1.0.0","entry":"index.html",
              "routing":{"mode":"hash"},"sdk_protocol":{"major":1},"required_capabilities":{},
              "config_schema":{"type":"object","properties":{}},"binding_schema":{},
              "files":[{"path":"index.html","size":len(html),"media_type":"text/html","sha256":sealed_hash}]}
    bundle=WORK/"screen.zip"
    with zipfile.ZipFile(bundle,"w") as package_zip:
        package_zip.writestr("manifest.json",json.dumps(manifest))
        package_zip.writestr("index.html",html)
    bundle_hash=file_hash(bundle)
    tokens={name:os.urandom(32).hex() for name in ("terminal","pair","session","asset")}
    write_json(WORK/"old-display-access.json",tokens)
    token_hash,pair_hash,session_hash,asset_hash=[hashlib.sha256(tokens[name].encode()).hexdigest() for name in ("terminal","pair","session","asset")]
    file="display/sealed/"+project+"/"+package+"/"+sealed_hash
    part="display/uploads/"+project+"/"+upload+"/0-"+bundle_hash
    shape="spatial/"+tenant+"/sources/"+blob
    stack["store"].request("PUT","restore-source/"+file,file=sealed)
    stack["store"].request("PUT","restore-source/"+part,file=bundle)
    stack["store"].request("PUT","restore-source/"+shape,file=geographic)
    pg(f"""BEGIN;
        INSERT INTO display_settings(tenant_id,project_id,enabled) VALUES('{tenant}','{project}',true);
        INSERT INTO display_instances(id,tenant_id,project_id,name) VALUES('{screen}','{tenant}','{project}','joint fixture');
        INSERT INTO display_uploads(id,tenant_id,project_id,instance_id,actor_id,size_bytes,sha256,state)
            VALUES('{upload}','{tenant}','{project}','{screen}','{actor}',{bundle.stat().st_size},'{bundle_hash}','uploaded');
        INSERT INTO display_upload_parts(upload_id,part,size_bytes,sha256,object_key)
            VALUES('{upload}',0,{bundle.stat().st_size},'{bundle_hash}','{part}');
        INSERT INTO display_package_jobs(id,upload_id,state,attempt,fence) VALUES('{job}','{upload}','ready',1,1);
        INSERT INTO display_packages(id,tenant_id,project_id,instance_id,job_id,sha256,manifest)
            VALUES('{package}','{tenant}','{project}','{screen}','{job}','{bundle_hash}','{json.dumps(manifest)}');
        INSERT INTO display_package_files(package_id,path,sha256,size_bytes,media_type,object_key)
            VALUES('{package}','index.html','{sealed_hash}',{len(html)},'text/html','{file}');
        INSERT INTO display_configs(id,tenant_id,project_id,instance_id,package_id,parameters,bindings,mode,created_by)
            VALUES('{config}','{tenant}','{project}','{screen}','{package}','{{}}','{{}}','fixed','{actor}');
        INSERT INTO display_releases(id,tenant_id,project_id,instance_id,package_id,config_id,requirements,created_by)
            VALUES('{release}','{tenant}','{project}','{screen}','{package}','{config}','{{}}','{actor}');
        UPDATE display_instances SET current_release_id='{release}',draft_config_id='{config}' WHERE id='{screen}';
        INSERT INTO display_grants(tenant_id,project_id,instance_id,valid_until,issued_by)
            VALUES('{tenant}','{project}','{screen}',now()+interval '1 day','{actor}');
        INSERT INTO display_terminals(id,tenant_id,project_id,instance_id,name,state,token_hash,pair_hash,
            pair_expires_at,valid_until,environment,actual_release_id)
            SELECT '{terminal}','{tenant}','{project}','{screen}','joint terminal','active',
            '{token_hash}','{pair_hash}',now()+interval '2 minutes',
            now()+interval '1 day',generation,'{release}' FROM display_environment WHERE id=1;
        INSERT INTO display_subjects(id,tenant_id,project_id,instance_id,terminal_id,environment)
            SELECT '{subject}','{tenant}','{project}','{screen}','{terminal}',generation FROM display_environment WHERE id=1;
        INSERT INTO display_sessions(id,tenant_id,project_id,instance_id,subject_id,release_id,package_id,config_id,
            token_hash,asset_hash,terminal_generation,state,authorization_revision,lease_until)
            VALUES('{session}','{tenant}','{project}','{screen}','{subject}','{release}','{package}','{config}',
            '{session_hash}','{asset_hash}',1,'ready','1',now()+interval '30 minutes');
        INSERT INTO spatial_assets(id,tenant_id,kind,code,name,created_by)
            VALUES('{layer}','{tenant}','geolayer','JOINT','joint geolayer','{actor}');
        INSERT INTO spatial_blobs(id,tenant_id,object_key,sha256,size,media_type,name)
            VALUES('{blob}','{tenant}','{shape}','{geo_hash}',{len(scene)},'application/geo+json','layer.geojson');
        INSERT INTO spatial_asset_versions(id,tenant_id,asset_id,number,source_blob_id,format,created_by)
            VALUES('{version}','{tenant}','{layer}',1,'{blob}','geojson','{actor}');
        COMMIT""",SOURCE)
    return {"screen":screen,"package":package,"release":release,"session":session,"terminal":terminal,
            "file":file,"sha256":sealed_hash,"archive_sha256":bundle_hash,"layer":layer,"version":version,"geometry_key":shape,"geometry_sha256":geo_hash}

def display_access(stack, presentation, restored):
    config=json.loads((stack["path"]/"maintenance.json").read_text())
    with socket.socket() as runtime, socket.socket() as assets:
        runtime.bind(("127.0.0.1",0))
        assets.bind(("127.0.0.1",0))
        addrs=["127.0.0.1:"+str(s.getsockname()[1]) for s in (runtime, assets)]
    origin="http://"+addrs[0]
    config["display"]={"runtime_addr":addrs[0],"runtime_origin":origin,
                       "asset_addr":addrs[1],"asset_origin":"http://"+addrs[1]}
    phase="target" if restored else "source"
    path=WORK/(phase+"-display.json")
    write_json(path,config)
    tokens=json.loads((WORK/"old-display-access.json").read_text())
    client=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    headers={"Origin":origin,"X-Display-Protocol":"1","Content-Type":"application/json"}
    cases={
        "terminal":(urllib.request.Request(origin+"/runtime/terminal/start",data=b"{}",
                    headers={**headers,"Cookie":"infrapro_terminal="+tokens["terminal"]}),404,"not_found"),
        "session":(urllib.request.Request(origin+"/runtime/"+presentation["session"]+"/control",data=b'{"action":"ready"}',
                   headers={**headers,"Cookie":"infrapro_display="+tokens["session"]}),403,"revoked"),
        "asset":(urllib.request.Request("http://"+addrs[1]+"/assets/"+tokens["asset"]+"/index.html"),403,"revoked"),
    }
    # Starting a terminal rotates its token. Retain that exact issued token
    # through the freeze and prove the authorized package/session before it.
    if not restored:
        del cases["terminal"]
    observations={}
    with (WORK/(phase+"-display.log")).open("wb") as log:
        proc=subprocess.Popen([BIN/"display-runtime"],env={**stack["env"],"INFRAPRO_CONFIG":str(path)},
                              cwd=ROOT,stdout=log,stderr=subprocess.STDOUT)
        try:
            deadline=time.monotonic()+30
            while time.monotonic()<deadline:
                if proc.poll() is not None:
                    raise ValueError(phase+" display runtime exited before credential reads")
                try:
                    with client.open(origin+"/readyz",timeout=1) as reply:
                        if reply.status==200:break
                except OSError:
                    time.sleep(.2)
            else:
                raise ValueError(phase+" display runtime did not become ready")
            for name,(req,status,code) in cases.items():
                try:
                    with client.open(req,timeout=5) as reply:
                        if restored:
                            raise ValueError("Restored runtime accepted an old "+name+" credential: "+str(reply.status))
                        body=reply.read()
                        actual={"status":reply.status}
                        if name=="asset":
                            actual["sha256"]=hashlib.sha256(body).hexdigest()
                            assert actual["sha256"]==presentation["sha256"]
                        else:
                            assert json.loads(body)["received"]
                except urllib.error.HTTPError as reply:
                    actual={"status":reply.code,"code":json.load(reply)["code"]}
                if (restored and actual!={"status":status,"code":code}) or (not restored and actual["status"]!=200):
                    raise ValueError("Unexpected old "+name+" credential response: "+str(actual))
                observations[name]=actual
        finally:
            if proc.poll() is None:proc.terminate()
            try:proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
    return observations


def ch(stack, query):
    return docker("exec", stack["ch"], "clickhouse-client", "--database", "arc_joint", "--query", query)


def staged_observation(stack, tenant, batch, sha):
    channel, observation, adoption, chunk, attempt = [str(uuid.uuid4()) for _ in range(5)]
    ch(stack, f"""INSERT INTO observations (tenant_id,channel_id,observation_id,adoption_id,batch_id,
       chunk_id,attempt_id,row_ordinal,observed_at,received_at,value_kind,integer_value,quality,payload_hash)
       VALUES ('{tenant}','{channel}','{observation}','{adoption}','{batch}','{chunk}','{attempt}',1,
       '2026-11-16 02:00:00','2026-11-16 02:00:01','integer',37,'good','{sha}')""")
    return {"channel":channel, "id":observation, "adoption":adoption, "chunk":chunk, "attempt":attempt, "batch":batch, "value":37}


def physical_observation(stack, observation):
    return json.loads(ch(stack, f"""SELECT toString(observation_id) AS id,toString(batch_id) AS batch,
       toString(channel_id) AS channel,toString(chunk_id) AS chunk,toString(adoption_id) AS adoption,
       toString(attempt_id) AS attempt,integer_value AS value FROM observations
       WHERE observation_id='{observation["id"]}' SETTINGS output_format_json_quote_64bit_integers=0 FORMAT JSONEachRow"""))


def checkpoint_keys(stack, database, spool, facts):
    def record(statement, db=database):
        return json.loads(pg("SELECT to_jsonb(r) FROM (" + statement + ") r", db))
    receipt=facts["receipt"]
    result={
        "relay":record(f"""SELECT r.receipt_id,r.spool_epoch,r.source_sequence,r.envelope_digest,r.sha256,
            octet_length(r.payload) AS retained_bytes,d.state,d.attempt FROM relay_receipts r
            JOIN relay_deliveries d USING(receipt_id) WHERE r.receipt_id='{receipt}'""",spool),
        "local_ack":record(f"""SELECT r.id,r.relay_id,r.epoch,r.receipt_id,r.envelope_digest,o.sha256,
            (SELECT count(*) FROM ingestion_ack_outbox a WHERE a.receipt_id=r.id) AS ack_intents
            FROM ingestion_receipts r JOIN ingestion_objects o ON o.id=r.object_id WHERE r.id='{facts["local_receipt"]}'"""),
        "publication":record(f"""SELECT j.id,j.tenant_id,j.target_id,j.state,j.fence,
            (SELECT count(*) FROM monitoring_publications p WHERE p.batch_id=j.target_id) AS selected
            FROM monitoring_jobs j WHERE j.id='{facts["publish_job"]}'"""),
        "observation":physical_observation(stack,facts["staged_observation"]),
        "file":record(f"""SELECT u.id,u.state,u.part_count,p.part_no,p.state AS part_state,p.object_id,
            o.actual_sha256 AS sha256 FROM file_uploads u JOIN file_upload_parts p ON p.upload_id=u.id
            JOIN file_objects o ON o.id=p.object_id WHERE u.id='{facts["upload"]}'"""),
        "event":record(f"""SELECT event_id,type,aggregate_id,scope,state,fence,restore_epoch FROM event_outbox
            WHERE event_id='{facts["event"]}'"""),
        "audit":record(f"""SELECT e.id,g.state,o.id AS object_id,o.state AS object_state,o.sha256
            FROM audit_exports e JOIN audit_export_generations g ON g.export_id=e.id AND g.generation=e.current_generation
            JOIN audit_export_objects o ON o.id=g.object_id WHERE e.id='{facts["audit_export"]["id"]}'"""),
        "display":record(f"""SELECT s.id AS session,s.state AS session_state,t.id AS terminal,t.state AS terminal_state,
            t.generation,t.token_hash IS NOT NULL AS has_token,t.pair_hash IS NOT NULL AS has_pairing,
            t.environment,g.revoked AS grant_revoked,s.package_id,s.release_id,f.sha256 FROM display_sessions s
            JOIN display_subjects u ON u.id=s.subject_id JOIN display_terminals t ON t.id=u.terminal_id
            JOIN display_grants g ON g.instance_id=s.instance_id JOIN display_package_files f ON f.package_id=s.package_id
            WHERE s.id='{facts["presentation"]["session"]}'"""),
        "object_content":{}
    }
    for bucket, fact in (("joint-files",facts["received_part"]),("joint-audit",facts["audit_export"])):
        data=stack["store"].request("GET",bucket+"/"+fact["key"])
        result["object_content"][bucket]={"size":len(data),"sha256":hashlib.sha256(data).hexdigest()}
        assert result["object_content"][bucket]=={"size":fact["size"],"sha256":fact["sha256"]}
    return result


def source_facts(stack):
    app(stack,"monitoring","storage","init")
    command([BIN/"ingest-relay","migrate",stack["path"]/"relay.json"],env=stack["env"])
    epoch=pg("SELECT spool_epoch FROM relay_state WHERE id=1",RSRC)
    project,tenant=pg("SELECT id,tenant_id FROM projects WHERE code='arcjoint'",SOURCE).split("|")
    actor=pg("SELECT id FROM accounts ORDER BY created_at LIMIT 1",SOURCE)
    assert tenant and actor
    p=WORK/"sample.raw"
    p.write_bytes(b"joint restore custody object v1")
    digest=file_hash(p)
    receipt,local,src,object_id=[str(uuid.uuid4()) for _ in range(4)]
    key="ingestion/v1/"+local
    # The internal receipt and ACK intent exist. The external side is between
    # delivery and acknowledgment and must keep its payload on restore.
    envelope={"relay_id":RELAY,"spool_epoch":epoch,"receipt_id":receipt,"envelope_digest":digest}
    pg(f"""INSERT INTO relay_receipts(receipt_id,spool_epoch,ingress_id,source_sequence,envelope,envelope_digest,sha256,byte_length,payload,principal_id,received_at,committed_at)
        VALUES('{receipt}','{epoch}','{src}',1,'{json.dumps(envelope)}','{digest}','{digest}',{p.stat().st_size},decode('{p.read_bytes().hex()}','hex'),'fixture',now(),now());
        INSERT INTO relay_deliveries(receipt_id,state,delivery_id,attempt,lease_token,lease_until) VALUES('{receipt}','leased',gen_random_uuid(),1,gen_random_uuid(),now()+interval '5 minutes');""",RSRC)
    stack["store"].request("PUT","joint-parsing/"+key,file=p)
    stored=f"""INSERT INTO ingestion_relays(id,name,configuration_ref) VALUES('{RELAY}','joint fixture','joint-test');
        INSERT INTO ingestion_objects(id,object_key,sha256,byte_length,state) VALUES('{object_id}','{key}','{digest}',{p.stat().st_size},'available');
        INSERT INTO ingestion_receipts(id,relay_id,epoch,receipt_id,source_id,source_revision,source_sequence,tenant_id,envelope,envelope_digest,object_id,received_at)
        VALUES('{local}','{RELAY}','{epoch}','{receipt}','{src}',1,1,'{tenant}','{json.dumps(envelope)}','{digest}','{object_id}',now());
        INSERT INTO ingestion_ack_outbox(receipt_id,payload) VALUES('{local}','{{}}');"""
    command([BIN/"event-relay","resume","--reason",REASON],env=stack["env"])
    command([BIN/"event-relay","enable","--reason",REASON],env=stack["env"])
    pg(stored,SOURCE)
    outbox=pg("SELECT event_id FROM event_outbox WHERE aggregate_id='"+local+"' ORDER BY occurred_at DESC LIMIT 1",SOURCE)
    if not outbox:raise ValueError("Missing canonical ingestion event")
    pg(f"UPDATE event_outbox SET state='leased',attempts=attempts+1,fence=fence+1,lease_until=now()+interval '2 minutes' WHERE event_id='{outbox}'",SOURCE)
    batch=str(uuid.uuid4())
    observation=staged_observation(stack,tenant,batch,digest)
    assert physical_observation(stack,observation)==observation
    publication=pg(f"""INSERT INTO monitoring_jobs(kind,tenant_id,target_id,target_revision,input,input_hash,state,lease_owner,fence,lease_until)
        VALUES('publish','{tenant}','{batch}',1,'{{}}','{digest}','running','joint-publisher',1,now()+interval '2 minutes') RETURNING id""",SOURCE)
    space=pg(f"INSERT INTO file_spaces(tenant_id,scope_kind) VALUES('{tenant}','tenant') ON CONFLICT DO NOTHING RETURNING id",SOURCE) or pg(f"SELECT id FROM file_spaces WHERE tenant_id='{tenant}' AND project_id IS NULL",SOURCE)
    host=pg(f"INSERT INTO file_hosts(space_id,kind,object_id) VALUES('{space}','tenant.profile','{tenant}') ON CONFLICT DO NOTHING RETURNING id",SOURCE) or pg(f"SELECT id FROM file_hosts WHERE space_id='{space}' LIMIT 1",SOURCE)
    member=pg(f"SELECT id FROM tenant_memberships WHERE account_id='{actor}' AND tenant_id='{tenant}' LIMIT 1",SOURCE)
    whole_upload_sha=hashlib.sha256(p.read_bytes()+b"\n").hexdigest()
    upload=pg(f"""INSERT INTO file_uploads(space_id,host_id,actor_id,tenant_membership_id,intent,filename,display_name,expected_size,expected_sha256,declared_media_type,part_size,part_count,policy_snapshot,restore_epoch,reason,idle_expires_at,hard_expires_at,request_id)
      SELECT '{space}','{host}','{actor}','{member}','add','joint.txt','joint upload',{p.stat().st_size+1},'{whole_upload_sha}','text/plain',{p.stat().st_size},2,'{{"schema_version":1}}',restore_epoch,'joint fixture',now()+interval '1 hour',now()+interval '2 hours',gen_random_uuid() FROM file_protocol WHERE id=1 RETURNING id""",SOURCE)
    part=accepted_part(stack,space,upload,p)
    audit=sealed_audit_export(stack,actor)
    presentation=presentation_facts(stack,tenant,project,actor)
    stack["store"].request("PUT","restore-source/"+tenant+"/sources/joint-proof",file=p)
    return {"tenant":tenant,"project":project,"receipt":receipt,"local_receipt":local,"publish_job":publication,"staged_observation":observation,"upload":upload,"received_part":part,"audit_export":audit,"presentation":presentation,"event":outbox,"object_sha256":digest}


def deployment(side,ident):
    path=WORK/side/"installation"
    write_json(path/"config/release.json",{"release":"candidate"})
    write_json(path/"config/architecture/checkpoint.json",{"instance":TAG+"-"+side,"docker":True,"systemd":False})
    return path,Checkpoint(path,ident)


def collect(stack,ident,target,facts):
    src=stack["path"]
    checkbase,freeze=deployment("source",ident)
    image="alpine:3.22"
    # Real process launches. The relay uses its native protected mTLS config and
    # the event relay reports its DB connection over the independently bound port.
    for role in ("ingest-relay","event-relay"):
        cfg_src=src/("relay.json" if role=="ingest-relay" else "maintenance.json")
        cfg=json.loads(cfg_src.read_text());cfg["database_url"]=cfg["database_url"].replace("127.0.0.1","host.docker.internal")
        if role=="ingest-relay":
            cfg.update(certificate_file="/work/tls.pem",key_file="/work/tls.key",client_ca_file="/work/tls.pem",principals_file="/work/source/principals.json")
        dest=src/(role+"-container.json");write_json(dest,cfg)
        name=TAG+"-writer-"+role
        specific=["-e","INFRAPRO_CONFIG=/work/source/"+dest.name,"-e","INFRAPRO_EVENTS_FILE=/work/source/events.json"] if role=="event-relay" else []
        args=["/bin-test/"+role]+(["serve","/work/source/"+dest.name] if role=="ingest-relay" else [])
        docker("run","-d","--name",name,"--label","io.infrapro.instance="+TAG+"-source","--label","io.infrapro.replica="+role+"-1",
               "-v",str(WORK)+":/work:ro","-v",str(LINUX)+":/bin-test:ro",*specific,image,*args)
        writers.append(name)
    time.sleep(1)
    prepared=freeze.prepare()
    if len(prepared["units"])!=2 or any(u["initial"]!="active" for u in prepared["units"].values()):
        raise ValueError("Real ingress/relay writers failed to start")
    for phase in freeze.spec["freezeOrder"]:freeze.change(phase,False)
    sealed=freeze.frozen()
    before=checkpoint_keys(stack,SOURCE,RSRC,facts)
    write_json(WORK/"keys-before.json", before)
    target.mkdir()
    write_json(target/"writer-boundaries.json",{"fixture":sealed})
    with tarfile.open(target/"node-fixture.tar.gz","w:gz") as archive:
        freeze.add_node_data(archive)
        (src/"relay.dump").write_bytes(dump(RSRC))
        (src/"relay-manifest.json").write_text(command([BIN/"ingest-relay","manifest",src/"relay.json"],env=stack["env"]))
        for k in ("relay.dump","relay-manifest.json","relay.json","maintenance.json","events.json"):
            archive.add(src/k,arcname=k)
        archive.add(PACKAGE,arcname="releases/parser-package")
    write_json(target/"event-watermarks.json",json.loads(command([BIN/"event-relay","status"],env=stack["env"])))
    write_json(target/"display-files.json", app(stack,"display","backup-manifest"))
    command([BIN/"ingestion-operator","manifest",target/"inner-receipts.json"],env=stack["env"])
    file=app(stack,"files","backup","prepare","--directory",target/"files","--snapshot-ref",ident,"--reason",REASON,"--external-dump")
    write_json(target/"files-prepare.json",file)
    app(stack,"audit","mode","--mode","read_only","--reason",REASON)
    monitor=app(stack,"monitoring","backup","--directory",target/"monitoring","--reason",REASON,"--pg-tools-container",PG)
    write_json(target/"monitoring-result.json",monitor)
    shutil.copyfile(target/"monitoring/control.dump",target/"files/database.dump")
    objects={}
    for name,bucket in zip(("source","files","audit","parsing"),("restore-source","joint-files","joint-audit","joint-parsing")):
        objects[name]=stack["store"].archive(bucket,target/"objects"/name)
    pages=[];cursor=None
    while True:
        page=app(stack,"audit","backup","manifest","--snapshot-ref",ident,*(["--cursor",cursor] if cursor else []))
        pages.append(page);cursor=page.get("next_cursor")
        if not cursor:break
    write_json(target/"audit-manifest.json",pages)
    app(stack,"files","backup","copy","--id",file["id"],"--directory",target/"files","--reason",REASON)
    app(stack,"audit","mode","--mode","ready","--reason",REASON)
    write_json(target/"files-seal.json",app(stack,"files","backup","seal","--id",file["id"],"--directory",target/"files","--reason",REASON))
    members=[{"path":str(p.relative_to(target)),"sha256":file_hash(p),"bytes":p.stat().st_size} for p in sorted(target.rglob("*")) if p.is_file()]
    write_json(target/"archive-manifest.json",members)
    write_json(target/"checkpoint.json",{"id":ident,"state":"complete","files":len(members),"writers":{"fixture":sealed},"objects":objects})
    # Reopen exactly the originally active instances, then stop the isolated
    # source. Recovery has no network dependency on this database or its stores.
    for phase in reversed(freeze.spec["freezeOrder"]):freeze.change(phase,True)
    for name in writers:docker("stop",name)
    return objects,before


def execute():
    command(["openssl","req","-x509","-newkey","ec","-pkeyopt","ec_paramgen_curve:P-256","-nodes","-keyout",WORK/"tls.key","-out",WORK/"tls.pem","-days","2","-subj","/CN=joint restore fixture","-addext","subjectAltName=IP:127.0.0.1","-addext","basicConstraints=critical,CA:TRUE"])
    arch={"aarch64":"arm64","arm64":"arm64","x86_64":"amd64","amd64":"amd64"}[docker("info","--format","{{.Architecture}}")]
    for directory,env,commands in ((BIN,os.environ,["./tools/infrapro/cmd/infrapro","./tools/infrapro/cmd/ingestion-operator","./services/ingest-relay/cmd/ingest-relay","./services/api/cmd/event-relay","./services/api/cmd/display-runtime","./services/parser/cmd/parser","./services/parser/cmd/parser-runner","./services/parser/cmd/parser-package"]),(LINUX,{**os.environ,"CGO_ENABLED":"0","GOOS":"linux","GOARCH":arch},["./services/api/cmd/event-relay","./services/ingest-relay/cmd/ingest-relay"])):
        directory.mkdir()
        compile_commands=[]
        for entry in commands:
            name=pathlib.Path(entry).name
            supplied=pathlib.Path(ctx["application_bins"])/name if directory==BIN and ctx.get("application_bins") else None
            if supplied and supplied.is_file():
                (directory/name).symlink_to(supplied.resolve(strict=True))
            else:
                compile_commands.append(entry)
        if compile_commands:
            command(["go","build","-trimpath","-o",str(directory)+"/",*compile_commands],env=env)
    command([BIN/"parser-package",BIN/"parser-runner",ROOT/"samples/parsing",PACKAGE])
    pg("CREATE DATABASE "+SOURCE+" TEMPLATE arc_restore",BASE)
    for db in (NEW,RSRC,RDST):pg("CREATE DATABASE "+db,SOURCE)
    source=profile("source")
    if ctx.get("application_bins"):
        rolesdb="arc_roles_"+short
        pg("CREATE DATABASE "+rolesdb+" TEMPLATE "+SOURCE,BASE)
        smokecfg=json.loads((source["path"]/"maintenance.json").read_text());smokecfg["database_url"]=uri(rolesdb)
        write_json(WORK/"roles.json",smokecfg)
        app(source,"monitoring","storage","init")
        command(["node",ROOT/"scripts/architecture-role-smoke.mjs",WORK/"roles.json",WORK/"smoke",PG,ctx["application_bins"],WORK/"tls.pem",PACKAGE,RELAY,source["ch"]],env=source["env"])
    facts=source_facts(source)
    source_credentials=display_access(source,facts["presentation"],False)
    output=WORK/"checkpoint"
    ident=datetime.datetime.now(datetime.timezone.utc).strftime("checkpoint-%Y%m%dT%H%M%SZ")
    objects,before=collect(source,ident,output,facts)
    assert before["relay"]["state"]=="leased" and before["relay"]["retained_bytes"]>0
    assert before["relay"]["receipt_id"]==before["local_ack"]["receipt_id"]==facts["receipt"]
    assert before["relay"]["sha256"]==before["local_ack"]["sha256"]==facts["object_sha256"]
    assert before["local_ack"]["ack_intents"]==1 and before["event"]["state"]=="leased"
    assert before["publication"]["state"]=="running" and before["publication"]["selected"]==0
    assert before["file"]["state"]=="receiving" and before["file"]["part_count"]==2 and before["file"]["part_state"]=="received"
    assert before["display"]["session_state"]=="ready" and before["display"]["terminal_state"]=="active"
    assert before["display"]["has_token"] and before["display"]["has_pairing"]
    source["compose"]("stop")
    target=profile("target")
    base,check=deployment("target",ident)
    check.prepare()
    for phase in check.spec["freezeOrder"]:check.change(phase,False)
    config={"bin_directory":str(BIN),"work_directory":str(WORK/"recovery"),"business_config":str(target["path"]/"maintenance.json"),"events_config":str(target["path"]/"events.json"),"audit_content_config":str(target["path"]/"audit-content.json"),"relay_config":str(target["path"]/"relay.json"),"parser_package":str(PACKAGE),"ca_file":str(WORK/"tls.pem"),"pg_tools_container":PG,"relay_pg_tools_container":PG,"writer_hosts":[{"base":str(base),"checkpoint":ident}]}
    write_json(WORK/"target.json",config)
    index=file_hash(output/"archive-manifest.json")
    def restore(action):
        return json.loads(command([sys.executable,PROD/"architecture-restore.py",action,"--checkpoint",output,"--manifest-sha256",index,"--target",WORK/"target.json","--reason",REASON]))
    restore("verify")
    record=restore("apply")
    assert record["state"]=="verified_read_only"
    assert pg("SELECT mode FROM monitoring_environment_state",NEW)=="read_only"
    assert pg("SELECT state FROM file_uploads WHERE id='"+facts["upload"]+"'",NEW)=="expired"
    assert pg("SELECT id FROM monitoring_jobs WHERE id='"+facts["publish_job"]+"'",NEW)==facts["publish_job"]
    assert pg("SELECT count(*) FROM account_sessions WHERE revoked_at IS NULL",NEW)=="0"
    assert pg("SELECT count(*) FROM ingestion_ack_outbox WHERE receipt_id='"+facts["local_receipt"]+"'",NEW)=="1"
    assert pg("SELECT count(*) FROM event_outbox WHERE event_id='"+facts["event"]+"'",NEW)=="1"
    after=checkpoint_keys(target,NEW,RDST,facts)
    write_json(WORK/"keys-after.json", after)
    for key in ("relay","local_ack","observation","audit","object_content"):
        assert after[key]==before[key],key
    assert after["file"]=={**before["file"],"state":"expired"}
    for key in ("session","terminal","grant_revoked","package_id","release_id","sha256"):
        assert after["display"][key]==before["display"][key],key
    assert not after["display"]["grant_revoked"] and after["display"]["session_state"]=="revoked"
    assert after["display"]["terminal_state"]=="unpaired" and not after["display"]["has_token"] and not after["display"]["has_pairing"]
    assert after["display"]["generation"]==before["display"]["generation"]+1
    assert after["display"]["environment"]!=before["display"]["environment"]
    assert after["publication"]["id"]==before["publication"]["id"] and after["publication"]["selected"]==0
    for key in ("event_id","type","scope","aggregate_id","restore_epoch"):
        assert before["event"][key]==after["event"][key]
    verified=json.loads((WORK/"recovery/recovery.json").read_text())["steps"]
    assert verified["display"]["result"]["package_files"]==1 and verified["display"]["result"]["matched_frozen_inventory"]
    assert verified["spatial"]["result"]["tenants"]==[{"tenant":facts["tenant"],"versions":1,"files":1,"bytes":len(b'{"type":"FeatureCollection","features":[]}')}]
    assert int(verified["files_verify"]["result"]["checked"])>0
    assert int(verified["audit_verify"]["result"]["checked"])>0
    assert all(objects[k]>0 for k in ("source","files","parsing","audit"))
    assert restore("open")["state"]=="open"
    old_credentials=display_access(target,facts["presentation"],True)
    return {"arc":"ARC-034","status":"passed","cold_starts":json.loads((WORK/"smoke/cold-start.json").read_text()) if (WORK/"smoke/cold-start.json").is_file() else None,"checkpoint":ident,"manifest_sha256":index,"rows":facts,"keys":{"before":before,"after":after},"object_counts":objects,"before_open":"verified_read_only","after_open":"ready","source_display_credentials":source_credentials,"old_display_credentials":old_credentials,"stopped_source_ch_and_objects":True,"steps":json.loads((WORK/"recovery/recovery.json").read_text())["steps"]}


if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("--keep-failed",action="store_true");parser.add_argument("--context",type=pathlib.Path,required=True);parser.add_argument("--output",type=pathlib.Path,required=True)
    opt=parser.parse_args();os.umask(0o077)
    ctx=json.loads(opt.context.read_text());DB=urllib.parse.urlsplit(ctx["database"]);PG=ctx["pg"];BASE=DB.path[1:];USER=DB.username
    if DB.hostname!="127.0.0.1" or BASE!="arc_b" or not re.fullmatch("arc-deploy-[0-9a-f]{10}-pg",PG):
        raise SystemExit("Expected the isolated deployment-check database")
    TAG="arc-joint-"+uuid.uuid4().hex[:10];short=TAG[-10:];RELAY=str(uuid.uuid4());SOURCE="arc_source_"+short;NEW="arc_target_"+short;RSRC="arc_relay_"+short;RDST="arc_targetrelay_"+short
    WORK=opt.output.resolve()/"joint-private";WORK.mkdir(mode=0o700,parents=True);BIN=WORK/"bin";LINUX=WORK/"linux-bin";PACKAGE=WORK/"parser-package"
    REASON="架构隔离联合恢复验证";stacks=[];writers=[];owned_networks=[]
    try:
        report=execute()
    except Exception as error:
        report={"arc":"ARC-034","status":"failed","phase":str(error)}
        raise
    finally:
        for name in writers:subprocess.run(["docker","rm","-f",name],capture_output=True)
        if not opt.keep_failed or report["status"]=="passed":
            for stack in reversed(stacks):subprocess.run(["docker","compose","-f",str(stack),"down","--volumes"],capture_output=True)
            for network in reversed(owned_networks):
                removed=subprocess.run(["docker","network","rm",network["id"]],capture_output=True)
                network["removed"]=removed.returncode==0
        cleanup_failed=report["status"]=="passed" and any(not network.get("removed") for network in owned_networks)
        if cleanup_failed:
            report.update(status="failed",phase="isolated network cleanup failed")
        report["isolated_networks"]=owned_networks
        write_json(opt.output/"joint-recovery.json",report)
        if cleanup_failed:
            raise RuntimeError(report["phase"])
