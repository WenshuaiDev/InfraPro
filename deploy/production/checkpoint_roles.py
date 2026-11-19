"""Resolve running writers from the release inventory and retain their pre-freeze state."""

import datetime
import hashlib
import json
import os
import pathlib
import re
import subprocess
import urllib.request

SAFE_ID = re.compile(r"checkpoint-\d{8}T\d{6}Z")
NAME = re.compile(r"[a-z][a-z0-9-]{0,62}")


def utc():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    tmp = path.with_suffix(".tmp")
    with tmp.open("w") as out:
        json.dump(value, out, indent=2)
        out.flush()
        os.fsync(out.fileno())
    os.replace(tmp, path)


def load_roles(base):
    release = json.loads((base / "config/release.json").read_text())["release"]
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9._-]*", release):
        raise ValueError("Invalid release ID")
    paths = [base / "releases" / release / "deploy/architecture/roles.json",
             base / "config/architecture/roles.json",
             pathlib.Path(__file__).resolve().parents[1] / "architecture/roles.json"]
    path = next((p for p in paths if p.is_file()), None)
    if path is None:
        raise ValueError("Missing role inventory for the installed release")
    content = path.read_bytes()
    spec = json.loads(content)
    if spec["version"] != 1 or not all(NAME.fullmatch(n) for n in spec["roles"]):
        raise ValueError("Invalid role inventory")
    return spec, hashlib.sha256(content).hexdigest()


def run(args):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          text=True, timeout=240).stdout.strip()


