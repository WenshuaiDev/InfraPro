"""Snapshot an owned Darwin/Linux process tree, including child parser runners."""
import ctypes
import json
import os
import subprocess
import sys

roots = {int(value) for value in sys.argv[1:]}
items = {}
for row in subprocess.check_output(["ps", "-axo", "pid=,ppid=,%cpu=,rss=,time=,comm="], text=True).splitlines():
    cols = row.strip().split(None, 5)
    if len(cols) != 6:
        continue
    pid, parent, cpu, rss, cpu_time, name = cols
    items[int(pid)] = dict(parent=int(parent), cpu_percent=float(cpu), rss_kib=int(rss), cpu_time=cpu_time, executable=os.path.basename(name))
owned = roots.copy()
while True:
    more = {p for p, info in items.items() if info["parent"] in owned}
    if more <= owned:
        break
    owned.update(more)
if sys.platform == "darwin":
    libproc = ctypes.CDLL("/usr/lib/libproc.dylib")
    # libproc CPU totals are measured in mach absolute ticks on Apple silicon.
    class Timebase(ctypes.Structure):
        _fields_ = [("numer", ctypes.c_uint32), ("denom", ctypes.c_uint32)]
    conversion = Timebase()
    ctypes.CDLL(None).mach_timebase_info(ctypes.byref(conversion))
    def nanoseconds(ticks):
        return ticks * conversion.numer // conversion.denom
    class Usage(ctypes.Structure):
        _fields_ = [("uuid", ctypes.c_ubyte * 16)] + [(key, ctypes.c_uint64) for key in
            "user system wakeups intr pageins wired resident physical start end child_user child_system child_wakeups child_intr child_pageins child_elapsed disk_read disk_written".split()]
    for pid in owned & items.keys():
        usage = Usage()
        if libproc.proc_pid_rusage(pid, 2, ctypes.byref(usage)) == 0:
            items[pid].update(cpu_ns=nanoseconds(usage.user + usage.system), completed_child_cpu_ns=nanoseconds(usage.child_user + usage.child_system), disk_read_bytes=usage.disk_read, disk_written_bytes=usage.disk_written)
        else:
            # Short-lived parser runners are charged to the parent after wait.
            del items[pid]
elif sys.platform.startswith("linux"):
    hz = os.sysconf("SC_CLK_TCK")
    for pid in owned & items.keys():
        try:
            with open(f"/proc/{pid}/stat", encoding="utf8") as file:
                fields = file.read().rsplit(") ", 1)[1].split()
            with open(f"/proc/{pid}/io", encoding="utf8") as file:
                io = dict(line.split(": ", 1) for line in file)
            items[pid].update(cpu_ns=(int(fields[11]) + int(fields[12])) * 10**9 // hz,
                              completed_child_cpu_ns=(int(fields[13]) + int(fields[14])) * 10**9 // hz,
                              disk_read_bytes=int(io["read_bytes"]), disk_written_bytes=int(io["write_bytes"]))
        except (FileNotFoundError, ProcessLookupError):
            # A parser runner can exit between ps and /proc. Its reaped CPU
            # is included in the long-lived parser's subsequent totals.
            del items[pid]
print(json.dumps({pid: items[pid] for pid in sorted(owned & items.keys())}))
