"""TCP pass-through for co-located or privately addressed native replica slots."""
import asyncio
import contextlib
import itertools
import json
import os
import pathlib
import signal
import sys
import tempfile


async def pipe(reader, writer):
    while data := await reader.read(65536):
        writer.write(data)
        await writer.drain()
    if writer.can_write_eof():
        writer.write_eof()


async def main(config):
    servers, active = [], set()
    stopped = asyncio.Event()
    pid_path = os.environ.get("INFRAPRO_DIAGNOSTICS_REPORT_FILE")
    report = pathlib.Path(pid_path) if pid_path else None
    our_pid = str(os.getpid()) + "\n"
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, stopped.set)

    def handler(peers, limit):
        rotate = itertools.cycle(range(len(peers)))
        occupied = set()

        async def connect(reader, writer):
            if len(occupied) >= limit:
                writer.close()
                return
            task = asyncio.current_task()
            occupied.add(task)
            active.add(task)
            upstream = None
            try:
                first = next(rotate)
                for offset in range(len(peers)):
                    target = peers[(first + offset) % len(peers)]
                    try:
                        backend, upstream = await asyncio.wait_for(
                            asyncio.open_connection(target["host"], target["port"]), 2)
                        break
                    except (OSError, asyncio.TimeoutError):
                        continue
                if upstream is not None:
                    # Choose the backend before forwarding any bytes. A partial
                    # request is never replayed to another slot.
                    await asyncio.gather(pipe(reader, upstream), pipe(backend, writer))
            except (ConnectionError, OSError):
                pass
            finally:
                for stream in (upstream, writer):
                    if stream is not None:
                        stream.close()
                        with contextlib.suppress(ConnectionError, OSError):
                            await stream.wait_closed()
                active.discard(task)
                occupied.discard(task)
        return connect

    try:
        for entry in config:
            addr = entry["listen"]
            servers.append(await asyncio.start_server(handler(entry["backends"], entry.get("max_connections", 128)), addr["host"], addr["port"]))
        if report:
            report.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
            with tempfile.NamedTemporaryFile(mode="w", dir=report.parent, delete=False) as tmp:
                tmp.write(our_pid)
            try:
                os.replace(tmp.name, report)
            finally:
                if os.path.exists(tmp.name):
                    os.unlink(tmp.name)
        await stopped.wait()
    finally:
        for server in servers:
            server.close()
        await asyncio.gather(*(server.wait_closed() for server in servers))
        if active:
            await asyncio.wait(tuple(active), timeout=75)
        if report and report.is_file() and not report.is_symlink():
            with report.open(encoding="ascii") as current:
                if current.read(32) == our_pid:
                    report.unlink(missing_ok=True)


if __name__ == "__main__":
    with open(sys.argv[1], encoding="utf-8") as file:
        asyncio.run(main(json.load(file)))
