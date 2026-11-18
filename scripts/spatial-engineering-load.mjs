import { requireAuthenticatedLogin } from './login-result.mjs';
// Add self-authored engineering packages to the explicitly authorized local runtime.
// No database bootstrap, deletion, grants or credential output. Re-entrant by recorded IDs.
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { join, resolve } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { request } from "@playwright/test";
const origin =
  process.env.INFRAPRO_ENGINEERING_ORIGIN || "http://localhost:25173";
if (!["http://localhost:25173", "http://127.0.0.1:25173"].includes(origin))
  throw Error(
    "Engineering loader is restricted to the named local acceptance runtime.",
  );
const tenant =
  process.env.INFRAPRO_ENGINEERING_TENANT ||
  "64a4ebf9-c549-4706-94a9-117333b62d4b";
const runtime = join(process.env.HOME, ".local/share/infrapro/runtime");
const root = resolve("samples/spatial/engineering");
const ledger = join(runtime, "engineering/spatial-engineering-v2.json");
const account = JSON.parse(readFileSync(join(runtime, "account.json"), "utf8"));
const manifest = JSON.parse(readFileSync(join(root, "manifest.json"), "utf8"));
const state = existsSync(ledger)
  ? JSON.parse(readFileSync(ledger, "utf8"))
  : { tenant, origin, facilities: {}, generator: manifest.generator };
if (state.tenant !== tenant || state.origin !== origin)
  throw Error("Ledger belongs to another scope.");
const save = () => {
  mkdirSync(join(runtime, "engineering"), { recursive: true });
  writeFileSync(ledger, JSON.stringify(state, null, 2) + "\n", { mode: 0o600 });
};
const c = await request.newContext({ baseURL: origin, timeout: 120000 });
const csrf = await (await c.get("/api/v1/auth/csrf")).json();
const headers = { Origin: origin, "X-CSRF-Token": csrf.token };
const wm = (revision) => ({
  request_id: randomUUID(),
  expected_revision: revision,
  reason: "设施树工程空间验收 / 自制仿真工程",
});
async function call(path, data, method = data ? "POST" : "GET") {
  const r = await c.fetch("/api/v1" + path, { method, headers, data });
  if (!r.ok())
    throw Error(`${method} ${path}: ${r.status()} ${await r.text()}`);
  return r.status() === 204 ? undefined : r.json();
}
async function waitJob(id) {
  const until = Date.now() + 240000;
  while (Date.now() < until) {
    const j = await call(spatial + "/jobs/" + id);
    if (j.state === "succeeded") return j;
    if (["failed", "cancelled"].includes(j.state))
      throw Error("Processing " + JSON.stringify(j));
    await new Promise((r) => setTimeout(r, 1200));
  }
  throw Error("Processing timeout " + id);
}
const resources = `/tenants/${tenant}/engineering/resources/facilities`,
  spatial = `/tenants/${tenant}/spatial`,
  catalog = "/platform/engineering/type-definitions";