class Checkpoint:
    def __init__(self, base, ident, runner=run):
        if not SAFE_ID.fullmatch(ident):
            raise ValueError("Invalid checkpoint ID")
        self.base = base
        self.spec, self.digest = load_roles(base)
        self.file = base / "data/backups/.freeze" / (ident + ".json")
        self.run = runner
        settings_file = base / "config/architecture/checkpoint.json"
        self.settings = json.loads(settings_file.read_text()) if settings_file.is_file() else {}
        layout_path = base / "config/architecture/layout.json"
        self.instances = json.loads(layout_path.read_text()).get("instances", {}) if layout_path.is_file() else {}
        self.instance = self.settings.get("instance", "infrapro-architecture")
        if not NAME.fullmatch(self.instance):
            raise ValueError("Invalid deployment instance")

    def discover(self):
        eligible = {role for role, item in self.spec["roles"].items() if item["phase"] in self.spec["freezeOrder"]}
        # Unit-file and loaded-unit discovery include transient units and stopped static replicas.
        units = set()
        native = self.settings.get("systemd", True)
        if not native and not self.settings.get("docker",False):
            raise ValueError("No checkpoint discovery driver enabled")
        for verb in (["list-unit-files", "list-units"] if native else []):
            output = self.run(["systemctl", verb, "--all", "--no-legend", "--plain", "--no-pager", "infrapro-*.service"])
            for line in output.splitlines():
                fields = line.strip().lstrip("●").split()
                if fields:
                    units.add(fields[0])
        binding = {}
        slot_roles = {}
        for role, slots in self.instances.items():
            if role not in eligible:
                raise ValueError("Unknown checkpoint replica role")
            for slot in slots:
                replica, label = slot["id"], slot["label"]
                if not NAME.fullmatch(replica) or not NAME.fullmatch(label) or replica in slot_roles:
                    raise ValueError("Invalid checkpoint replica declaration")
                slot_roles[replica] = role
                name = "infrapro-" + label + ".service"
                if name in units:
                    binding[name] = {"role": role, "driver": "systemd", "replica": replica}
        if "infrapro-replica-proxy.service" in units:
            binding["infrapro-replica-proxy.service"] = {"role": "replica-proxy" if "replica-proxy" in eligible else "api", "driver": "systemd"}
        for role in eligible:
            for unit in units:
                # systemd templates may run numbered replicas. Do not attempt to stop templates.
                if re.fullmatch(r"infrapro-" + re.escape(role) + r"(@[a-zA-Z0-9_-]+)?\.service", unit):
                    binding[unit] = {"role": role, "driver": "systemd"}
        for unit, role in self.settings.get("units", {}).items():
            if role not in eligible or not re.fullmatch(r"infrapro-[a-z0-9@_-]+\.service", unit) or unit not in units:
                raise ValueError("Invalid configured checkpoint unit")
            binding[unit] = {"role": role, "driver": "systemd"}
        if self.settings.get("docker", False):
            output = self.run(["docker", "ps", "--all", "--filter", "label=io.infrapro.instance="+self.instance,
                               "--format", '{{.ID}} {{.Label "io.infrapro.replica"}}'])
            for line in output.splitlines():
                container, replica = line.split()
                if not re.fullmatch("[a-f0-9]{12,64}", container):
                    raise ValueError("Invalid container identity")
                role = slot_roles.get(replica, re.sub(r"-\d+$", "", replica))
                if role in eligible:
                    binding[container] = {"role": role, "driver": "docker", "replica":replica}
                elif role not in self.settings.get("infrastructure", []):
                    raise ValueError("Unclassified container in checkpoint deployment: "+replica)
        for name, entry in binding.items():
            entry["phase"] = self.spec["roles"][entry["role"]]["phase"]
            entry["initial"] = self.status(name, entry)
        return binding

    def status(self, name, unit):
        if unit["driver"] == "docker":
            status = self.run(["docker", "inspect", "--format", "{{.State.Status}}", name])
            return {"running": "active", "exited": "inactive", "created": "inactive"}.get(status, status)
        return self.run(["systemctl", "show", "--property=ActiveState", "--value", name])

    def set_running(self, name, entry, desired):
        current = self.status(name, entry)
        if (desired and current == "active") or (not desired and current in ("inactive", "failed")):
            return
        action = "start" if desired else "stop"
        if entry["driver"] == "docker":
            args = ["docker", action]
            if action == "stop":
                args += ["--time", str(self.spec["roles"][entry["role"]]["stopSeconds"])]
            args += [name]
        else:
            args = ([] if os.geteuid() == 0 else ["/usr/bin/sudo", "-n"]) + ["/usr/bin/systemctl", action, name]
        self.run(args)
        if self.status(name, entry) != ("active" if desired else "inactive"):
            raise RuntimeError("Service failed to settle: "+name)

    def prepare(self):
        if self.file.is_file():
            state = json.loads(self.file.read_text())
            if state["inventory_sha256"] != self.digest:
                raise ValueError("Inventory changed during checkpoint")
            return state
        entries = self.discover()
        for name, item in entries.items():
            if item["initial"] not in ("active", "inactive", "failed"):
                raise RuntimeError("Service transition in progress: "+name)
        state = {"inventory_sha256": self.digest, "prepared_at": utc(), "units": entries, "phases": {}}
        write_json(self.file, state)
        return state

    def monitor(self, name, unit):
        # Only local JetStream statistics, never broker credentials or message bodies.
        port = self.settings.get("jetstream_monitor_port", 8222)
        if type(port) is not int or not (1 <= port <= 65535):
            raise ValueError("Invalid broker monitor port")
        endpoint = "http://127.0.0.1:"+str(port)+"/jsz?accounts=true&streams=true&consumers=true&config=false"
        if unit["driver"] == "docker":
            data = self.run(["docker","exec",name,"wget","-qO-",endpoint])
        else:
            with urllib.request.urlopen(endpoint, timeout=4) as response:
                data = response.read(8 * 1024 * 1024 + 1)
        if len(data) > 8 * 1024 * 1024:
            raise ValueError("Broker status exceeds bound")
        info = json.loads(data)
        if "server_id" not in info:
            raise ValueError("Unrecognized JetStream monitor")
        return info

    def change(self, phase, desired):
        if phase not in self.spec["freezeOrder"] or not self.file.is_file():
            raise ValueError("Unprepared checkpoint or unknown phase")
        state = self.prepare()
        units = [(n, v) for n, v in state["units"].items() if v["phase"] == phase and v["initial"] == "active"]
        if phase == "broker" and not desired and units and "jetstream" not in state:
            state["jetstream"] = {name:self.monitor(name,entry) for name,entry in units}
            write_json(self.file, state)
        for name, entry in units:
            self.set_running(name, entry, desired)
        state["phases"][phase] = {"action": "started" if desired else "stopped", "at": utc(), "units":[name for name, _ in units]}
        write_json(self.file, state)
        return state["phases"][phase]

    def frozen(self, allow_brokers=False):
        if not self.file.is_file():
            raise ValueError("Checkpoint not prepared")
        state = self.prepare()
        if set(state["phases"]) != set(self.spec["freezeOrder"]) or any(p["action"] != "stopped" for n,p in state["phases"].items() if not (allow_brokers and n=="broker")):
            raise RuntimeError("Quiescence not established for this checkpoint")
        for name, entry in self.discover().items():
            if allow_brokers and entry["role"]=="nats-server":
                continue
            if entry["initial"] not in ("inactive", "failed"):
                raise RuntimeError("Writer active after freeze: "+name)
        return state

    def add_node_data(self, tar):
        state = self.frozen()
        tar.add(self.file, arcname="writer-checkpoint.json")
        for name, entry in state["units"].items():
            if entry["role"] != "nats-server":
                continue
            replica = entry.get("replica", name)
            directory = self.settings.get("jetstream_store_dirs", {}).get(replica,self.settings.get("jetstream_store_dir"))
            if not directory or not pathlib.Path(directory).is_absolute() or not pathlib.Path(directory).is_dir():
                raise ValueError("Missing local JetStream store directory")
            tar.add(directory, arcname="jetstream-store/"+replica)
