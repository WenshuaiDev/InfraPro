// Read only the operating system counters of processes started by the load
// driver. Darwin ps does not expose the disk byte totals for a live process.
#include <stdio.h>
#include <stdlib.h>
#ifdef __APPLE__
#include <libproc.h>
#include <sys/resource.h>
#endif

int main(int argc, char *argv[]) {
  for (int i = 1; i < argc; i++) {
    char *end = NULL;
    long pid = strtol(argv[i], &end, 10);
    if (pid <= 0 || !end || *end != '\0') return 2;
#ifdef __APPLE__
    struct rusage_info_v4 usage = {0};
    if (proc_pid_rusage((int)pid, RUSAGE_INFO_V4, (rusage_info_t *)&usage) != 0) return 1;
    printf("%ld %llu %llu\n", pid, (unsigned long long)usage.ri_diskio_bytesread,
           (unsigned long long)usage.ri_diskio_byteswritten);
#else
    char buf[256], file[64];
    unsigned long long input = 0, output = 0;
    snprintf(file, sizeof(file), "/proc/%ld/io", pid);
    FILE *fp = fopen(file, "r");
    if (!fp) return 1;
    while (fgets(buf, sizeof(buf), fp)) {
      if (sscanf(buf, "read_bytes: %llu", &input) == 1) continue;
      if (sscanf(buf, "write_bytes: %llu", &output) == 1) continue;
    }
    fclose(fp);
    printf("%ld %llu %llu\n", pid, input, output);
#endif
  }
  return 0;
}
