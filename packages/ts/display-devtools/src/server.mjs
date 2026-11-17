import http from "node:http";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, extname } from "node:path";
import { randomBytes, randomUUID } from "node:crypto";
import { createFixtureTransport } from "../lib/sdk/testing.js";
import { digest, media, safePath } from "./package.mjs";
const shell = fileURLToPath(new URL("../lib/shell/", import.meta.url));
const token = () => randomBytes(32).toString("hex");
function json(res, value, status = 200) {
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Cache-Control": "no-store",
  });
  res.end(JSON.stringify(value));
}
async function body(req, max = 1024 ** 2) {
  const chunks = [];
  let size = 0;
  for await (const b of req) {
    size += b.length;
    if (size > max) throw new Error("request_too_large");
    chunks.push(b);
  }
  const raw = Buffer.concat(chunks);
  return raw.length ? JSON.parse(raw.toString()) : {};
}
async function serveFile(res, root, name) {
  if (!safePath(name)) return json(res, { code: "not_found" }, 404);
  try {
    const data = await readFile(resolve(root, name));
    res.writeHead(200, {
      "Content-Type": media[extname(name)] ?? "application/octet-stream",
      "Cache-Control": "no-store",
    });
    res.end(data);
  } catch {
    return json(res, { code: "not_found" }, 404);
  }
}
export async function startServer(options = {}) {
  const port = Number(options.port ?? 5190);
  if (!Number.isInteger(port) || port < 1024 || port > 65533)
    throw new Error("port must be 1024–65533");
  const audience = `http://127.0.0.1:${port}`,
    assetOrigin = `http://127.0.0.1:${port + 1}`,
    scenario = options.scenario ?? "normal",
    fixture = createFixtureTransport(scenario),
    listeners = new Set(),
    history = [];
  let boot,
    credential,
    runtime,
    verifier = token(),
    state = token(),
    authorizedResolve,
    closed = false;
  const authorized = new Promise((resolve) => {
    authorizedResolve = resolve;
  });
  authorized.catch(() => {});
  fixture.onEvent((event) => {
    const entry = `event: ${"subscription_id" in event ? "data" : "control"}\ndata: ${JSON.stringify("subscription_id" in event ? event : { kind: event.type, code: event.code })}\n\n`;
    history.push(entry);
    if (history.length > 16) history.shift();
    for (const res of listeners) res.write(entry);
  });
  async function remote(path, params = {}, signal, raw, headers = {}) {
    const response = await fetch(runtime + path, {
      method: "POST",
      redirect: "error",
      headers: {
        Origin: runtime,
        "X-Display-Protocol": "1",
        "Content-Type": raw ? "application/octet-stream" : "application/json",
        ...(credential ? { Authorization: `Bearer ${credential}` } : {}),
        ...headers,
      },
      body: raw ?? JSON.stringify(params),
      signal: signal ?? AbortSignal.timeout(20000),
    });
    const value = await response.json();
    if (!response.ok) {
      const e = new Error(value.message ?? value.code);
      e.code = value.code;
      e.status = response.status;
      e.issues = value.issues;
      throw e;
    }
    return value;
  }
  async function retryRemote(...args) {
    for (let attempt=0; ; attempt++) {
      try { return await remote(...args); }
      catch(e) {
        if (attempt>=2 || (e.status && e.status<500 && e.status!==429)) throw e;
        await new Promise(resolve=>setTimeout(resolve,250*2**attempt));
      }
    }
  }
  const server = http.createServer(async (req, res) => {
    try {
      const url = new URL(req.url, audience);
      res.setHeader("Cache-Control", "no-store");
      res.setHeader("Referrer-Policy", "no-referrer");
      res.setHeader("X-Content-Type-Options", "nosniff");
      if (req.headers.host !== new URL(audience).host)
        return json(res, { code: "host_rejected" }, 403);
      if (
        req.method === "POST" &&
        (req.headers.origin !== audience ||
          req.headers["x-display-protocol"] !== "1")
      )
        return json(res, { code: "origin_rejected" }, 403);
      if (url.pathname === "/runtime-config")
        return json(res, {
          console_url: options.console ?? audience,
          runtime_origin: audience,
          asset_origin: assetOrigin,
          protocol: 1,
          switch_grace_ms: 30000,
          local: true,
          simulated: !options.real && !options.upload,
        });
      if (url.pathname === "/pair") {
        res.setHeader("Content-Type", "text/html");
        res.end(
          '<!doctype html><meta charset="utf-8"><h1>正在完成本机配对…</h1><script src="/pair.js"></script>',
        );
        return;
      }
      if (url.pathname === "/pair.js") {
        res.setHeader("Content-Type", "text/javascript");
        res.end(
          `const q=new URLSearchParams(location.hash.slice(1));history.replaceState(null,'','/pair');fetch('/pair',{method:'POST',headers:{'Content-Type':'application/json','X-Display-Protocol':'1'},body:JSON.stringify(Object.fromEntries(q))}).then(async r=>{if(!r.ok)throw Error('配对失败');document.querySelector('h1').textContent='配对完成，可以返回开发工具';${options.upload ? '' : "location.replace('/')"}}).catch(()=>document.querySelector('h1').textContent='配对失败，请重新运行 login')`,
        );
        return;
      }
      if (url.pathname === "/pair" && req.method === "POST") return; // POST handled below before page routing.
      if (url.pathname === "/local/pair" && req.method === "POST") {
        const input = await body(req);
        if (input.state !== state || !input.code || boot || credential)
          return json(res, { code: "pair_rejected" }, 403);
        const result = await remote(
          options.upload ? "/tool/exchange" : "/runtime/exchange",
          { code: input.code, verifier, audience, ...(options.upload ? {} : {tool:true}) },
        );
        verifier = "";
        state = "";
        if (options.upload) {
          credential = result.token;
          boot = { session: { id: result.id } };
        } else {
          credential = result.tool_token;
          delete result.tool_token;
          boot = { ...result, entry_url: assetOrigin + "/index.html" };
        }
        authorizedResolve();
        return json(res, { paired: true });
      }
      if (url.pathname === "/local/bootstrap" && req.method === "POST") {
        if (options.real) {
          if (!boot) return json(res, { code: "login_required" }, 401);
          return json(res, boot);
        }
        const context = await fixture.request("context.get", {});
        boot = {
          session: {
            ...context,
            id: "mock",
            lease_until: new Date(Date.now() + 60000).toISOString(),
            target_release_id: "mock-release",
          },
          config: await fixture.request("config.get", {}),
          capabilities: await fixture.request("capabilities.list", {}),
          entry_url: assetOrigin + "/index.html",
          protocol: 1,
          server_time: context.server_time,
          timezone: context.timezone,
          mode: "fixed",
          simulated: true,
        };
        return json(res, boot);
      }
      if (url.pathname.startsWith("/runtime/")) {
        if (!boot) return json(res, { code: "login_required" }, 401);
        const [, , session, action] = url.pathname.split("/");
        if (session !== boot.session.id)
          return json(res, { code: "not_found" }, 404);
        if (options.real) {
          if (action === "events") {
            const abort = new AbortController();
            req.on("close", () => abort.abort());
            const upstream = await fetch(runtime + url.pathname, {
              redirect: "error",
              headers: {
                Authorization: `Bearer ${credential}`,
                "Last-Event-ID": req.headers["last-event-id"] ?? "",
              },
              signal: abort.signal,
            });
            res.writeHead(upstream.status, {
              "Content-Type":
                upstream.headers.get("content-type") ?? "text/event-stream",
            });
            for await (const b of upstream.body) res.write(b);
            res.end();
            return;
          }
          if (
            ![
              "query",
              "bindings",
              "subscribe",
              "unsubscribe",
              "ack",
              "renew",
              "control",
              "navigation",
            ].includes(action)
          )
            return json(res, { code: "method_not_allowed" }, 405);
          const abort = new AbortController();
          res.on("close", () => abort.abort());
          return json(
            res,
            await remote(url.pathname, await body(req), abort.signal),
          );
        }
        if (action === "events") {
          res.writeHead(200, { "Content-Type": "text/event-stream" });
          res.write(": simulated\n\n");
          listeners.add(res);
          for (const item of history) res.write(item);
          req.on("close", () => listeners.delete(res));
          return;
        }
        const input = await body(req);
        if (action === "query")
          return json(res, await fixture.request(input.method, input.params, {generation:input.generation}));
        if (action === "bindings")
          return json(res, input.name ? { [input.name]: await fixture.request("bindings.resolve", input) } : {site: await fixture.request("bindings.resolve", {name:"site"})});
        if (action === "subscribe")
          return json(res, await fixture.request("subscription.create", input));
        if (action === "renew") {
          boot.session.lease_until = new Date(Date.now() + 60000).toISOString();
          return json(res, boot.session);
        }
        return json(res, {});
      }
      if (req.method !== "GET")
        return json(res, { code: "method_not_allowed" }, 405);
      return serveFile(
        res,
        shell,
        url.pathname === "/" ? "index.html" : url.pathname.slice(1),
      );
    } catch (error) {
      if (res.headersSent || res.destroyed) { res.end(); return; }
      json(
        res,
        { code: error.code ?? "request_failed", message: error.message, issues: error.issues ?? [] },
        error.status ?? 400,
      );
    }
  });
  // The browser callback uses a dedicated POST path, so HTML and redemption cannot collide.
  server.prependListener("request", (req) => {
    if (req.url === "/pair" && req.method === "POST") req.url = "/local/pair";
  });
  const assets = http.createServer(async (req, res) => {
    const u = new URL(req.url, assetOrigin);
    if (req.headers.host !== new URL(assetOrigin).host || req.method !== "GET")
      return json(res, { code: "not_found" }, 404);
    res.setHeader("Access-Control-Allow-Origin", "*");
    res.setHeader(
      "Content-Security-Policy",
      `default-src 'none'; script-src ${assetOrigin} 'unsafe-inline' 'wasm-unsafe-eval'; style-src ${assetOrigin} 'unsafe-inline'; img-src ${assetOrigin} data: blob:; font-src ${assetOrigin} data:; connect-src ${assetOrigin}; worker-src blob:; frame-ancestors ${audience}; sandbox allow-scripts; base-uri 'none'; form-action 'none'`,
    );
    return serveFile(
      res,
      resolve(options.dir ?? "dist"),
      u.pathname.slice(1) || "index.html",
    );
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", resolve);
  });
  try {
    await new Promise((resolve, reject) => {
      assets.once("error", reject);
      assets.listen(port + 1, "127.0.0.1", resolve);
    });
  } catch (e) {
    server.close();
    throw e;
  }
  const close = () => {
    if (closed) return Promise.resolve();
    closed = true;
    for (const res of listeners) res.end();
    fixture.dispose();
    server.closeAllConnections();
    assets.closeAllConnections();
    const done = Promise.all([new Promise(resolve=>server.close(resolve)),new Promise(resolve=>assets.close(resolve))]);
    credential = undefined;
    return done;
  };
  let loginURL;
  if (options.real || options.upload) {
    if (
      !options.console ||
      !options.instance ||
      (!options.config && !options.upload)
    ) {
      close();
      throw new Error(
        "real preview needs --console, --instance and --config; upload needs --console and --instance",
      );
    }
    let url, ru;
    try {
      url = new URL(options.console);
      if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash || /[?#]/.test(options.console))
        throw new Error("console must be an HTTP/HTTPS base URL without credentials, query or fragment");
      ru = new URL(options.runtime);
      if (ru.protocol !== url.protocol || ru.pathname !== "/" || ru.username || ru.password || ru.search || ru.hash || /[?#]/.test(options.runtime))
        throw new Error("runtime must be an exact HTTP/HTTPS origin with the console scheme");
      url.pathname = url.pathname.replace(/\/$/, "") + "/display-launch";
    } catch (error) {
      await close();
      throw error;
    }
    url.search = new URLSearchParams({
      instance: options.instance,
      config: options.config ?? "",
      challenge: digest(verifier),
      audience,
      state,
      purpose: options.upload ? "upload" : "preview",
    }).toString();
    loginURL = url.href;
    // Only the management response may select its configured runtime. Callback includes it.
    runtime = ru.origin;
  }
  async function upload(path) {
    await authorized;
    const data = await readFile(path);
    const root = `/tool/${boot.session.id}/`,
      inspect = await remote(root + "inspect"),
      meta = (revision) => ({
        request_id: randomUUID(),
        reason: "开发工具上传",
        expected_revision: revision,
      });
    if (!options.resume && data.length > Number(inspect.max_package_bytes)) throw new Error(`ZIP exceeds configured ${inspect.max_package_bytes} byte limit`);
    let revision = inspect.instance.revision;
    let created;
    let completedParts = [];
    if (options.resume) {
      const prior = await remote(root+`upload.get?target=${encodeURIComponent(options.resume)}`);
      if (prior.size !== String(data.length) || prior.sha256 !== digest(data)) throw new Error("Resume ZIP does not match upload");
      if(prior.state === 'uploaded') return {upload_id:prior.id,state:prior.state,jobs:prior.jobs};
      if(prior.state !== 'receiving') throw new Error("Upload cannot be resumed: "+prior.state);
      created={upload_id:prior.id,revision};completedParts=prior.parts;
    } else {
      created = await retryRemote(root + "upload", {
        ...meta(revision), size: data.length, sha256: digest(data),
      });
    }
    revision = created.revision;
    try {
      for (let part = 0; part * 4194304 < data.length; part++) {
        const chunk = data.subarray(part * 4194304, (part + 1) * 4194304);
        const prior=completedParts.find(p=>p.part===part);
        if(prior){if(prior.sha256!==digest(chunk))throw new Error('Remote part hash mismatch');continue;}
        await retryRemote(
          root + `part?upload=${created.upload_id}&part=${part}`,
          {},
          undefined,
          chunk,
          { "X-Content-SHA256": digest(chunk) },
        );
      }
      const complete = await retryRemote(
        root + `upload.complete?target=${created.upload_id}`,
        meta(revision),
      );
      for(let n=0;n<120;n++) {
        const status=await retryRemote(root+`upload.get?target=${created.upload_id}`);
        const job=status.jobs?.[0];
        if(job?.state==='ready')return {upload_id:created.upload_id,...complete,state:'ready',job};
        if(job?.state==='failed'||job?.state==='canceled'){const e=new Error('Server package validation '+job.state);e.code=job.error_code||job.state;e.issues=job.issues;throw e;}
        await new Promise(resolve=>setTimeout(resolve,1000));
      }
      return { upload_id: created.upload_id, ...complete, state:'processing' };
    } catch (e) {
      e.upload_id = created.upload_id;
      throw e;
    }
  }
  return { url: audience, loginURL, authorized, close, upload };
}
