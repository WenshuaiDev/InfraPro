import { connectBridge } from "./transport.js";
import { DisplayError } from "./error.js";
import { createReads } from "./reads.js";
import type { ReadMethod, ReadRequest, ReadData, ReadResult, ReadConsumer, ReadSubscriptionEvent, ReadFileChunk } from './read-types.js';
export * from './read-types.js';
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
  consumers: Set<Consumer<unknown>>;
  promise: Promise<void>;
  generation: bigint;
  sequence: bigint;
  busy: boolean;
  active: boolean;
  pendingReset?: SubscriptionEvent;
  pendingEvent?: SubscriptionEvent;
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
  const clearURLs=()=>{for(const url of urls)URL.revokeObjectURL(url);urls.clear();};
  const reads=createReads(request,subscribe,clearURLs);
  const isNamed=(params:unknown):boolean=>!!params&&typeof params==='object'&&'target' in params&&!!params.target&&typeof params.target==='object'&&('view' in params.target||'selection' in params.target);
  function compatibleQuery<M extends ReadMethod,L>(method:M){
    function call(params:ReadRequest<M>,options?:CallOptions):Promise<ReadResult<ReadData[M]>>;
    function call(params?:Query,options?:CallOptions):Promise<Result<L>>;
    function call(params:ReadRequest<M>|Query={},options?:CallOptions):Promise<ReadResult<ReadData[M]>|Result<L>>{return isNamed(params)?reads.call<ReadData[M]>(method,params,options):request<Result<L>>(method,params,options);}
    return call;
  }
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
      !/^\d+$/.test(event.generation) ||
      !/^\d+$/.test(event.sequence)
    )
      return;
    const generation = BigInt(event.generation),
      sequence = BigInt(event.sequence);
    if (generation < sub.generation) return;
    if(event.kind==='reset')reads.invalidate();
    if (generation > sub.generation) {
      sub.generation = generation;
      sub.sequence = 0n;
      sub.pendingReset = undefined;
      sub.pendingEvent = undefined;
    }
    // A reset must fence a slow consumer immediately. Deliver callbacks in
    // order after it settles, so an older async render cannot overwrite a
    // newer render. Retain only the bounded reset and next unacknowledged event.
    if (sub.busy) {
      if (event.kind === 'reset') sub.pendingReset = event;
      else if (!sub.pendingEvent || sequence < BigInt(sub.pendingEvent.sequence)) sub.pendingEvent = event;
      return;
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
      const next = sub.pendingReset ?? sub.pendingEvent;
      if (sub.pendingReset) sub.pendingReset = undefined;
      else sub.pendingEvent = undefined;
      if (next && sub.active && !disposed) void receive(next);
    }
  }
  function dispose() {
    if (disposed) return;
    reads.invalidate(true);
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
    clearURLs();
    transport.dispose();
    emit({ type: "disposed" });
    listeners.clear();
  }
  const off = transport.onEvent((event) => {
    if ("subscription_id" in event) {
      void receive(event);
    } else {
      emit(event);
      if(event.type==='release_changed')reads.invalidate();
      if (event.type === "revoked") dispose();
    }
  });
  async function subscribe(
    kind: string,
    input: unknown,
    consumer: Consumer<unknown>,
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
          if(shared.get(key)===current)shared.delete(key);
          throw error;
        });
    }
    const current = sub;
    const ownConsumer: Consumer<unknown> = (event) => consumer(event);
    current.consumers.add(ownConsumer);
    let ended = false;
    const unsubscribe = () => {
      if (ended) return;
      ended = true;
      opts.signal?.removeEventListener("abort", unsubscribe);
      current.consumers.delete(ownConsumer);
      if (!current.consumers.size) {
        current.active = false;
        if(shared.get(key)===current)shared.delete(key);
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
      await new Promise<void>((resolve,reject)=>{
        const abort=()=>{unsubscribe();reject(new DisplayError('aborted'));};
        opts.signal?.addEventListener('abort',abort,{once:true});
        current.promise.then(()=>{opts.signal?.removeEventListener('abort',abort);if(ended)reject(new DisplayError('aborted'));else resolve();},error=>{opts.signal?.removeEventListener('abort',abort);reject(error);});
        if(opts.signal?.aborted)abort();
      });
    } catch (error) {
      unsubscribe();
      throw error;
    }
    return { unsubscribe };
  }
  function readFile(params:ReadRequest<'spatial.readFile'>,opts?:CallOptions):Promise<ReadResult<Omit<ReadFileChunk,'base64'>&{bytes:ArrayBuffer}>>;
  function readFile(params:FileQuery,opts?:CallOptions):Promise<Result<FileChunk>>;
  async function readFile(params:FileQuery|ReadRequest<'spatial.readFile'>,opts?:CallOptions):Promise<Result<FileChunk>|ReadResult<Omit<ReadFileChunk,'base64'>&{bytes:ArrayBuffer}>> {
    if(isNamed(params)){
      const result=await reads.call<ReadFileChunk>('spatial.readFile',params,opts);
      if(result.state==='pending')return result;
      const binary=atob(result.data.base64),bytes=new Uint8Array(binary.length);
      for(let i=0;i<binary.length;i++)bytes[i]=binary.charCodeAt(i);
      const {base64,...metadata}=result.data;void base64;
      return {...result,data:{...metadata,bytes:bytes.buffer}};
    }
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
    views:reads.views,queries:reads.queries,devices:reads.devices,classification:reads.classification,map:reads.map,statistics:reads.statistics,
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
      types:reads.read('resources.types'),query:reads.read('resources.query'),tree:reads.read('resources.tree'),hierarchy:reads.read('resources.hierarchy'),detail:reads.read('resources.detail'),
      list: query<ResourcePage>("resources.list"),
      get: query<Resource>("resources.get"),
      summary:
        query<Pick<ResourcePage, "count" | "counts">>("resources.summary"),
    },
    channels: {
      list: compatibleQuery<'channels.list',Page<Channel>>("channels.list"),
      get: compatibleQuery<'channels.get',Channel>("channels.get"),
    },
    monitoring: {
      latest: compatibleQuery<'monitoring.latest',AnalyticResult>("monitoring.latest"),
      details: compatibleQuery<'monitoring.details',DetailResult>("monitoring.details"),
      trends: compatibleQuery<'monitoring.trends',AnalyticResult>("monitoring.trends"),
      statistics: compatibleQuery<'monitoring.statistics',AnalyticResult>("monitoring.statistics"),
      overview: compatibleQuery<'monitoring.overview',AnalyticResult>("monitoring.overview"),
      subscribe:(kind:SubscriptionInput['kind'],input:Query,consumer:Consumer,opts?:CallOptions)=>subscribe(kind,input,event=>consumer(event as SubscriptionEvent),opts),
      subscribeRead:<K extends SubscriptionInput['kind']>(kind:K,input:ReadRequest<`monitoring.${K}`>,consumer:ReadConsumer<ReadData[`monitoring.${K}`]>,opts?:CallOptions)=>subscribe(kind,input,event=>consumer(event as ReadSubscriptionEvent<ReadData[`monitoring.${K}`]>),opts),
    },
    sources: {
      bindings:reads.read('sources.bindings'),bindingHistory:reads.read('sources.bindingHistory'),
      get: query<SourceProjection>("sources.get"),
      history: query<Page<SourceProjection>>("sources.history"),
    },
    artifacts: {
      list: query<Page<Artifact>>("artifacts.list"),
      get: query<Artifact>("artifacts.get"),
      read: query<Page<ArtifactRow>>("artifacts.read"),
    },
    spatial: {
      assets:reads.read('spatial.assets'),versions:reads.read('spatial.versions'),locations:reads.read('spatial.locations'),componentLinks:reads.read('spatial.componentLinks'),settings:reads.read('spatial.settings'),
      list: query<Page<SpatialVersion>>("spatial.list"),
      manifest: compatibleQuery<'spatial.manifest',SpatialManifest>("spatial.manifest"),
      objects: compatibleQuery<'spatial.objects',Page<JSONValue>>("spatial.objects"),
      placements: compatibleQuery<'spatial.placements',Page<JSONValue>>("spatial.placements"),
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
