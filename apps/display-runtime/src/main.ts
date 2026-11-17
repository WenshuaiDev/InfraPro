import {runtimeMethods} from "./protocol.generated";
import { validParams } from "./params";
import { nonce, sha256 } from "./crypto";
import "./style.css";
type Session = {
  id: string;
  tenant_id: string;
  project_id: string;
  instance_id: string;
  release_id: string;
  preview: boolean;
  generation: string;
  environment: string;
  lease_until: string;
  target_release_id: string;
};
type Bootstrap = {
  session: Session;
  config: unknown;
  capabilities: unknown;
  entry_url: string;
  protocol: number;
  server_time: string;
  timezone: string;
  mode: string;
  simulated?: boolean;
};
type RuntimeConfig = {
  console_url: string;
  runtime_origin: string;
  asset_origin: string;
  protocol: number;
  switch_grace_ms: number;
  local?: boolean;
  simulated?: boolean;
};
const status = document.querySelector<HTMLElement>("#status")!,
  stage = document.querySelector<HTMLElement>("#stage")!,
  actions = document.querySelector<HTMLElement>("#actions")!;
let configuration: RuntimeConfig,
  active: Frame | undefined,
  preparing: Frame | undefined,
  stopped = false;
const methods = new Set<string>(runtimeMethods);

