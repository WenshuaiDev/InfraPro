import {
  readFile,
  writeFile,
  readdir,
  lstat,
  realpath,
} from "node:fs/promises";
import { resolve, relative, extname } from "node:path";
import { createHash } from "node:crypto";
import { inflateRawSync } from "node:zlib";
export const media = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".wasm": "application/wasm",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".webp": "image/webp",
  ".gif": "image/gif",
  ".svg": "image/svg+xml",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".ttf": "font/ttf",
  ".glb": "model/gltf-binary",
  ".gltf": "model/gltf+json",
  ".bin": "application/octet-stream",
  ".ico": "image/x-icon",
  ".txt": "text/plain",
  ".map": "application/json",
};
const caps = new Set([
  "resources.read",
  "channels.read",
  "monitoring.latest",
  "monitoring.details",
  "monitoring.trends",
  "monitoring.statistics",
  "monitoring.overview",
  "sources.read",
  "artifacts.read",
  "spatial.read",
  "alerts.query",
  "alerts.summary",
  "alerts.subscribe",
]);
export const digest = (b) => createHash("sha256").update(b).digest("hex");
function fail(path, message) {
  const e = new Error(`${path}: ${message}`);
  e.code = "package_invalid";
  e.issues = [{ path, code: "invalid", message }];
  throw e;
}
// Paths and JSON strings must reject control bytes.
export const safePath = (p) =>
  typeof p === "string" &&
  p.length > 0 &&
  Buffer.byteLength(p) <= 240 &&
  // eslint-disable-next-line no-control-regex
  !/[\\:%?#\x00]/.test(p) &&
  !p.startsWith("/") &&
  !p.split("/").some((s) => !s || s.startsWith(".")) &&
  p.split("/").length <= 12;
export function strictJSON(text) {
  const tokens =
    text.match(
      // eslint-disable-next-line no-control-regex, no-useless-escape
      /"(?:[^"\\\u0000-\u001f]|\\(?:["\\/bfnrt]|u[\da-fA-F]{4}))*"|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}\[\],:]|\S/g,
    ) ?? [];
  let i = 0;
  function value(depth = 0) {
    if (depth > 40) fail("/", "JSON nesting exceeds budget");
    let t = tokens[i++];
    if (t === "{") {
      const out = Object.create(null);
      if (tokens[i] === "}") {
        i++;
        return out;
      }
      while (true) {
        const k = tokens[i++];
        if (!k?.startsWith('"')) fail("/", "Invalid property");
        const name = JSON.parse(k);
        if (Object.hasOwn(out, name)) fail("/", "Duplicate JSON property");
        if (tokens[i++] !== ":") fail("/", "Invalid JSON");
        out[name] = value(depth + 1);
        const end = tokens[i++];
        if (end === "}") return out;
        if (end !== ",") fail("/", "Invalid JSON");
      }
    }
    if (t === "[") {
      const out = [];
      if (tokens[i] === "]") {
        i++;
        return out;
      }
      while (true) {
        out.push(value(depth + 1));
        const end = tokens[i++];
        if (end === "]") return out;
        if (end !== ",") fail("/", "Invalid JSON");
      }
    }
    try {
      return JSON.parse(t);
    } catch {
      fail("/", "Invalid JSON");
    }
  }
  const out = value();
  if (i !== tokens.length) fail("/", "Trailing JSON");
  return out;
}
function keys(value, allowed, path) {
  if (!value || typeof value !== "object" || Array.isArray(value))
    fail(path, "Expected object");
  for (const key of Object.keys(value))
    if (!allowed.includes(key)) fail(path + "/" + key, "Unknown field");
}
function schema(s, path, depth = 0) {
  keys(
    s,
    [
      "type",
      "title",
      "description",
      "default",
      "properties",
      "required",
      "additionalProperties",
      "enum",
      "minLength",
      "maxLength",
      "minimum",
      "maximum",
      "items",
      "maxItems",
    ],
    path,
  );
  if (
    depth > 8 ||
    !["object", "string", "number", "integer", "boolean", "array"].includes(
      s.type,
    )
  )
    fail(path, "Unsupported schema");
  if (s.additionalProperties) fail(path, "Additional properties are forbidden");
  if (s.type === "object") {
    if (Object.keys(s.properties ?? {}).length > 64)
      fail(path, "Too many properties");
    for (const [key, v] of Object.entries(s.properties ?? {}))
      schema(v, path + "/" + key, depth + 1);
    for (const key of s.required ?? [])
      if (!Object.hasOwn(s.properties ?? {}, key))
        fail(path, "Required property missing");
  }
  if (s.type === "array") {
    if (!s.items || s.maxItems < 1 || s.maxItems > 1000)
      fail(path, "Array must have bounded items");
    schema(s.items, path + "/items", depth + 1);
  }
}
export function validateManifest(m) {
  keys(
    m,
    [
      "manifest_version",
      "app_key",
      "name",
      "version",
      "entry",
      "routing",
      "sdk_protocol",
      "required_capabilities",
      "optional_capabilities",
      "config_schema",
      "binding_schema",
      "files",
    ],
    "/manifest",
  );
  if (m.manifest_version !== 1 || m.sdk_protocol?.major !== 1)
    fail("/sdk_protocol", "Unsupported version");
  keys(m.sdk_protocol, ["major"], "/sdk_protocol");
  if (
    !/^[A-Za-z][\w.-]{0,79}$/.test(m.app_key) ||
    typeof m.version !== "string" ||
    !m.version ||
    m.version.length > 64 ||
    !safePath(m.entry) ||
    extname(m.entry) !== ".html"
  )
    fail("/entry", "Invalid app or entry");
  keys(m.routing, ["mode", "fallback"], "/routing");
  if (
    !["hash", "history"].includes(m.routing.mode) ||
    (m.routing.fallback ?? []).some((p) => !safePath(p))
  )
    fail("/routing", "Invalid routing");
  for (const set of [
    m.required_capabilities ?? {},
    m.optional_capabilities ?? {},
  ])
    for (const [c, v] of Object.entries(set))
      if (!caps.has(c) || v !== 1)
        fail("/capabilities", "Unknown capability or version");
  schema(m.config_schema, "/config_schema");
  if (m.config_schema.type !== "object")
    fail("/config_schema", "Root must be object");
  for (const [name, b] of Object.entries(m.binding_schema ?? {})) {
    if (!/^[A-Za-z][\w.-]{0,79}$/.test(name))
      fail("/binding_schema", "Invalid name");
    keys(
      b,
      [
        "kind",
        "required",
        "min_items",
        "max_items",
        "value_kind",
        "source_kind",
        "metric_version_id",
        "unit_dimension",
        "observation_direction",
        "reference_basis",
      ],
      "/binding_schema/" + name,
    );
    if (
      !["channels", "resources", "spatial", "artifacts"].includes(b.kind) ||
      b.min_items < 0 ||
      b.max_items > 500
    )
      fail("/binding_schema/" + name, "Invalid binding");
  }
  if (!Array.isArray(m.files) || m.files.length > 2048 || !m.files.length)
    fail("/files", "File list required");
  const seen = new Set();
  let sum = 0;
  for (const file of m.files) {
    keys(file, ["path", "size", "media_type", "sha256"], "/files");
    if (
      !safePath(file.path) ||
      seen.has(file.path.toLowerCase()) ||
      file.path === "manifest.json" ||
      !Number.isSafeInteger(file.size) ||
      file.size < 0 ||
      file.size > 32 * 1024 ** 2 ||
      !/^[0-9a-f]{64}$/.test(file.sha256) ||
      (!media[extname(file.path).toLowerCase()] || media[extname(file.path).toLowerCase()] !== file.media_type)
    )
      fail("/files/" + file.path, "Invalid file");
    seen.add(file.path.toLowerCase());
    sum += file.size;
  }
  if (sum > 256 * 1024 ** 2 || !m.files.some((f) => f.path === m.entry))
    fail("/files", "Entry missing or package too large");
}
async function directory(dir) {
  const root = await realpath(dir),
    files = new Map();
  async function walk(path) {
    for (const entry of await readdir(path, { withFileTypes: true })) {
      const p = resolve(path, entry.name),
        rel = relative(root, p).split("\\").join("/");
      const stat = await lstat(p);
      if (stat.isSymbolicLink() || !safePath(rel)) fail(rel, "Unsafe path");
      if (stat.isDirectory()) {
        await walk(p);
        continue;
      }
      if (!stat.isFile() || stat.size > 32 * 1024 ** 2)
        fail(rel, "Invalid file");
      if (files.size >= 2048) fail(rel, "File budget exceeded");
      files.set(rel, await readFile(p));
    }
  }
  await walk(root);
  return files;
}
const crcTable = Array.from({ length: 256 }, (_, n) => {
  for (let k = 0; k < 8; k++) n = n & 1 ? 0xedb88320 ^ (n >>> 1) : n >>> 1;
  return n >>> 0;
});
function crc(b) {
  let c = 0xffffffff;
  for (const n of b) c = crcTable[(c ^ n) & 255] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}