function position(s, xyz) {
  return {
    description: "仿真工程统一坐标；不是实测坐标",
    source_reference: manifest.generator,
    geographic: {
      crs: "EPSG:4326",
      longitude:
        s.origin[0] +
        xyz[0] / (111320 * Math.cos((s.origin[1] * Math.PI) / 180)),
      latitude: s.origin[1] - xyz[2] / 110540,
    },
    local: {
      reference_description: s.name + "模型局部原点",
      axes_description: "X 沿线路，Y 竖直向上，Z 横向；右手坐标系",
      unit: "m",
      x: xyz[0],
      y: xyz[1],
      z: xyz[2],
    },
  };
}
try {
  requireAuthenticatedLogin(await call("/auth/login", {
    username: account.username,
    password: account.password,
  }));
  for (const s of manifest.facilities) {
    const f = (state.facilities[s.key] ??= {
      components: {},
      points: {},
      assets: {},
      name: s.name,
    });
    save();
    if (!f.type) {
      const code = "spatial_engineering_" + s.key + "_v2";
      const existing = await call(catalog + "?q=" + code + "&limit=100");
      let d = existing.items.find((x) => x.code === code);
      if (!d) {
        const content = {
          metamodel_version: "1",
          name: s.summary,
          description: "自制仿真工程完整层级，用于工程空间验收",
          release_notes: manifest.generator,
          fields: [],
          components: s.components.map((x) => ({
            key: x.key.toLowerCase().replaceAll("-", "_"),
            name: x.name,
            description: "工程层级 / " + x.category,
            fields: [],
          })),
          relations: s.components.map((x, i) => ({
            parent: (x.parent || "root").toLowerCase().replaceAll("-", "_"),
            child: x.key.toLowerCase().replaceAll("-", "_"),
            min_count: 0,
            max_count: 50,
            order: i,
          })),
        };
        const a = await call(catalog, {
          request_id: randomUUID(),
          code,
          content,
          reason: "工程空间完整设施层级验收",
        });
        d = await call(catalog + "/" + a.definition_id);
      } else d = await call(catalog + "/" + d.id);
      if (!d.versions.length) {
        const pub = await call(catalog + "/" + d.id + "/publish", {
          request_id: randomUUID(),
          revision: d.revision,
          reason: "发布仿真工程层级定义",
        });
        f.type = pub.version_id;
      } else f.type = d.versions.at(-1).id;
      d = await call(catalog + "/" + d.id);
      const v = d.versions.find((v) => v.id === f.type);
      if (!v.enabled)
        await call(catalog + "/" + d.id + "/versions/" + v.id + "/state", {
          request_id: randomUUID(),
          revision: v.state_revision,
          enabled: true,
          reason: "本地空间交互验收",
        });
      save();
    }
    if (!f.id) {
      const code = "ENG-" + s.key.toUpperCase() + "-2026";
      const existing = await call(resources + "?q=" + code + "&limit=100");
      f.id = existing.items.find((x) => x.code === code)?.id;
      if (!f.id)
        f.id = (
          await call(resources, {
            request_id: randomUUID(),
            code,
            name: s.name,
            type_version_id: f.type,
            values: {},
            description:
              s.summary +
              "；按真实构造自行设计的仿真工程；未经施工或安全验算。",
            source_reference: manifest.generator,
            position: position(s, [0, 0, 0]),
          })
        ).id;
      save();
    }
    for (const comp of s.components) {
      if (f.components[comp.key]) continue;
      const code = "ENG-" + comp.key;
      const existing = await call(
        resources +
          "/" +
          f.id +
          "/components?q=" +
          encodeURIComponent(code) +
          "&limit=100",
      );
      let id = existing.items.find((x) => x.code === code)?.id;
      if (!id) {
        const d = await call(resources + "/" + f.id);
        id = (
          await call(resources + "/" + f.id + "/components", {
            request_id: randomUUID(),
            expected_structure_revision: d.structure_revision,
            code,
            name: comp.name,
            category_key: comp.key.toLowerCase().replaceAll("-", "_"),
            parent_component_id: comp.parent
              ? f.components[comp.parent]
              : undefined,
            values: {},
            description: "空间资料对应实体组 / " + comp.category,
          })
        ).id;
      }
      f.components[comp.key] = id;
      save();
    }
    for (const p of s.points) {
      if (f.points[p.code]) continue;
      const existing = await call(
        resources +
          "/" +
          f.id +
          "/points?q=" +
          encodeURIComponent(p.code) +
          "&limit=100",
      );
      let id = existing.items.find((x) => x.code === p.code)?.id;
      if (!id)
        id = (
          await call(resources + "/" + f.id + "/points", {
            request_id: randomUUID(),
            code: p.code,
            name: p.name + " " + p.code,
            component_id: f.components[p.component],
            purpose: "空间交互验收；未接真实传感器",
            source_reference: manifest.generator,
            position: position(s, p.xyz),
          })
        ).id;
      f.points[p.code] = id;
      save();
    }
    for (const ext of ["pdf", "geojson", "ifc", "glb"]) {
      const file = s.files[ext],
        kind =
          ext === "pdf" ? "layout" : ext === "geojson" ? "geolayer" : "model";
      let a = (f.assets[ext] ??= {});
      if (a.published && a.sha256 === file.sha256) {
        console.log(`${s.key} ${ext}: 已发布，跳过`);
        continue;
      }
      if (!a.id) {
        const code =
          "ENG-" + s.key.toUpperCase() + "-" + ext.toUpperCase() + "-V2";
        const existing = await call(
          spatial + "/assets?q=" + code + "&limit=100",
        );
        a.id = existing.items.find((x) => x.code === code)?.id;
        if (!a.id)
          a.id = (
            await call(spatial + "/assets", {
              request_id: randomUUID(),
              kind,
              facility_id: f.id,
              code,
              name:
                s.name +
                " · " +
                {
                  pdf: "综合布设与构造图",
                  geojson: "工程地理要素",
                  ifc: "结构 BIM（IFC4）",
                  glb: "精细三维模型（GLB）",
                }[ext],
              category: ext === "pdf" ? "plan" : "general",
              source_note:
                "自行设计的仿真工程；CC0-1.0；米制坐标；" + s.summary,
            })
          ).id;
        save();
      }
      if (a.sha256 !== file.sha256) {
        a.version = undefined;
        a.published = false;
        a.sha256 = file.sha256;
        save();
      }
      if (!a.version) {
        const d = await call(spatial + "/assets/" + a.id);
        const v = await call(spatial + "/assets/" + a.id + "/versions", {
          ...wm(d.revision),
          new_source: true,
          change_note: "完整工程几何与监测布设 / " + manifest.generator,
        });
        a.version = v.version_id || v.id;
        save();
      }
      let v = await call(spatial + "/versions/" + a.version + "/manifest");
      if (!v.manifest_hash) {
        if (!v.source) {
          const bytes = readFileSync(join(root, file.name));
          const up = await call(spatial + "/uploads", {
            ...wm(v.revision),
            version_id: a.version,
            filename: file.name,
            size: bytes.length,
            sha256: file.sha256,
          });
          let u = await call(spatial + "/uploads/" + up.id);
          for (
            let offset = 0, part = 1;
            offset < bytes.length;
            offset += u.part_size, part++
          ) {
            const data = bytes.subarray(offset, offset + u.part_size);
            const r = await c.put(
              "/api/v1" + spatial + `/uploads/${u.id}/parts/${part}`,
              {
                headers: {
                  ...headers,
                  "Content-Type": "application/octet-stream",
                  "X-Content-SHA256": createHash("sha256")
                    .update(data)
                    .digest("hex"),
                },
                data,
              },
            );
            if (!r.ok()) throw Error("Upload failed " + (await r.text()));
          }
          u = await call(spatial + "/uploads/" + u.id);
          await call(
            spatial + "/uploads/" + u.id + "/complete",
            wm(u.revision),
          );
        }
        v = await call(spatial + "/versions/" + a.version + "/manifest");
        const job = await call(
          spatial + "/versions/" + a.version + "/process",
          wm(v.revision),
        );
        await waitJob(job.id);
      }
      let draft = await call(spatial + "/versions/" + a.version + "/draft");
      if (draft.version.state === "published") {
        a.published = true;
        save();
        continue;
      }
      const m = draft.version.manifest,
        placements = [],
        component_links = [],
        partIDs = new Map(s.parts.map((p) => [p.key, p.id]));
      if (ext === "pdf")
        for (const page of s.pages) {
          const target = m.pages.find((p) => p.number === page.number);
          for (const p of page.points)
            placements.push({
              id: "",
              point_id: f.points[p.code],
              page_id: target.id,
              anchor: p.anchor,
              element_key: "",
              label_offset: [12, -18],
              basis_hash: "",
              basis_changed: false,
            });
          for (const r of page.regions)
            component_links.push({
              id: "",
              component_id: f.components[r.component],
              page_id: target.id,
              element_key: "",
              region: r.region,
              basis_hash: "",
              basis_changed: false,
            });
        }
      if (kind === "model") {
        const elementKeys = new Set(m.elements.map((e) => e.key));
        for (const p of s.points) {
          const key = partIDs.get(p.part);
          if (!elementKeys.has(key))
            throw Error("Missing model target " + p.part);
          placements.push({
            id: "",
            point_id: f.points[p.code],
            page_id: "",
            anchor: p.xyz,
            element_key: key,
            label_offset: [12, -18],
            basis_hash: "",
            basis_changed: false,
          });
        }
        for (const p of s.parts)
          if (p.component) {
            if (!elementKeys.has(p.id))
              throw Error("Missing model element " + p.key);
            component_links.push({
              id: "",
              component_id: f.components[p.component],
              page_id: "",
              element_key: p.id,
              region: [],
              basis_hash: "",
              basis_changed: false,
            });
          }
      }
      const camera = {
        bridge: { position: [105, 65, 95], target: [0, 3, 0] },
        tunnel: { position: [-110, 60, 90], target: [-8, 2, 0] },
        slope: { position: [85, 60, -75], target: [0, 10, 10] },
        road: { position: [100, 65, 90], target: [0, 0, 0] },
      }[s.key];
      await call(
        spatial + "/versions/" + a.version + "/draft",
        {
          ...wm(draft.version.revision),
          placements,
          component_links,
          feature_links: m.features.map((feature) => ({
            feature_key: feature.id,
            facility_id: f.id,
          })),
          remove_placements: draft.placements.map((p) => p.id),
          remove_component_links: draft.component_links.map((p) => p.id),
          settings: {
            source_properties: true,
            main_channels: {},
            page_id: m.pages[0]?.id,
            basemap: "local",
            ...(kind === "model" ? { camera } : {}),
          },
          clear_candidates: true,
        },
        "PATCH",
      );
      v = await call(spatial + "/versions/" + a.version + "/manifest");
      const input = wm(v.revision);
      const preview = await call(
        spatial + "/versions/" + a.version + "/publish-preview",
        input,
      );
      if (!preview.can_submit)
        throw Error("Publish blocked " + JSON.stringify(preview));
      await call(spatial + "/versions/" + a.version + "/publish", {
        ...input,
        preview_hash: preview.preview_hash,
      });
      const currentAsset = await call(spatial + "/assets/" + a.id);
      if (currentAsset.current_version_id !== a.version) {
        const input = { ...wm(currentAsset.revision), version_id: a.version };
        const preview = await call(
          spatial + "/assets/" + a.id + "/current-preview",
          input,
        );
        await call(spatial + "/assets/" + a.id + "/current", {
          ...input,
          preview_hash: preview.preview_hash,
        });
      }
      a.published = true;
      a.counts = {
        placements: placements.length,
        links: component_links.length,
        elements: m.elements.length,
        pages: m.pages.length,
        features: m.features.length,
      };
      a.url = `${origin}/t/${tenant}/spatial?facility=${f.id}&owner_kind=facility&owner=${f.id}&asset=${a.id}&section=${kind === "layout" ? "layouts" : kind === "model" ? "models" : "gis"}`;
      save();
      console.log(`${s.name} ${ext}: 已发布 ${JSON.stringify(a.counts)}`);
    }
  }
  console.log("工程资料导入完成；台账 " + ledger);
} finally {
  await c.dispose();
}
