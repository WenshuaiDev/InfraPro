import { DisplayError } from "./error.js";
import { PROTOCOL_MAJOR } from "./types.js";
import type {
  CallOptions,
  DisplayTransport,
  RuntimeEvent,
  SubscriptionEvent,
} from "./types.js";
interface Response {
  kind: "response";
  id: string;
  result?: unknown;
  error?: unknown;
}
export async function connectBridge(
  signal?: AbortSignal,
): Promise<DisplayTransport> {
  if (typeof window === "undefined" || window.parent === window)
    throw new DisplayError(
      "runtime_required",
      "请在可信运行壳或本地开发壳中运行。",
    );
  if (signal?.aborted) throw new DisplayError("aborted");
  const port = await new Promise<MessagePort>((resolve, reject) => {
    const cleanup = () => {
      window.removeEventListener("message", handshake);
      clearTimeout(timeout);
      clearInterval(retry);
      signal?.removeEventListener("abort", abort);
    };
    const abort = () => {
      cleanup();
      reject(new DisplayError("aborted"));
    };
    const handshake = (event: MessageEvent<unknown>) => {
      const data = event.data;
      if (
        event.source !== window.parent ||
        !data ||
        typeof data !== "object" ||
        !("kind" in data) ||
        data.kind !== "infrapro.display.init"
      )
        return;
      if (!("protocol" in data) || data.protocol !== PROTOCOL_MAJOR) {
        cleanup();
        reject(new DisplayError("version_incompatible"));
        return;
      }
      if (
        !("nonce" in data) ||
        typeof data.nonce !== "string" ||
        data.nonce.length < 32 ||
        !event.ports[0]
      )
        return;
      cleanup();
      event.ports[0].postMessage({
        kind: "handshake",
        nonce: data.nonce,
        protocol: PROTOCOL_MAJOR,
      });
      resolve(event.ports[0]);
    };
    const hello = () =>
      window.parent.postMessage(
        { kind: "infrapro.display.hello", protocol: PROTOCOL_MAJOR },
        "*",
      );
    const timeout = setTimeout(() => {
      cleanup();
      reject(new DisplayError("handshake_timeout"));
    }, 10_000);
    const retry = setInterval(hello, 300);
    window.addEventListener("message", handshake);
    signal?.addEventListener("abort", abort, { once: true });
    hello();
  });
  return portTransport(port);
}
export function portTransport(port: MessagePort): DisplayTransport {
  let disposed = false,
    next = 0;
  const listeners = new Set<
    (event: SubscriptionEvent | RuntimeEvent) => void
  >();
  const pending = new Map<
    string,
    {
      resolve(value: unknown): void;
      reject(error: unknown): void;
      cleanup(): void;
    }
  >();
  port.onmessage = (
    event: MessageEvent<
      Response | { kind: "event"; event: SubscriptionEvent | RuntimeEvent }
    >,
  ) => {
    const msg = event.data;
    if (!msg || disposed) return;
    if (msg.kind === "event") {
      for (const listener of listeners) listener(msg.event);
      return;
    }
    if (msg.kind !== "response" || typeof msg.id !== "string") return;
    const task = pending.get(msg.id);
    if (!task) return;
    pending.delete(msg.id);
    task.cleanup();
    if (msg.error) task.reject(DisplayError.from(msg.error));
    else task.resolve(msg.result);
  };
  port.start();
  return {
    request<T>(
      method: string,
      params: unknown,
      options: CallOptions = {},
    ): Promise<T> {
      if (disposed) return Promise.reject(new DisplayError("disposed"));
      if (options.signal?.aborted)
        return Promise.reject(new DisplayError("aborted"));
      if (pending.size >= 32)
        return Promise.reject(new DisplayError("budget_exceeded"));
      if (JSON.stringify(params).length > 256 * 1024)
        return Promise.reject(new DisplayError("budget_exceeded"));
      return new Promise<T>((resolve, reject) => {
        const id = String(++next);
        const abort = () => {
          const task = pending.get(id);
          if (!task) return;
          pending.delete(id);
          task.cleanup();
          port.postMessage({ kind: "cancel", id });
          reject(new DisplayError("aborted"));
        };
        const timeout = setTimeout(() => {
          abort();
        }, 20_000);
        const cleanup = () => {
          clearTimeout(timeout);
          options.signal?.removeEventListener("abort", abort);
        };
        pending.set(id, {
          resolve: (value) => resolve(value as T),
          reject,
          cleanup,
        });
        options.signal?.addEventListener("abort", abort, { once: true });
        port.postMessage({
          kind: "request",
          protocol: PROTOCOL_MAJOR,
          id,
          method,
          params,
          generation: options.generation ?? "1",
        });
      });
    },
    onEvent(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      for (const task of pending.values()) {
        task.cleanup();
        task.reject(new DisplayError("disposed"));
      }
      pending.clear();
      listeners.clear();
      port.postMessage({ kind: "dispose" });
      port.close();
    },
  };
}
