"""Exercise the emitted object IAM policies on a disposable SeaweedFS 4.53 instance."""
import argparse
import hashlib
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import time
import urllib.parse
import uuid
import xml.etree.ElementTree as ET

IMAGE = "chrislusf/seaweedfs:4.53@sha256:ccad23e18639c8b98cee17e7100b95830a06077b56d8e5b006d6dcf8ec5e94a0"


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, **kwargs).stdout


def check(layout, output):
    image_tag = uuid.uuid4().hex[:12]
    name = "arc-object-policy-" + image_tag
    buckets = {k: "arc-" + image_tag + "-" + k for k in ("parsing", "source", "files", "audit")}
    admin = ("arc" + secrets.token_hex(10), secrets.token_hex(24))
    events = []
    with tempfile.TemporaryDirectory(dir=output.parent, prefix="object-policy-") as temporary:
        folder = pathlib.Path(temporary)
        config = folder / "identities.json"
        config.write_text(json.dumps({"identities": [{"name": "test-owner", "credentials": [
            {"accessKey": admin[0], "secretKey": admin[1]}], "actions": ["Admin", "Read", "List", "Write", "Tagging"]}]}))
        run(["docker", "run", "--detach", "--rm", "--name", name, "--label", "infrapro.architecture.fixture="+image_tag,
             "--memory", "384m", "--cpus", "1", "-p", "127.0.0.1::8333", "-v", str(config)+":/etc/arc/s3.json:ro",
             IMAGE, "server", "-ip=127.0.0.1", "-ip.bind=0.0.0.0", "-dir=/data", "-s3", "-s3.iam.readOnly=false",
             "-s3.config=/etc/arc/s3.json", "-volume.max=200", "-master.volumeSizeLimitMB=8"])
        try:
            address = run(["docker", "port", name, "8333/tcp"]).decode().strip()
            if not address.startswith("127.0.0.1:"):
                raise ValueError("Fixture requires a loopback port")
            origin = "http://" + address
            reply = folder / "response"

            def signed(credential, service, method, path, body=None, form=False):
                args = ["curl", "--disable", "--silent", "--show-error", "--max-time", "25", "--config", "-",
                        "--aws-sigv4", "aws:amz:us-east-1:"+service, "--request", method, "--output", str(reply),
                        "--write-out", "%{http_code}", origin + path]
                if body is not None:
                    payload = folder / "request"
                    payload.write_bytes(body)
                    args += ["-H", "Content-Type: "+("application/x-www-form-urlencoded" if form else "application/octet-stream"),
                             "--data-binary", "@"+str(payload)]
                credentials = ('user = "' + credential[0]+":"+credential[1]+'"\n').encode()
                status = int(run(args, input=credentials).decode())
                return status, reply.read_bytes()

            def iam(action, **fields):
                body = urllib.parse.urlencode(dict(Action=action, Version="2010-05-08", **fields)).encode()
                status, data = signed(admin, "iam", "POST", "/", body, form=True)
                xml = ET.fromstring(data)
                if status != 200:
                    code = xml.findtext(".//{*}Code", default="unknown")
                    raise RuntimeError("IAM "+action+" "+str(status)+" "+code)
                return xml

            for attempt in range(90):
                try:
                    if signed(admin, "s3", "GET", "/")[0] == 200:
                        break
                except subprocess.CalledProcessError:
                    pass
                if attempt == 89:
                    raise RuntimeError("Isolated object fixture did not start")
                time.sleep(.25)
            for bucket in buckets.values():
                status, _ = signed(admin, "s3", "PUT", "/"+bucket)
                assert status in (200, 201), status

            def provision(role):
                policy = (layout / "roles" / role / "objects.iam.json").read_text()
                for kind, bucket in buckets.items():
                    policy = policy.replace("REPLACE_"+kind.upper()+"_BUCKET", bucket)
                assert "REPLACE_" not in policy
                iam("CreateUser", UserName=role)
                iam("PutUserPolicy", UserName=role, PolicyName="infrapro-"+role, PolicyDocument=policy)
                key = iam("CreateAccessKey", UserName=role)
                return key.findtext(".//{*}AccessKeyId"), key.findtext(".//{*}SecretAccessKey")

            content = b"sealed architecture policy fixture"
            def verify(role, creds, method, path, expected, data=None):
                status, body = signed(creds, "s3", method, path, data)
                events.append({"role":role,"method":method,"scope":path,"status":status,"expected":expected})
                assert status == expected, (role, method, path, status, expected)
                return body
            source = "/"+buckets["source"]+"/"
            parsing = "/"+buckets["parsing"]+"/"
            for path in [source+"display/sealed/ok", source+"display/uploads/other", parsing+"ingestion/v1/raw", parsing+"parsing/v1/sample"]:
                if signed(admin,"s3","PUT",path,content)[0] != 200:
                    raise RuntimeError("Failed to seed private objects")
            for role in ["display-assets","control-api","transfer-api","monitoring-query-api","ingest-custody-worker","parsing-dispatch-worker"]:
                creds=provision(role)
                before=len(events)
                if role=="display-assets":
                    assert verify(role,creds,"GET",source+"display/sealed/ok",200)==content
                    verify(role,creds,"GET",source+"display/uploads/other",403)
                    verify(role,creds,"PUT",source+"display/sealed/write",403,content)
                    verify(role,creds,"GET",source+"?list-type=2&prefix=display%2Fsealed%2F",200)
                    verify(role,creds,"GET",source+"?list-type=2&prefix=display%2Fuploads%2F",403)
                elif role in ("control-api","transfer-api"):
                    assert verify(role,creds,"GET",parsing+"ingestion/v1/raw",200)==content
                    verify(role,creds,"PUT",parsing+"ingestion/v1/new",403,content)
                    verify(role,creds,"DELETE",parsing+"ingestion/v1/raw",403)
                    verify(role,creds,"PUT",parsing+"parsing/v1/new",200 if role=="transfer-api" else 403,content)
                elif role=="monitoring-query-api":
                    verify(role,creds,"GET",source+"?list-type=2&prefix=00000000-0000-0000-0000-000000000000%2Fsources%2F",200)
                    verify(role,creds,"PUT",source+"00000000-0000-0000-0000-000000000000/sources/new",403,content)
                    verify(role,creds,"GET",parsing+"ingestion/v1/raw",403)
                else:
                    target=parsing+"ingestion/v1/"+role
                    verify(role,creds,"PUT",target,200,content)
                    assert verify(role,creds,"GET",target,200)==content
                    verify(role,creds,"DELETE",target,204 if role=="ingest-custody-worker" else 403)
                    verify(role,creds,"GET",source+"display/sealed/ok",403)
                assert len(events)>before
            output.write_text(json.dumps({"status":"passed","image":IMAGE,"content_sha256":hashlib.sha256(content).hexdigest(),"checks":events},indent=2)+"\n")
        finally:
            run(["docker","stop",name])


if __name__=="__main__":
    flags=argparse.ArgumentParser()
    flags.add_argument("--layout",required=True,type=pathlib.Path)
    flags.add_argument("--output",required=True,type=pathlib.Path)
    options=flags.parse_args()
    os.umask(0o077)
    options.output.parent.mkdir(parents=True,exist_ok=True)
    check(options.layout,options.output)
