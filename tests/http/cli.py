"""Exercise the real no-echo maintenance CLI using a pseudo-terminal, only in the dedicated HTTP test stack."""
import json
import os
import pty
import re
import select
import subprocess
import sys
import time

BASE = ['docker', 'compose', '-f', 'deploy/compose/http-test.yaml', 'run', '--rm', '--no-deps', 'ops', 'account']
PASSWORD = 'Real browser 河流 secret 592'

def run(args, prompts=()):
    master, slave = pty.openpty()
    proc = subprocess.Popen(BASE + args, stdin=slave, stdout=slave, stderr=slave, close_fds=True)
    os.close(slave)
    transcript = ''
    index = 0
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if select.select([master], [], [], 0.1)[0]:
            try:
                data = os.read(master, 65536)
            except OSError:
                break
            if not data:
                break
            transcript += data.decode('utf-8', 'replace')
            if index < len(prompts) and prompts[index][0] in transcript:
                os.write(master, (prompts[index][1] + '\n').encode())
                index += 1
        if proc.poll() is not None:
            break
    if proc.poll() is None:
        proc.terminate()
    proc.wait(timeout=5)
    os.close(master)
    if PASSWORD in transcript:
        raise RuntimeError('Password echoed by maintenance terminal')
    if proc.returncode:
        # No credential-bearing output is printed on failure.
        raise RuntimeError('Maintenance CLI failed with exit code ' + str(proc.returncode))
    return transcript

if sys.argv[1] == 'bootstrap':
    result = run(['bootstrap'], [('用户名：', 'e2e_owner'), ('显示名称', 'HTTP 验收'), ('密码（8', PASSWORD), ('再次输入密码', PASSWORD)])
    if '首个账号创建成功' not in result:
        raise RuntimeError('Bootstrap success not observed')
    print('Interactive bootstrap passed; password was not echoed.')
elif sys.argv[1] == 'reset':
    result = run(['reset-link', '--username', sys.argv[2], '--reason', 'HTTP 验收已核验本人身份'])
    match = re.search(r'http://[^\s"\x1b]+#token=[A-Za-z0-9_-]+', result)
    if not match:
        raise RuntimeError('Reset link not found')
    # Caller captures this synthetic one-time test credential in memory, never in logs.
    print(json.dumps({'url': match.group(0)}))
