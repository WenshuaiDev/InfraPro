"""Private stdin carries synthetic test inputs; no terminal transcript is saved."""
import json
import os
import pty
import re
import select
import subprocess
import sys
import time

def exercise(case):
    master, slave = pty.openpty()
    process = subprocess.Popen(case['args'], stdin=slave, stdout=slave, stderr=slave, close_fds=True, env={**os.environ, **case.get('env', {})})
    os.close(slave)
    transcript, index = '', 0
    deadline = time.monotonic() + 45
    try:
        while time.monotonic() < deadline:
            if select.select([master], [], [], .1)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                transcript += chunk.decode('utf-8', 'replace')
                if index < len(case.get('inputs', [])) and case['inputs'][index]['prompt'] in transcript:
                    os.write(master, (case['inputs'][index]['value'] + '\n').encode())
                    index += 1
            if process.poll() is not None:
                break
        if process.poll() is None:
            process.terminate()
        process.wait(timeout=5)
    finally:
        os.close(master)
    if (process.returncode == 0) != case['success']:
        raise RuntimeError('CLI outcome mismatch (transcript withheld)')
    for value in case.get('secrets', []):
        if value in transcript:
            raise RuntimeError('CLI echoed an input credential')
    if case.get('maintenance') and not re.search(r'/mfa-recovery#token=[A-Za-z0-9_-]{40,}', transcript):
        raise RuntimeError('Maintenance CLI did not return a controlled recovery grant')
    if 'session' in case:
        path = case['session']
        if os.path.exists(path) != case['success']:
            raise RuntimeError('CLI wrote a false successful session file')
        if case['success']:
            if os.stat(path).st_mode & 0o077:
                raise RuntimeError('Session file permissions')
            with open(path) as stream:
                saved = json.load(stream)
            cookies = saved['cookies']
            names = {cookie['Name'] for cookie in cookies}
            if 'infrapro_session' not in names or names - {'infrapro_session', 'infrapro_csrf'}:
                raise RuntimeError('Pending challenge persisted as a session')
            os.remove(path)

try:
    for item in json.load(sys.stdin):
        exercise(item)
except Exception as failure:
    print(type(failure).__name__ + ': ' + str(failure), file=sys.stderr)
    sys.exit(1)
print('Interactive CLI MFA and controlled recovery checks passed.')