class Failure extends Error {
  constructor(public code: string, message = code, public issues: {path:string;code:string;message:string}[] = []) {
    super(message);
  }
}
function message(value: string) {
  status.textContent = configuration?.simulated && !value.startsWith("模拟数据") ? `模拟数据 · ${value}` : value;
}
async function post<T>(
  path: string,
  body: unknown = {},
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-Display-Protocol": "1" },
    body: JSON.stringify(body),
    signal,
  });
  const value = await response.json();
  if (!response.ok)
    throw new Failure(value.error?.code ?? value.code ?? "runtime_error", value.error?.message ?? value.message, value.error?.issues ?? value.issues ?? []);
  return value;
}
function fatal(code: string) {
  stopped = true;
  preparing?.dispose();
  active?.dispose();
  stage.replaceChildren();
  message(`运行已停止 · ${code}`);
  actions.replaceChildren();
  const button = document.createElement("button");
  button.textContent = "重新进入";
  button.onclick = () => location.reload();
  actions.append(button);
}
class Frame {
  iframe = document.createElement("iframe");
  port?: MessagePort;
  source?: EventSource;
  renewing = false;
  controllers = new Map<string, AbortController>();
  connected = false;
  dead = false;
  ready = false;
  handshake = nonce();
  timer: ReturnType<typeof setInterval>;
  leaseTimer: ReturnType<typeof setInterval>;
  deadline: ReturnType<typeof setTimeout>;
  loads = 0;
  constructor(
    public boot: Bootstrap,
    public switching = false,
  ) {
    if (boot.protocol !== 1) throw new Failure("version_incompatible");
    const entry = new URL(boot.entry_url);
    if (entry.origin !== configuration.asset_origin)
      throw new Failure("asset_origin_rejected");
    this.iframe.sandbox.add("allow-scripts");
    this.iframe.referrerPolicy = "no-referrer";
    this.iframe.title = boot.session.preview ? "大屏草稿预览" : "大屏应用";
    this.iframe.className = "preparing";
    this.iframe.src = entry.href;
    this.iframe.addEventListener("load", () => {
      this.loads++;
      if (this.loads > 1) this.fail("unexpected_navigation");
    });
    stage.append(this.iframe);
    this.deadline = setTimeout(
      () => this.fail("application_ready_timeout"),
      switching ? configuration.switch_grace_ms : 20000,
    );
    this.timer = setInterval(() => {
      void this.renew();
    }, 15000);
    this.leaseTimer = setInterval(() => {
      if (!this.dead && Date.now() >= Date.parse(this.boot.session.lease_until))
        fatal("lease_expired");
    }, 1000);
    this.stream();
  }
  url(action: string) {
    return `/runtime/${this.boot.session.id}/${action}`;
  }
  send(event: unknown) {
    if (!this.dead) this.port?.postMessage({ kind: "event", event });
  }
  begin(source: MessageEventSource | null) {
    if (this.dead || this.connected || source !== this.iframe.contentWindow)
      return;
    this.connected = true;
    const pair = new MessageChannel();
    this.port = pair.port1;
    let proved = false;
    this.port.onmessage = async (event: MessageEvent) => {
      const msg = event.data;
      if (!msg || this.dead) return;
      if (!proved) {
        if (
          msg.kind === "handshake" &&
          msg.nonce === this.handshake &&
          msg.protocol === 1
        ) {
          proved = true;
          this.send({ type: "connected" });
        } else this.fail("handshake_rejected");
        return;
      }
      if (msg.kind === "cancel") {
        this.controllers.get(msg.id)?.abort();
        return;
      }
      if (msg.kind === "dispose") {
        this.dispose();
        return;
      }
      if (
        msg.kind !== "request" ||
        msg.protocol !== 1 ||
        typeof msg.id !== "string" ||
        msg.id.length > 80 ||
        typeof msg.method !== "string"
      )
        return;
      if (
        this.controllers.size >= 32 ||
        this.controllers.has(msg.id) ||
        !validParams(msg.params)
      ) {
        this.port?.postMessage({
          kind: "response",
          id: msg.id,
          error: { code: "budget_exceeded" },
        });
        return;
      }
      const controller = new AbortController();
      this.controllers.set(msg.id, controller);
      try {
        const result = await this.rpc(
          msg.method,
          msg.params ?? {},
          controller.signal,
          msg.generation,
        );
        if (!controller.signal.aborted && !this.dead)
          this.port?.postMessage({ kind: "response", id: msg.id, result });
      } catch (e) {
        if (!this.dead)
          this.port?.postMessage({
            kind: "response",
            id: msg.id,
            error: e instanceof Failure ? {code:e.code,message:e.message,issues:e.issues} : {code:"request_failed"},
          });
      } finally {
        this.controllers.delete(msg.id);
      }
    };
    this.port.start();
    this.iframe.contentWindow!.postMessage(
      { kind: "infrapro.display.init", protocol: 1, nonce: this.handshake },
      "*",
      [pair.port2],
    );
  }
  async rpc(
    method: string,
    p: Record<string, unknown>,
    signal: AbortSignal,
    generation: string,
  ) {
    if (method === "context.get")
      return {
        ...this.boot.session,
        mode: this.boot.mode,
        timezone: this.boot.timezone,
        server_time: this.boot.server_time,
        simulated: this.boot.simulated,
      };
    if (method === "config.get") return this.boot.config;
    if (method === "capabilities.list") return this.boot.capabilities;
    if (method === "runtime.ready") {
      await post(this.url("control"), { action: "ready" }, signal);
      this.ready = true;
      clearTimeout(this.deadline);
      this.iframe.className = "";
      const old = active;
      // eslint-disable-next-line @typescript-eslint/no-this-alias
      active = this;
      preparing = undefined;
      if (old && old !== this) old.dispose();
      message(
        this.boot.simulated
          ? "模拟数据 · 本地调试"
          : this.boot.session.preview
            ? "草稿预览 · 已就绪"
            : "已连接 · 应用已就绪",
      );
      return;
    }
    if (method === "runtime.reportError")
      return post(
        this.url("control"),
        { action: "error", code: p.code },
        signal,
      );
    if (method === "bindings.resolve" || method === "bindings.list") {
      const bindings = await post<Record<string, unknown>>(this.url("bindings"), p, signal);
      return method === "bindings.resolve" ? bindings[String(p.name)] : bindings;
    }
    const sub: Record<string, string> = {
      "subscription.create": "subscribe",
      "subscription.cancel": "unsubscribe",
      "subscription.ack": "ack",
    };
    if (sub[method]) return post(this.url(sub[method]), p, signal);
    if (method === "navigation.open") {
      const result = await post<{ url: string }>(
        this.url("navigation"),
        p,
        signal,
      );
      const url = new URL(result.url);
      if (
        url.origin !== new URL(configuration.console_url).origin &&
        url.origin !== configuration.runtime_origin
      )
        throw new Failure("navigation_rejected");
      const a = document.createElement("a");
      a.href = url.href;
      a.target = "_blank";
      a.rel = "noopener noreferrer";
      a.textContent = "打开所选业务页面";
      actions.replaceChildren(a);
      return;
    }
    if (methods.has(method))
      return post(
        this.url("query"),
        { method, params: p, generation: generation ?? "1" },
        signal,
      );
    throw new Failure("method_not_allowed");
  }
  stream() {
    if (this.dead) return;
    this.source = new EventSource(this.url("events"));
    this.source.onopen = () => {
      this.send({ type: "connected" });
      if (this.ready)
        message(
          this.boot.simulated ? "模拟数据 · 本地调试" : this.boot.session.preview ? "草稿预览 · 已就绪" : "已连接 · 应用已就绪",
        );
    };
    this.source.onerror = () => {
      if (this.dead) return;
      this.send({ type: "disconnected" });
      this.send({ type: "reconnecting" });
      message("连接中断，正在重连 · 数据截止时间保持原值");
      // A rejected SSE reconnect has no readable response body. Probe current
      // authority now so revocation at the reconnect boundary still unloads.
      void this.renew();
    };
    this.source.addEventListener("data", (e) => {
      if (this.dead) return;
      try {
        this.send(JSON.parse(e.data));
      } catch {
        this.fail("protocol_invalid");
      }
    });
    this.source.addEventListener("control", (e) => {
      if (this.dead) return;
      try {
        const v = JSON.parse(e.data);
        if (v.kind === "revoked") {
          fatal(v.code ?? "session_revoked");
          return;
        }
        if (
          v.target_release_id &&
          v.target_release_id !== this.boot.session.release_id &&
          !this.boot.session.preview &&
          active === this &&
          !preparing
        ) {
          this.send({
            type: "release_changed",
            target_release_id: v.target_release_id,
          });
          void this.switch();
        }
      } catch {
        this.fail("protocol_invalid");
      }
    });
  }
  async renew() {
    if (this.dead || this.renewing) return;
    this.renewing = true;
    try {
      const session = await post<Session>(this.url("renew"), {}, AbortSignal.timeout(12000));
      if (this.dead) return;
      this.boot.session = session;
    } catch (e) {
      if (this.dead) return;
      if (
        e instanceof Failure &&
        [
          "session_expired",
          "session_revoked",
          "revoked",
          "binding_invalid",
          "release_changed",
          "package_unavailable",
          "not_found",
          "permission_denied",
        ].includes(e.code)
      ) {
        fatal(e.code);
        return;
      }
      if (Date.now() >= Date.parse(this.boot.session.lease_until))
        fatal("lease_expired");
    } finally {
      this.renewing = false;
    }
  }
  async switch() {
    message("正在准备新发布…");
    try {
      preparing = new Frame(await post<Bootstrap>(this.url("switch")), true);
    } catch (e) {
      fatal(e instanceof Failure ? e.code : "switch_failed");
    }
  }
  fail(code: string) {
    if (this.dead) return;
    void post(this.url("control"), { action: "error", code }).catch(() => {});
    fatal(code);
  }
  dispose() {
    if (this.dead) return;
    this.dead = true;
    clearInterval(this.timer);
    clearInterval(this.leaseTimer);
    clearTimeout(this.deadline);
    this.source?.close();
    for (const c of this.controllers.values()) c.abort();
    this.controllers.clear();
    this.port?.close();
    this.iframe.remove();
    void post(this.url("control"), { action: "close" }).catch(() => {});
  }
}
window.addEventListener("message", (e) => {
  if (e.data?.kind !== "infrapro.display.hello" || e.data.protocol !== 1)
    return;
  for (const frame of [active, preparing]) frame?.begin(e.source);
});
window.addEventListener("pagehide", () => {
  active?.dispose();
  preparing?.dispose();
});
document.addEventListener("visibilitychange", () =>
  active?.send({ type: document.hidden ? "paused" : "resumed" }),
);
async function start() {
  configuration = await (await fetch("/runtime-config")).json();
  const hash = new URLSearchParams(location.hash.slice(1));
  history.replaceState(null, "", location.pathname + location.search);
  if (configuration.local) {
    preparing = new Frame(await post<Bootstrap>("/local/bootstrap"));
    return;
  }
  if (hash.has("code")) {
    const verifier = sessionStorage.getItem("display_verifier") ?? "";
    sessionStorage.removeItem("display_verifier");
    const boot = await post<Bootstrap>("/runtime/exchange", {
      code: hash.get("code"),
      verifier,
      audience: location.origin,
    });
    sessionStorage.setItem("display_session", boot.session.id);
    preparing = new Frame(boot);
    return;
  }
  if (location.pathname === "/terminal") {
    try {
      preparing = new Frame(await post<Bootstrap>("/runtime/terminal/start"));
      return;
    } catch {
      message("固定终端 · 输入管理员签发的一次性配对码");
    }
    const form = document.createElement("form"),
      label = document.createElement("label"),
      input = document.createElement("input"),
      button = document.createElement("button");
    input.autocomplete = "off";
    input.required = true;
    input.id = "pair-code";
    label.htmlFor = input.id;
    label.textContent = "一次性配对码";
    button.textContent = "配对并启动";
    form.append(label, input, button);
    form.onsubmit = async (e) => {
      e.preventDefault();
      try {
        await post("/runtime/terminal/pair", { code: input.value });
        input.value = "";
        preparing = new Frame(await post<Bootstrap>("/runtime/terminal/start"));
        actions.replaceChildren();
      } catch (err) {
        message(
          `配对失败 · ${err instanceof Failure ? err.code : "network_error"}`,
        );
      }
    };
    actions.append(form);
    return;
  }
  const instance = location.pathname.split("/")[2];
  if (!instance) throw new Failure("instance_required");
  const stored = sessionStorage.getItem("display_session");
  if (stored) {
    try {
      const boot = await post<Bootstrap>(`/runtime/${stored}/bootstrap`);
      if (boot.session.instance_id === instance) {
        preparing = new Frame(boot);
        return;
      }
    } catch {
      sessionStorage.removeItem("display_session");
    }
  }
  const verifier = nonce();
  sessionStorage.setItem("display_verifier", verifier);
  const target = new URL(
    configuration.console_url.replace(/\/$/, "") + "/display-launch",
  );
  target.searchParams.set("instance", instance);
  target.searchParams.set("challenge", sha256(verifier));
  target.searchParams.set("audience", location.origin);
  const config = new URLSearchParams(location.search).get("config");
  if (config) target.searchParams.set("config", config);
  location.replace(target.href);
}
void start().catch((e) => {
  if (!stopped) fatal(e instanceof Failure ? e.code : "startup_failed");
});
