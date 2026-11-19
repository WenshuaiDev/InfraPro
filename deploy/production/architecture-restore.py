#!/usr/bin/env python3
"""Verify a sealed checkpoint, recover an independent target, then explicitly open it."""
import argparse
import fcntl
import hashlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import tarfile
import urllib.parse
import uuid

from checkpoint_roles import Checkpoint, SAFE_ID, utc, write_json
from s3_archive import S3, file_hash


def read(path):
    return json.loads(pathlib.Path(path).read_text())


def verify(directory, expected):
    state = read(directory/"checkpoint.json")
    if state["state"] != "complete" or not SAFE_ID.fullmatch(state["id"]):
        raise ValueError("Checkpoint has not been sealed")
    index = directory/"archive-manifest.json"
    if not re.fullmatch("[0-9a-f]{64}", expected) or file_hash(index) != expected:
        raise ValueError("Checkpoint manifest checksum mismatch")
    paths = set()
    for item in read(index):
        path = directory/item["path"]
        if not path.resolve().is_relative_to(directory) or item["path"] in paths or path.is_symlink() or not path.is_file():
            raise ValueError("Unexpected or duplicate archive path")
        if path.stat().st_size != item["bytes"] or file_hash(path) != item["sha256"]:
            raise ValueError("Archive member checksum mismatch")
        paths.add(item["path"])
    actual = {str(p.relative_to(directory)) for p in directory.rglob("*") if p.is_file()}
    if actual != paths | {"checkpoint.json", "archive-manifest.json"}:
        raise ValueError("Unlisted or missing archive member")
    sealed = read(directory/"writer-boundaries.json")
    if sealed != state["writers"] or not sealed:
        raise ValueError("Checkpoint writer boundary does not match manifest")
    for node, writer in sealed.items():
        if not re.fullmatch(r"[a-zA-Z0-9_-]+", node) or ("node-"+node+".tar.gz") not in paths:
            raise ValueError("Missing node archive")
        if not writer["phases"] or any(p["action"] != "stopped" for p in writer["phases"].values()):
            raise ValueError("Checkpoint lacks a frozen writer phase")
        with tarfile.open(directory/("node-"+node+".tar.gz"), "r:gz") as node_archive:
            if json.load(node_archive.extractfile("writer-checkpoint.json")) != writer:
                raise ValueError("Node writer checkpoint differs from coordinator")
    for key in ("monitoring/control.dump","monitoring/manifest.json","files/database.dump","files-seal.json","audit-manifest.json","event-watermarks.json"):
        if key not in paths:
            raise ValueError("Missing joint recovery evidence: "+key)
    if file_hash(directory/"files/database.dump") != file_hash(directory/"monitoring/control.dump"):
        raise ValueError("File and monitoring checkpoint use different databases")
    return state


