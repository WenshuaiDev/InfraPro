"""Streaming TLS/SigV4 backup with checksum verification and create-only recovery."""
import datetime
import hashlib
import hmac
import http.client
import json
import os
import pathlib
import ssl
import tempfile
import urllib.parse
import xml.etree.ElementTree as ET


def file_hash(path):
    digest = hashlib.sha256()
    with pathlib.Path(path).open("rb") as stream:
        while chunk := stream.read(1 << 20):
            digest.update(chunk)
    return digest.hexdigest()


class S3:
    def __init__(self, endpoint, key, secret, ca):
        self.url = urllib.parse.urlsplit(endpoint)
        if self.url.scheme != "https" or not self.url.hostname or self.url.username or self.url.path not in ("", "/"):
            raise ValueError("Expected an HTTPS S3 endpoint")
        self.key = key
        self.secret = secret
        self.ctx = ssl.create_default_context(cafile=str(ca))

    def request(self, method, key="", query=None, file=None, destination=None, create_only=False):
        path = "/" + urllib.parse.quote(key, safe="/~")
        query = urllib.parse.urlencode(sorted((query or {}).items()), quote_via=urllib.parse.quote, safe="~")
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        date = stamp[:8]
        size = pathlib.Path(file).stat().st_size if file else 0
        digest = file_hash(file) if file else hashlib.sha256(b"").hexdigest()
        headers = {"host": self.url.netloc, "x-amz-content-sha256": digest, "x-amz-date": stamp}
        if create_only:
            headers["if-none-match"] = "*"
        names = ";".join(sorted(headers))
        canonical = "\n".join([method, path, query, "".join(k+":"+headers[k]+"\n" for k in sorted(headers)), names, digest])
        scope = date + "/us-east-1/s3/aws4_request"
        signing = ("AWS4"+self.secret).encode()
        for field in (date, "us-east-1", "s3", "aws4_request"):
            signing = hmac.new(signing, field.encode(), hashlib.sha256).digest()
        message = "\n".join(("AWS4-HMAC-SHA256", stamp, scope, hashlib.sha256(canonical.encode()).hexdigest()))
        signature = hmac.new(signing, message.encode(), hashlib.sha256).hexdigest()
        headers["Authorization"] = "AWS4-HMAC-SHA256 Credential="+self.key+"/"+scope+", SignedHeaders="+names+", Signature="+signature
        headers["Content-Length"] = str(size)
        client = http.client.HTTPSConnection(self.url.hostname, self.url.port, context=self.ctx, timeout=180)
        try:
            client.putrequest(method, path+("?"+query if query else ""), skip_host=True)
            for name, value in headers.items():
                client.putheader(name, value)
            client.endheaders()
            if file:
                with open(file, "rb") as stream:
                    while chunk := stream.read(1 << 20):
                        client.send(chunk)
            response = client.getresponse()
            if not 200 <= response.status < 300:
                raise RuntimeError("S3 "+method+" HTTP "+str(response.status))
            if destination is None:
                return response.read()
            measured = hashlib.sha256()
            length = 0
            with open(destination, "wb") as out:
                while chunk := response.read(1 << 20):
                    out.write(chunk)
                    measured.update(chunk)
                    length += len(chunk)
            return {"size": length, "sha256": measured.hexdigest()}
        finally:
            client.close()

    def list(self, bucket):
        token = ""
        ns = {"s": "http://s3.amazonaws.com/doc/2006-03-01/"}
        while True:
            query = {"list-type": "2", "max-keys": "1000"}
            if token:
                query["continuation-token"] = token
            page = ET.fromstring(self.request("GET", bucket, query))
            for row in page.findall("s:Contents", ns):
                yield row.findtext("s:Key", namespaces=ns), int(row.findtext("s:Size", namespaces=ns))
            token = page.findtext("s:NextContinuationToken", default="", namespaces=ns)
            if not token:
                break

    def archive(self, bucket, directory):
        directory = pathlib.Path(directory)
        directory.mkdir(parents=True)
        entries = []
        for index, (key, size) in enumerate(self.list(bucket)):
            path = directory / f"{index:08d}.object"
            measured = self.request("GET", bucket+"/"+key, destination=path)
            if measured["size"] != size:
                raise RuntimeError("S3 listing size mismatch")
            entries.append({"bucket": bucket, "key": key, "path": path.name, **measured})
        (directory/"manifest.json").write_text(json.dumps(entries, ensure_ascii=False, indent=2))
        return len(entries)

    def restore(self, bucket, directory):
        directory = pathlib.Path(directory)
        entries = json.loads((directory/"manifest.json").read_text())
        expected = {item["key"]: item for item in entries}
        if len(expected) != len(entries):
            raise ValueError("Duplicate keys in archive")
        current = dict(self.list(bucket))
        if current.keys() - expected.keys():
            raise ValueError("Restore target contains unrelated objects")
        # Preflight all local files before creating any destination key.
        for entry in entries:
            path = directory/entry["path"]
            if path.resolve().parent != directory.resolve() or path.is_symlink() or not path.is_file():
                raise ValueError("Unexpected archive path")
            if path.stat().st_size != entry["size"] or file_hash(path) != entry["sha256"]:
                raise ValueError("Archive checksum mismatch")
        for key, entry in expected.items():
            target = bucket+"/"+key
            if key not in current:
                self.request("PUT", target, file=directory/entry["path"], create_only=True)
            # Verify both pre-existing files written by domain restore and new files.
            actual = self.request("GET", target, destination=os.devnull)
            if actual != {name:entry[name] for name in ("size","sha256")}:
                raise ValueError("Restore object content mismatch")
        if dict(self.list(bucket)) != {key: v["size"] for key,v in expected.items()}:
            raise ValueError("Restore object inventory changed")
        return len(entries)
