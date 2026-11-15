// Lists may contain many fixed versions. Keep optional row metadata reads below
// the account authorization lock budget; canvas and user actions stay immediate.
// Every request still carries its own scope and TanStack Query abort signal.
const limit = 2;
let active = 0;
const pending: Array<() => void> = [];
function drain() {
  while (active < limit && pending.length) pending.shift()!();
}
export function readSpatialMetadata<T>(
  signal: AbortSignal,
  read: () => Promise<T>,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const cancel = () => {
      const index = pending.indexOf(start);
      if (index >= 0) pending.splice(index, 1);
      signal.removeEventListener("abort", cancel);
      reject(
        signal.reason ??
          new DOMException("Canceled metadata read", "AbortError"),
      );
    };
    const start = () => {
      signal.removeEventListener("abort", cancel);
      if (signal.aborted) {
        cancel();
        return;
      }
      active++;
      Promise.resolve()
        .then(read)
        .then(resolve, reject)
        .finally(() => {
          active--;
          drain();
        });
    };
    if (signal.aborted) {
      cancel();
      return;
    }
    signal.addEventListener("abort", cancel, { once: true });
    pending.push(start);
    drain();
  });
}
