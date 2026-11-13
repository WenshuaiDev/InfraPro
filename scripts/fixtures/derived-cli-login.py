"""Drive only the disposable CLI fixture's actual TTY password prompt."""
import json
import os
import pty
import subprocess
import sys
import termios
import time

binary, origin, username, session = sys.argv[1:]
password = os.environ.pop("INFRAPRO_CLI_TEST_PASSWORD")
master, slave = pty.openpty()
child = subprocess.Popen(
    [binary, "monitoring", "derived", "login", "--url", origin,
     "--username", username, "--session-file", session],
    stdin=slave, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
)
try:
    deadline = time.monotonic() + 10
    # Send only after ReadPassword has disabled terminal echo. Never log the
    # password or session cookies, including in subprocess error diagnostics.
    while termios.tcgetattr(slave)[3] & termios.ECHO:
        if child.poll() is not None or time.monotonic() > deadline:
            raise RuntimeError("CLI did not enter its secure password prompt")
        time.sleep(0.01)
    os.write(master, password.encode() + b"\n")
    stdout, stderr = child.communicate(timeout=40)
    if child.returncode:
        raise RuntimeError("CLI fixture login failed: " + stderr.decode())
    result = json.loads(stdout)
    if result.get("state") != "authenticated":
        raise RuntimeError("CLI login did not authenticate")
    print(json.dumps({"state": result["state"], "mode": oct(os.stat(session).st_mode & 0o777)}))
finally:
    if child.poll() is None:
        child.kill()
        child.wait()
    os.close(master)
    os.close(slave)