def command(args, env, step, *, input_file=None, input_bytes=None, json_output=True):
    # Config and credential paths stay in the protected environment and reports.
    proc = subprocess.run(list(map(str,args)), env=env, input=input_bytes,
                          stdin=None if input_bytes is not None else input_file or subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if proc.returncode:
        if "INFRAPRO_RECOVERY_ERROR_LOG" in env:
            with pathlib.Path(env["INFRAPRO_RECOVERY_ERROR_LOG"]).open("ab") as out:
                out.write(step.encode()+b"\n"+proc.stderr)
        raise RuntimeError(step+" failed (exit "+str(proc.returncode)+"). Target remains fenced.")
    return json.loads(proc.stdout) if json_output and proc.stdout.strip() else proc.stdout.decode().strip()


def pg_args(dsn, container, tool, args, env):
    uri = urllib.parse.urlsplit(dsn)
    if uri.scheme not in ("postgres", "postgresql") or not uri.hostname or not uri.path.strip("/"):
        raise ValueError("Expected a target PostgreSQL connection URL")
    values = {
        "PGHOST": uri.hostname, "PGPORT": str(uri.port or 5432), "PGDATABASE": urllib.parse.unquote(uri.path[1:]),
        "PGUSER": urllib.parse.unquote(uri.username or ""), "PGPASSWORD": urllib.parse.unquote(uri.password or ""),
    }
    for key, value in urllib.parse.parse_qsl(uri.query):
        mapped = {"sslmode":"PGSSLMODE","sslrootcert":"PGSSLROOTCERT","sslcert":"PGSSLCERT","sslkey":"PGSSLKEY","options":"PGOPTIONS"}
        if key not in mapped:
            raise ValueError("Unsupported restore DSN parameter")
        values[mapped[key]] = value
    prefix = []
    if container:
        if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", container):
            raise ValueError("Invalid PG tools container")
        # Same contract as the monitoring go CLI for a DB in its own container.
        values["PGHOST"], values["PGPORT"] = "127.0.0.1", "5432"
        prefix = ["docker","exec","-i"]+[element for k in values for element in ("-e",k)]+[container]
    return prefix+[tool,*args], {**env, **values}


def storage(cfg, name, ca):
    if name == "source":
        return S3(cfg["monitoring_object_endpoint"],cfg["monitoring_object_access_key"],cfg["monitoring_object_secret_key"],ca), cfg["monitoring_source_bucket"]
    domain = cfg[name]
    return S3(domain["object_endpoint"],domain.get("access_key",domain.get("object_access_key")),
              domain.get("secret_key",domain.get("object_secret_key")),ca), domain["bucket"]


class Recovery:
    def __init__(self, options, state):
        self.source = options.checkpoint.resolve()
        self.target = read(options.target)
        self.id = state["id"]
        self.tgt_hash = file_hash(options.target)
        self.index_hash = options.manifest_sha256
        self.reason = options.reason
        self.work = pathlib.Path(self.target["work_directory"])
        self.bin = pathlib.Path(self.target["bin_directory"])
        if not self.work.is_absolute() or not self.bin.is_absolute() or self.work.resolve().is_relative_to(self.source):
            raise ValueError("Expected separate absolute target and work directories")
        self.work.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.ca = pathlib.Path(self.target["ca_file"])
        self.app_config = pathlib.Path(self.target["business_config"])
        self.events_config = pathlib.Path(self.target["events_config"])
        self.base_env = {k:v for k,v in os.environ.items() if not k.startswith(("INFRAPRO_","PG"))}
        self.base_env["INFRAPRO_RECOVERY_ERROR_LOG"] = str(self.work/"command-errors.log")
        self.env = {**self.base_env,"INFRAPRO_CONFIG":str(self.app_config),"INFRAPRO_EVENTS_FILE":str(self.events_config),"SSL_CERT_FILE":str(self.ca)}
        self.cfg = read(self.app_config)
        self.pgcontainer = self.target.get("pg_tools_container")
        self.audit_content = pathlib.Path(self.target["audit_content_config"])
        self.relay = self.target.get("relay_config")
        self.relaycfg = read(self.relay) if self.relay else {}
        if self.relay:
            def identity(cfg):
                uri=urllib.parse.urlsplit(cfg["database_url"])
                return (uri.hostname,uri.port or 5432,urllib.parse.unquote(uri.path))
            if identity(self.cfg)==identity(self.relaycfg):
                raise ValueError("Business and relay databases must be distinct")
        self.state_path = self.work/"recovery.json"
        self.state = {"id": self.id, "manifest_sha256": self.index_hash, "target_sha256": self.tgt_hash,
                      "audit_content_sha256":file_hash(self.audit_content),"business_config_sha256": file_hash(self.app_config),"events_config_sha256":file_hash(self.events_config),
                      "state":"fenced","steps":{}}
        if self.relay:
            self.state["relay_config_sha256"] = file_hash(self.relay)
        if self.state_path.exists():
            saved = read(self.state_path)
            for key in self.state.keys()-{"state","steps"}:
                if self.state[key] != saved.get(key):
                    raise ValueError("Target or checkpoint changed since recovery")
            self.state = saved

    def record(self, name, result):
        self.state["steps"][name] = {"at":utc(), "result":result}
        write_json(self.state_path,self.state)
        return result

    def verify_display(self):
        inventory=self.app("display","backup-manifest")
        source_manifest=self.source/"display-files.json"
        frozen=read(source_manifest) if source_manifest.is_file() else None
        intent=self.state["steps"].get("display_invalidation",{}).get("result")
        previous=intent["old_epoch"] if intent else inventory["environment"]
        if frozen and (frozen["files"]!=inventory["files"] or frozen["environment"]!=previous):
            raise ValueError("Restored display package inventory differs from the frozen source")
        if self.app("display","verify-storage"):
            raise ValueError("A retained display package failed content verification")
        if not intent:
            self.record("display_invalidation",{"old_epoch":previous})
        generation=inventory["environment"]
        if generation==previous:
            generation=self.app("display","restore-invalidate")
        if not generation or generation==previous:
            raise ValueError("Restored display sessions or terminals retained their old generation")
        active=self.query("business","""SELECT (SELECT count(*) FROM display_sessions WHERE state IN ('starting','ready','degraded'))
            +(SELECT count(*) FROM display_terminals WHERE state='active' OR token_hash IS NOT NULL OR pair_hash IS NOT NULL)
            +(SELECT count(*) FROM display_read_jobs WHERE state IN ('queued','running'))""")
        if active!="0":
            raise ValueError("Old display credentials or reader leases remain active")
        self.record("display",{"old_epoch":previous,"new_epoch":generation,"package_files":len(inventory["files"]),
                               "matched_frozen_inventory":source_manifest.is_file(),"old_active":0})

    def verify_spatial(self):
        reports=[]
        tenants=self.query("business","SELECT DISTINCT tenant_id FROM spatial_asset_versions ORDER BY tenant_id")
        for tenant in tenants.splitlines():
            text=command([self.bin/"infrapro","spatial","verify","--tenant",tenant],self.env,"spatial content verification",json_output=False)
            versions=[json.loads(row) for row in text.splitlines()]
            if any(not version["available"] for version in versions):
                raise ValueError("A retained spatial version failed content verification")
            reports.append({"tenant":tenant,"versions":len(versions),"files":sum(v["files"] for v in versions),
                            "bytes":sum(v["bytes"] for v in versions)})
        self.record("spatial",{"tenants":reports})

    def verify_display_and_spatial(self):
        if "display" not in self.state["steps"]:
            self.verify_display()
        if "spatial" not in self.state["steps"]:
            self.verify_spatial()

    def app(self, *args):
        env = self.env
        if args == ("audit","verify","--mode","content"):
            creds = read(self.audit_content)
            if creds["access_key"] == self.cfg["audit"].get("access_key"):
                raise ValueError("Audit content verifier needs its dedicated read-only object identity")
            env = {**env, "INFRAPRO_AUDIT_CONTENT_ACCESS_KEY":creds["access_key"], "INFRAPRO_AUDIT_CONTENT_SECRET_KEY":creds["secret_key"]}
            if creds.get("database_url"):
                env["INFRAPRO_DATABASE_URL"] = creds["database_url"]
        return command([self.bin/"infrapro", *args], env, "infrapro "+args[0]+" "+args[1])

    def event(self, *args):
        return command([self.bin/"event-relay", *args], self.env, "event-relay "+args[0])

    def query(self, db, stmt):
        cfg, container = (self.relaycfg, self.target.get("relay_pg_tools_container")) if db == "relay" else (self.cfg, self.pgcontainer)
        args, env = pg_args(cfg["database_url"],container,"psql",["-XqAt","-v","ON_ERROR_STOP=1","-c",stmt],self.base_env)
        return command(args,env,"database readback",json_output=False)

    def restore_schema(self, backup_id):
        # Earlier schema-59 checkpoints predate one or both metadata readers.
        # Keep the restored database fenced while the release CLI finishes any
        # same-version installation and before restoring function/table ACLs.
        statement = """SELECT json_build_object(
          'schema', (SELECT max(g.version_id) FROM (SELECT DISTINCT ON (version_id) version_id,is_applied
                    FROM goose_db_version ORDER BY version_id,id DESC) g WHERE g.is_applied),
          'monitoring', json_build_object('mode',m.mode,'access_revoked',m.access_revoked,'backup_id',m.backup_id),
          'epochs', json_build_object('event',e.restore_epoch,'audit',a.restore_epoch,
                                      'tasks',t.restore_epoch,'files',f.restore_epoch),
          'baseline_function', (SELECT json_build_object('name',p.proname,
                 'arguments',oidvectortypes(p.proargtypes),'result',pg_get_function_result(p.oid),
                 'security_definer',p.prosecdef,'definition_sha256',
                 encode(sha256(convert_to(pg_get_functiondef(p.oid),'UTF8')),'hex'))
             FROM pg_proc p WHERE p.oid=to_regprocedure(format(
                  '%I.architecture_fixed_baseline_metadata(uuid,uuid,text,text)',current_schema()))),
          'notification_invitation_function', (SELECT json_build_object('name',p.proname,
                 'arguments',oidvectortypes(p.proargtypes),'result',pg_get_function_result(p.oid),
                 'security_definer',p.prosecdef,'definition_sha256',
                 encode(sha256(convert_to(pg_get_functiondef(p.oid),'UTF8')),'hex'),
                 'fixed_search_path',p.proconfig @> ARRAY[format('search_path=pg_catalog, %I, pg_temp',current_schema())],
                 'public_execute',EXISTS (SELECT FROM aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a
                    WHERE a.grantee=0 AND a.privilege_type='EXECUTE'))
             FROM pg_proc p WHERE p.oid=to_regprocedure(format(
                  '%I.architecture_notification_invitation_targets(uuid,uuid,uuid[])',current_schema()))))
          FROM monitoring_environment_state m CROSS JOIN event_protocol e CROSS JOIN audit_protocol a
          CROSS JOIN task_protocol t CROSS JOIN file_protocol f
          WHERE m.id=1 AND e.id=1 AND a.id=1 AND t.id=1 AND f.id=1"""
        before = json.loads(self.query("business", statement))
        guard = before["monitoring"]
        if (before["schema"] != 59 or guard["mode"] != "read_only" or not guard["access_revoked"]
                or guard["backup_id"] != backup_id):
            raise ValueError("Expected a fenced, verified schema-59 business restore before applying role ACLs")
        cli = self.bin / "infrapro"
        command([cli,"database","migrate"], self.env, "infrapro database migrate", json_output=False)
        after = json.loads(self.query("business", statement))
        for key in ("schema", "monitoring", "epochs"):
            if before[key] != after[key]:
                raise ValueError("Business restore state changed during same-version schema installation: " + key)
        function = after["baseline_function"]
        if not function or not function["security_definer"] or function["arguments"] != "uuid, uuid, text, text" or function["result"] != "TABLE(snapshot jsonb, input_ranges jsonb)":
            raise ValueError("Restored schema is missing the release's restricted baseline projection")
        invitation = after["notification_invitation_function"]
        expected = ("TABLE(invitation_id uuid, is_recipient boolean, inviter_id text, inviter_membership_id text, "
                    "effective_status text, membership_status text, accepted_tenant_name text)")
        if (not invitation or not invitation["security_definer"] or not invitation["fixed_search_path"]
                or invitation["public_execute"] or invitation["arguments"] != "uuid, uuid, uuid[]" or invitation["result"] != expected):
            raise ValueError("Restored schema is missing the protected personal invitation reader")
        return {"cli_sha256": file_hash(cli), "before": before, "after": after}

    def frozen(self, allow_brokers=False):
        # New target hosts have independent service manifests; the source units may
        # use an entirely different placement. Query the actual target every time.
        targets = self.target["writer_hosts"]
        if not targets:
            raise ValueError("Target writer host inventory required")
        for entry in targets:
            if "base" in entry:
                Checkpoint(pathlib.Path(entry["base"]), entry["checkpoint"]).frozen(allow_brokers=allow_brokers)
            else:
                alias = entry["ssh_alias"]
                if not re.fullmatch("[a-zA-Z0-9][a-zA-Z0-9_-]*",alias) or not SAFE_ID.fullmatch(entry["checkpoint"]):
                    raise ValueError("Invalid target freeze binding")
                cmd = ["ssh","-o","BatchMode=yes","-F",self.target["ssh_config"],alias,("checkpoint-business-frozen " if allow_brokers else "checkpoint-frozen ")+entry["checkpoint"]]
                result = command(cmd,self.base_env,"target freeze verification")
                if not result["phases"] or any(p["action"]!="stopped" for n,p in result["phases"].items() if not (allow_brokers and n=="broker")):
                    raise ValueError("Remote target writer is active")

    def recover_relay(self, writers):
        candidates = [n for n,s in writers.items() if any(u["role"]=="ingest-relay" for u in s["units"].values())]
        if not candidates:
            if self.relay:
                raise ValueError("Relay target supplied without a source relay checkpoint")
            return
        if len(candidates)!=1 or not self.relay:
            raise ValueError("Expected exactly one independent relay target")
        dump = self.work/"relay.dump"
        manifest = self.work/"cloud-receipts.json"
        with tarfile.open(self.source/("node-"+candidates[0]+".tar.gz"),"r:gz") as archive:
            for name,dest in [("relay.dump",dump),("relay-manifest.json",manifest)]:
                entry = archive.getmember(name)
                if not entry.isfile():
                    raise ValueError("Expected regular relay archive member")
                with archive.extractfile(entry) as src, dest.open("xb") as dst:
                    shutil.copyfileobj(src,dst,1 << 20)
        args,pg_env = pg_args(self.relaycfg["database_url"],self.target.get("relay_pg_tools_container"),
                              "pg_restore",["--single-transaction","--exit-on-error","--no-acl","--no-owner","--dbname="+urllib.parse.unquote(urllib.parse.urlsplit(self.relaycfg["database_url"]).path[1:])],self.base_env)
        with dump.open("rb") as stream:
            command(args,pg_env,"relay pg_restore",input_file=stream,json_output=False)
        before = self.query("relay","SELECT spool_epoch FROM relay_state WHERE id=1")
        command([self.bin/"ingest-relay","new-epoch",self.relay],self.env,"relay epoch rotation",json_output=False)
        after = self.query("relay","SELECT spool_epoch FROM relay_state WHERE id=1")
        restored = command([self.bin/"ingest-relay","manifest",self.relay],self.env,"relay receipt manifest")
        if not after or before==after or restored!=read(manifest):
            raise ValueError("Relay receipt or epoch reconciliation failed")
        cmd = self.bin/"ingestion-operator"
        local_manifest = self.work/"inner-receipts.json"
        command([cmd,"manifest",local_manifest],self.env,"inner receipt boundary",json_output=False)
        if read(local_manifest)!=read(self.source/"inner-receipts.json"):
            raise ValueError("Inner receipt boundary changed")
        counts = command([cmd,"reconcile",manifest],self.env,"cloud custody reconciliation")
        objects = command([cmd,"objects",self.work/"ingestion-objects.json"],self.env,"inner object integrity")
        packages = command([cmd,"release",self.target["parser_package"]],self.env,"frozen parser method coverage")
        self.record("custody",{"old_epoch":before,"new_epoch":after,"receipts":len(restored),"reconciliation":counts,"objects":objects,"method_coverage":packages})

    def restore_diagnostics(self, writers):
        # Restore bbolt while diagnostics writers are stopped. The target server
        # config supplies its own address, trust roots and private key material.
        for node, writer in writers.items():
            if not any(e["role"]=="diagnostics" for e in writer["units"].values()):
                continue
            binding=self.target.get("diagnostics",{}).get(node)
            if not binding:
                raise ValueError("Missing independent target directory for archived diagnostics node "+node)
            dest=pathlib.Path(binding["state_directory"])
            if not dest.is_absolute() or dest.is_symlink() or (dest.exists() and any(dest.iterdir())):
                raise ValueError("Expected an empty diagnostics target directory")
            dest.mkdir(mode=0o700,parents=True,exist_ok=True)
            measured=[]
            with tarfile.open(self.source/("node-"+node+".tar.gz"),"r:gz") as archive:
                for member in archive:
                    name=pathlib.PurePosixPath(member.name)
                    if not name.parts or name.parts[0]!="diagnostics-state" or member.isdir():
                        continue
                    if ".." in name.parts or not member.isfile():
                        raise ValueError("Unexpected diagnostics archive member")
                    target=dest.joinpath(*name.parts[1:])
                    if not target.resolve().is_relative_to(dest.resolve()):
                        raise ValueError("Unexpected diagnostics target path")
                    target.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
                    with archive.extractfile(member) as src,target.open("xb") as out:
                        shutil.copyfileobj(src,out,1<<20)
                    measured.append({"path":str(target.relative_to(dest)),"bytes":member.size,"sha256":file_hash(target)})
            if not measured:
                raise ValueError("No diagnostics state in sealed node archive")
            result=command([self.bin/"infrapro","diagnostics","state","inspect","--server",binding["server_config"]],self.env,"diagnostics state verification")
            self.record("diagnostics_"+node,{"files":measured,"inspection":result})

    def apply(self, state):
        if self.state_path.exists():
            raise ValueError("Recovery has already started in this directory; inspect its evidence or use a new empty target")
        self.frozen()
        for database in (["business","relay"] if self.relay else ["business"]):
            if self.query(database,"SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')")!="0":
                raise ValueError("Expected an empty "+database+" database")
        stores = {name:storage(self.cfg,name,self.ca) for name in state["objects"]}
        for store,bucket in stores.values():
            if next(iter(store.list(bucket)),None) is not None:
                raise ValueError("Expected empty private target bucket")
        self.record("empty_target",{"frozen":True,"objects":list(stores)})
        deploy = pathlib.Path(__file__).resolve().parents[1]
        pgcmd, pg_env = pg_args(self.cfg["database_url"],self.pgcontainer,"psql",["-Xq","-v","ON_ERROR_STOP=1"],self.base_env)
        with (deploy/"architecture/group-identities.sql").open("rb") as groups:
            command(pgcmd,pg_env,"target role identities",input_file=groups,json_output=False)
        mon = self.source/"monitoring"
        opt = ["--pg-tools-container",self.pgcontainer] if self.pgcontainer else []
        result = self.app("monitoring","restore","--directory",str(mon),"--manifest-sha256",file_hash(mon/"manifest.json"),"--apply","--reason",self.reason,*opt)
        if result["state"]!="read_only" or not result["verified"]:
            raise ValueError("Domain recovery guard did not reach read-only")
        self.record("monitoring",{"backup_id":result["backup"]["id"],"mode":result["state"],"access_revoked":True})
        schema_installation = self.restore_schema(result["backup"]["id"])
        args, pg_env = pg_args(self.cfg["database_url"],self.pgcontainer,"psql",["-Xq","-v","ON_ERROR_STOP=1"],self.base_env)
        applied={}
        for rel in ("architecture/restore-base-grants.sql","display/package-worker-role.sql","architecture/postgres-roles.sql","events/postgres-grants.sql"):
            body=(deploy/rel).read_bytes()
            command(args,pg_env,"restored role ACLs",input_bytes=body,json_output=False)
            applied["deploy/"+rel]=hashlib.sha256(body).hexdigest()
        self.record("role_grants",{"schema":59,"schema_installation":schema_installation,
                                   "domain_and_split_roles":True,"applied_role_sql":applied})
        old = self.event("status")["protocol"]["restore_epoch"]
        if old!=read(self.source/"event-watermarks.json")["protocol"]["restore_epoch"]:
            raise ValueError("Event checkpoint does not match restored database")
        self.event("freeze","--epoch",old,"--reason",self.reason)
        # Go restores the sealed, application-visible file objects. The full bucket
        # archive additionally covers durable temporary/reference and quarantine keys.
        self.app("files","restore","objects","--directory",str(self.source/"files"),"--reason",self.reason)
        for name,(store,bucket) in stores.items():
            count = store.restore(bucket,self.source/"objects"/name)
            if count!=state["objects"][name]:
                raise ValueError("Object snapshot cardinality changed")
            self.record("objects_"+name,{"count":count,"content_verified":True})
        self.verify_display_and_spatial()
        self.recover_relay(state["writers"])
        self.restore_diagnostics(state["writers"])
        self.app("audit","mode","--mode","reconciling","--reason",self.reason)
        self.app("audit","restore","reconcile","--reason",self.reason)
        self.app("files","restore","reconcile","--reason",self.reason)
        rotated = self.event("rotate-epoch","--epoch",old,"--reason",self.reason)
        if rotated["restore_epoch"]==old or not rotated["frozen"]:
            raise ValueError("Event/dispatch epoch guard failed")
        self.record("events",{"old_epoch":old,"new_epoch":rotated["restore_epoch"],"frozen":True})
        for subsystem in ("files","audit"):
            result = self.app(subsystem,"verify","--mode","content")
            if subsystem=="files" and (not result["complete"] or result["items"]):
                raise ValueError("File integrity reconciliation failed")
            if subsystem=="audit" and (int(result["invalid"]) or int(result["unclassified"])):
                raise ValueError("Audit integrity reconciliation failed")
            self.record(subsystem+"_verify",result)
        self.frozen()
        if not self.event("status")["protocol"]["frozen"]:
            raise ValueError("Event namespace reopened during reconciliation")
        self.state["state"]="verified_read_only"
        self.record("verified",{"public_writers":"stopped","monitoring":"read_only","events":"frozen"})

    def open(self):
        if self.state["state"] not in ("verified_read_only","opening"):
            raise ValueError("Complete and verify recovery before opening writes")
        self.frozen(allow_brokers=True)
        if "display" not in self.state["steps"] or "spatial" not in self.state["steps"]:
            self.verify_display_and_spatial()
        epoch = self.state["steps"]["events"]["result"]["new_epoch"]
        snapshot = self.event("status")["protocol"]
        if snapshot["restore_epoch"]!=epoch:
            raise ValueError("Restored event epoch changed")
        self.state["state"]="opening"
        write_json(self.state_path,self.state)
        if read(self.events_config).get("mode")=="jetstream":
            self.event("provision")
        if self.query("business","SELECT mode FROM monitoring_environment_state WHERE id=1")!="ready":
            self.app("monitoring","restore","open","--backup-id",self.state["steps"]["monitoring"]["result"]["backup_id"],"--reason",self.reason)
        self.app("audit","mode","--mode","ready","--request-id",str(uuid.uuid5(uuid.NAMESPACE_URL,self.id+"."+epoch)),"--reason",self.reason)
        if snapshot["frozen"]:
            self.event("resume","--epoch",epoch,"--reason",self.reason)
        self.state["state"]="open"
        self.record("opened",{"restore_epoch":epoch,"restart":"only approved target services after operator verification"})


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action",choices=["verify","apply","open"])
    parser.add_argument("--checkpoint",type=pathlib.Path,required=True)
    parser.add_argument("--manifest-sha256",required=True)
    parser.add_argument("--target",type=pathlib.Path)
    parser.add_argument("--reason")
    opt=parser.parse_args()
    os.umask(0o077)
    state=verify(opt.checkpoint.resolve(),opt.manifest_sha256)
    if opt.action=="verify":
        print(json.dumps({"id":state["id"],"verified":True,"files":state["files"]}))
        return
    if not opt.target or not opt.reason or not opt.reason.strip():
        parser.error("Target JSON and reason required")
    recovery=Recovery(opt,state)
    with (recovery.work/".lock").open("a") as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        if opt.action=="apply":
            recovery.apply(state)
        else:
            recovery.open()
    print(json.dumps({"id":state["id"],"state":recovery.state["state"]}))


if __name__=="__main__":
    main()
