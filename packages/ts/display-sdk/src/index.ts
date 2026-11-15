import { connectBridge } from "./transport.js";
import { DisplayError } from "./error.js";
import type {
  Alert,
  AlertConsumer,
  AlertQuery,
  AlertSummary,
  AnalyticResult,
  Artifact,
  ArtifactRow,
  Binding,
  CallOptions,
  Capability,
  Channel,
  Consumer,
  Context,
  DetailResult,
  DisplayTransport,
  FileChunk,
  FileQuery,
  JSONValue,
  Page,
  Query,
  Resource,
  ResourcePage,
  Result,
  RuntimeEvent,
  SourceProjection,
  SpatialManifest,
  SpatialVersion,
  Subscription,
  SubscriptionEvent,
  SubscriptionInput,
} from "./types.js";
export * from "./types.js";
export { DisplayError } from "./error.js";
function stable(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
  if (value && typeof value === "object")
    return `{${Object.entries(value)
      .filter(([, v]) => v !== undefined)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([k, v]) => `${JSON.stringify(k)}:${stable(v)}`)
      .join(",")}}`;
  return JSON.stringify(value) ?? "null";
}
interface Shared {
  id?: string;
  consumers: Set<Consumer>;
  promise: Promise<void>;
  generation: bigint;
  sequence: bigint;
  busy: boolean;
  active: boolean;
}
/** Connect only inside the trusted shell. Inject a transport for deterministic tests. */
export async function connect(
  options: { signal?: AbortSignal; transport?: DisplayTransport } = {},
) {
  const transport = options.transport ?? (await connectBridge(options.signal));
  let disposed = false;
  const shared = new Map<string, Shared>(),
    byID = new Map<string, Shared>(),
    early = new Map<string, SubscriptionEvent>();
  const listeners = new Set<(event: RuntimeEvent) => void>(),
    urls = new Set<string>();
  const emit = (event: RuntimeEvent) => {
    for (const f of listeners) {
      try {
        f(event);
      } catch {
        /* Diagnostics cannot interrupt cleanup. */
      }
    }
  };
  const request = <T>(
    method: string,
    params: unknown = {},
    opts?: CallOptions,
  ): Promise<T> =>
    disposed
      ? Promise.reject(new DisplayError("disposed"))
      : transport.request<T>(method, params, opts);
  const query =
    <T>(method: string) =>
    (params: Query = {}, opts?: CallOptions) =>
      request<Result<T>>(method, params, opts);
  async function receive(event: SubscriptionEvent) {
    const sub = byID.get(event.subscription_id);
    if (!sub) {
      if (early.size < 16) early.set(event.subscription_id, event);
      return;
    }
    if (
      !sub.active ||
      sub.busy ||
      !/^\d+$/.test(event.generation) ||
      !/^\d+$/.test(event.sequence)
    )
      return;
    const generation = BigInt(event.generation),
      sequence = BigInt(event.sequence);
    if (generation < sub.generation) return;
    if (generation > sub.generation) {
      sub.generation = generation;
      sub.sequence = 0n;
    }
    if (sequence <= sub.sequence && event.kind !== "reset") {
      await request("subscription.ack", {
        id: sub.id,
        generation: event.generation,
        sequence: event.sequence,
      }).catch(() => {});
      return;
    }
    if (event.kind !== "reset" && sequence !== sub.sequence + 1n) {
      emit({ type: "error", code: "cursor_gap" });
      return;
    }
    sub.busy = true;
    try {
      await Promise.all([...sub.consumers].map((f) => f(event)));
      if (
        sub.active &&
        !disposed &&
        generation === sub.generation &&
        event.kind !== "reset"
      ) {
        await request("subscription.ack", {
          id: sub.id,
          generation: event.generation,
          sequence: event.sequence,
        });
        sub.sequence = sequence;
        if(generation>1n&&sequence===1n)emit({type:"resynchronized"});
      }
    } catch {
      emit({
        type: "error",
        code: "consumer_failed",
      }); /* Server replays the unacknowledged event. */
    } finally {
      sub.busy = false;
    }
  }
  function dispose() {
    if (disposed) return;
    for (const sub of shared.values()) {
      sub.active = false;
      if (sub.id)
        void request("subscription.cancel", { id: sub.id }).catch(() => {});
    }
    disposed = true;
    off();
    shared.clear();
    byID.clear();
    early.clear();
    for (const url of urls) URL.revokeObjectURL(url);
    urls.clear();
    transport.dispose();
    emit({ type: "disposed" });
    listeners.clear();
  }
  const off = transport.onEvent((event) => {
    if ("subscription_id" in event) {
      void receive(event);
    } else {
      emit(event);
      if (event.type === "revoked") dispose();
    }
  });
  async function subscribe(
    kind: SubscriptionInput["kind"] | "alerts",
    input: Query | AlertQuery,
    consumer: Consumer,
    opts: CallOptions = {},
  ): Promise<Subscription> {
    if (disposed || opts.signal?.aborted)
      throw new DisplayError(disposed ? "disposed" : "aborted");
    const key = stable({ kind, query: input, generation: opts.generation });
    let sub = shared.get(key);
    if (!sub) {
      sub = {
        consumers: new Set(),
        promise: Promise.resolve(),
        generation: 0n,
        sequence: 0n,
        busy: false,
        active: true,
      };
      const current = sub;
      shared.set(key, current);
      current.promise = request<{ id: string }>("subscription.create", {
        kind,
        query: input,
        generation: opts.generation ?? "1",
      })
        .then(async (result) => {
          current.id = result.id;
          if (!current.active || disposed) {
            await request("subscription.cancel", { id: result.id }).catch(
              () => {},
            );
            return;
          }
          byID.set(result.id, current);
          const queued = early.get(result.id);
          if (queued) {
            early.delete(result.id);
            void receive(queued);
          }
        })
        .catch((error) => {
          current.active = false;
          shared.delete(key);
          throw error;
        });
    }
    const current = sub;
    const ownConsumer: Consumer = (event) => consumer(event);
    current.consumers.add(ownConsumer);
    let ended = false;
    const unsubscribe = () => {
      if (ended) return;
      ended = true;
      opts.signal?.removeEventListener("abort", unsubscribe);
      current.consumers.delete(ownConsumer);
      if (!current.consumers.size) {
        current.active = false;
        shared.delete(key);
        if (current.id) {
          byID.delete(current.id);
          void request("subscription.cancel", { id: current.id }).catch(
            () => {},
          );
        }
      }
    };
    opts.signal?.addEventListener("abort", unsubscribe, { once: true });
    try {
      await current.promise;
    } catch (error) {
      unsubscribe();
      throw error;
    }
    return { unsubscribe };
  }
  async function readFile(
    params: FileQuery,
    opts?: CallOptions,
  ): Promise<Result<FileChunk>> {
    const result = await request<
      Result<Omit<FileChunk, "bytes"> & { base64: string }>
    >("spatial.readFile", params, opts);
    const binary = atob(result.data.base64),
      bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    const metadata = {
      file_id: result.data.file_id,
      media_type: result.data.media_type,
      size: result.data.size,
      offset: result.data.offset,
      length: result.data.length,
      sha256: result.data.sha256,
    };
    return { ...result, data: { ...metadata, bytes: bytes.buffer } };
  }
  return {
    context: { get: () => request<Context>("context.get") },
    config: { get: () => request<Record<string, JSONValue>>("config.get") },
    capabilities: {
      list: () => request<Capability[]>("capabilities.list"),
      get: async (name: string) => (await request<Capability[]>("capabilities.list")).find(c => c.name === name),
    },
    bindings: {
      list: (opts?: CallOptions) => request<Record<string, Binding>>("bindings.list", {}, opts),
      resolve: (name: string, opts?: CallOptions) =>
        request<Binding>("bindings.resolve", { name }, opts),
    },
    resources: {
      list: query<ResourcePage>("resources.list"),
      get: query<Resource>("resources.get"),
      summary:
        query<Pick<ResourcePage, "count" | "counts">>("resources.summary"),
    },
    channels: {
      list: query<Page<Channel>>("channels.list"),
      get: query<Channel>("channels.get"),
    },
    monitoring: {
      latest: query<AnalyticResult>("monitoring.latest"),
      details: query<DetailResult>("monitoring.details"),
      trends: query<AnalyticResult>("monitoring.trends"),
      statistics: query<AnalyticResult>("monitoring.statistics"),
      overview: query<AnalyticResult>("monitoring.overview"),
      subscribe,
    },
    sources: {
      get: query<SourceProjection>("sources.get"),
      history: query<Page<SourceProjection>>("sources.history"),
    },
    artifacts: {
      list: query<Page<Artifact>>("artifacts.list"),
      get: query<Artifact>("artifacts.get"),
      read: query<Page<ArtifactRow>>("artifacts.read"),
    },
    spatial: {
      list: query<Page<SpatialVersion>>("spatial.list"),
      manifest: query<SpatialManifest>("spatial.manifest"),
      objects: query<Page<JSONValue>>("spatial.objects"),
      placements: query<Page<JSONValue>>("spatial.placements"),
      readFile,
      createObjectURL: (blob: Blob) => {
        if (disposed) throw new DisplayError("disposed");
        const url = URL.createObjectURL(blob);
        urls.add(url);
        return url;
      },
      revokeObjectURL: (url: string) => {
        if (urls.delete(url)) URL.revokeObjectURL(url);
      },
    },
    alerts: {
      query: (q: AlertQuery = {}, o?: CallOptions) =>
        request<Result<Page<Alert>>>("alerts.query", q, o),
      summary: (q: AlertQuery = {}, o?: CallOptions) =>
        request<Result<AlertSummary>>("alerts.summary", q, o),
      subscribe: (q: AlertQuery, f: AlertConsumer, o?: CallOptions) => subscribe('alerts', q, event=>f(event as unknown as SubscriptionEvent<Page<Alert>>), o),
    },
    navigation: {
      open: (kind: "resource" | "monitoring" | "display", id: string) =>
        request<void>("navigation.open", { kind, id }),
    },
    runtime: {
      on: (f: (e: RuntimeEvent) => void) => {
        listeners.add(f);
        return () => {
          listeners.delete(f);
        };
      },
      ready: () => request<void>("runtime.ready"),
      reportError: (code: string) =>
        request<void>("runtime.reportError", { code }),
    },
    dispose,
  };
}
export type DisplayClient = Awaited<ReturnType<typeof connect>>;