function zip(files) {
  const local = [],
    central = [];
  let offset = 0;
  for (const [path, data] of files) {
    const name = Buffer.from(path),
      header = Buffer.alloc(30);
    header.writeUInt32LE(0x04034b50);
    header.writeUInt16LE(20, 4);
    header.writeUInt16LE(0x800, 6);
    header.writeUInt32LE(crc(data), 14);
    header.writeUInt32LE(data.length, 18);
    header.writeUInt32LE(data.length, 22);
    header.writeUInt16LE(name.length, 26);
    local.push(header, name, data);
    const c = Buffer.alloc(46);
    c.writeUInt32LE(0x02014b50);
    c.writeUInt16LE(20, 4);
    header.copy(c, 6, 4, 30);
    c.writeUInt32LE(offset, 42);
    central.push(c, name);
    offset += header.length + name.length + data.length;
  }
  const directory = Buffer.concat(central),
    end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50);
  end.writeUInt16LE(files.size, 8);
  end.writeUInt16LE(files.size, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...local, directory, end]);
}
function unzip(buf) {
  if (buf.length > 128 * 1024 ** 2) fail("/", "ZIP too large");
  let end = -1;
  for (let i = buf.length - 22; i >= Math.max(0, buf.length - 65557); i--)
    if (buf.readUInt32LE(i) === 0x06054b50) {
      end = i;
      break;
    }
  if (end < 0) fail("/", "ZIP directory missing");
  const count = buf.readUInt16LE(end + 10);
  if (
    count > 2049 ||
    buf.readUInt16LE(end + 4) !== 0 ||
    buf.readUInt16LE(end + 6) !== 0
  )
    fail("/", "ZIP layout unsupported");
  let p = buf.readUInt32LE(end + 16),
    total = 0;
  const files = new Map();
  for (let i = 0; i < count; i++) {
    if (p + 46 > end || buf.readUInt32LE(p) !== 0x02014b50)
      fail("/", "Invalid directory");
    const flags = buf.readUInt16LE(p + 8),
      method = buf.readUInt16LE(p + 10),
      compressed = buf.readUInt32LE(p + 20),
      size = buf.readUInt32LE(p + 24),
      nl = buf.readUInt16LE(p + 28),
      extra = buf.readUInt16LE(p + 30),
      comment = buf.readUInt16LE(p + 32),
      mode = buf.readUInt32LE(p + 38) >>> 16,
      offset = buf.readUInt32LE(p + 42),
      name = buf.subarray(p + 46, p + 46 + nl).toString("utf8");
    total += size;
    if (
      !safePath(name) ||
      files.has(name) ||
      flags & 1 ||
      (mode && (mode & 0xf000) !== 0x8000) ||
      ![0, 8].includes(method) ||
      size > 32 * 1024 ** 2 ||
      total > 256 * 1024 ** 2 ||
      size > Math.max(1, compressed) * 200 ||
      offset + 30 > buf.length ||
      buf.readUInt32LE(offset) !== 0x04034b50
    )
      fail(name, "Unsafe ZIP entry");
    const start =
      offset +
      30 +
      buf.readUInt16LE(offset + 26) +
      buf.readUInt16LE(offset + 28);
    if (start + compressed > p) fail(name, "Invalid ZIP range");
    const raw = buf.subarray(start, start + compressed),
      data =
        method === 8
          ? inflateRawSync(raw, { maxOutputLength: 32 * 1024 ** 2 })
          : raw;
    if (data.length !== size || crc(data) !== buf.readUInt32LE(p + 16))
      fail(name, "ZIP integrity mismatch");
    files.set(name, data);
    p += 46 + nl + extra + comment;
  }
  return files;
}
function checkFiles(m, files) {
  validateManifest(m);
  if (files.size !== m.files.length)
    fail("/files", "Undeclared or missing files");
  for (const f of m.files) {
    const b = files.get(f.path);
    if (!b || b.length !== f.size || digest(b) !== f.sha256)
      fail(f.path, "Hash or length mismatch");
    if (
      [".html", ".css"].includes(extname(f.path)) &&
      /(?:\b(?:src|href)\s*=\s*["']\s*(?:\/|https?:|javascript:)|url\(\s*["']?\s*(?:\/|https?:))/i.test(
        b.toString(),
      )
    )
      fail(f.path, "Use only package-relative dependencies");
  }
}
export async function validate(input, manifestPath) {
  let files, manifest;
  const stat = await lstat(input);
  if (stat.isDirectory()) {
    files = await directory(input);
    manifest = strictJSON(
      await readFile(manifestPath ?? resolve(input, "manifest.json"), "utf8"),
    );
    files.delete("manifest.json");
    manifest.files = [...files].map(([path, b]) => ({
      path,
      size: b.length,
      sha256: digest(b),
      media_type: media[extname(path).toLowerCase()],
    }));
  } else {
    files = unzip(await readFile(input));
    const raw = files.get("manifest.json");
    if (!raw || raw.length > 1024 ** 2) fail("/manifest", "Missing manifest");
    manifest = strictJSON(raw.toString());
    files.delete("manifest.json");
  }
  checkFiles(manifest, files);
  return { manifest, files };
}
export async function pack(directory, manifestPath, out) {
  const { manifest, files } = await validate(directory, manifestPath);
  files.set("manifest.json", Buffer.from(JSON.stringify(manifest)));
  const data = zip(files);
  if (data.length > 128 * 1024 ** 2) fail("/", "ZIP too large");
  await writeFile(out, data);
  return {
    size: data.length,
    sha256: digest(data),
    files: manifest.files.length,
  };
}
