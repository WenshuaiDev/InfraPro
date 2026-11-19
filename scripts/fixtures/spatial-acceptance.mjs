import { requireAuthenticatedLogin } from '../login-result.mjs';
import { seedSpatialConnections, grantSpatialDeviceOutputs } from './spatial-connections.mjs';
import { createHash, randomUUID } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { request } from "@playwright/test";
// All business objects, original uploads, publications and grants use production HTTP APIs.
export async function seedSpatialAcceptance(directory, origin, options = {}) {
  if (!(options.paired?.origin===origin && new URL(origin).hostname==='127.0.0.1') && !["http://127.0.0.1:15473","http://127.0.0.1:15573","http://127.0.0.1:15773","http://127.0.0.1:15576"].includes(origin))
    throw Error("Spatial samples require their isolated loopback laboratory.");
  const prior = JSON.parse(
      readFileSync(join(directory, "acceptance.json"), "utf8"),
    ),
    login = JSON.parse(readFileSync(join(directory, "login.json"), "utf8")),
    client = options.paired ? await options.paired.client() : await request.newContext(),
    api = origin + "/api/v1";
  const wm = (revision = "0") => ({
      request_id: randomUUID(),
      expected_revision: revision,
      reason: "工程空间独立合成验收样本",
    }),
    state = {
      synthetic: true,
      tenant: prior.tenant,
      facility: prior.facility,
      point: prior.point,
      second_point: prior.second_point,
      channels: prior.channels,
      assets: {},
      projects: {},
      expected: {
        directA: "20",
        directB: "11",
        derivedAminusB: "9",
        geographic: [116.38, 39.9],
      },
      startedAt: new Date().toISOString(),
    };
  const save = () =>
    writeFileSync(
      join(directory, "spatial-acceptance.json"),
      JSON.stringify(state, null, 2),
      { mode: 0o600 },
    );
  const csrf = await (await client.get(api + "/auth/csrf")).json(),
    headers = { Origin: origin, "X-CSRF-Token": csrf.token };
  const call = async (
    path,
    data,
    method = data === undefined ? "GET" : "POST",
  ) => {
    const r = await client.fetch(api + path, { method, headers, data });
    if (![200, 201, 202, 204].includes(r.status()))
      throw Error(`${method} ${path}: ${r.status()} ${await r.text()}`);
    return r.status() === 204 ? undefined : r.json();
  };
  const wait = async (path, pick, want) => {
    if(options.paired) return client.wait(api+path,headers,pick,want);
    const end = Date.now() + 120000;
    while (Date.now() < end) {
      const value = await call(path),
        status = pick(value);
      if (status === want) return value;
      if (["failed", "canceled", "needs_action"].includes(status))
        throw Error(`Sample task failed: ${JSON.stringify(value)}`);
      await new Promise((r) => setTimeout(r, 300));
    }
    throw Error("Sample task timed out");
  };
  try {
    requireAuthenticatedLogin(await call("/auth/login", {
      username: login.username,
      password: login.password,
    }));
    const base = `/tenants/${prior.tenant}`,
      spatial = base + "/spatial",
      resources = base + "/engineering/resources/facilities",
      facilityPath = resources + "/" + prior.facility;
    const position = {
      geographic: { crs: "EPSG:4326", longitude: 116.38, latitude: 39.9 },
      source_reference: "InfraPro synthetic CC0; no surveyed coordinates",
    };
    const facility = await call(facilityPath);
    await call(
      facilityPath,
      {
        request_id: randomUUID(),
        expected_revision: facility.revision,
        name: "示例桥梁 · 非真实工程",
        position,
      },
      "PATCH",
    );
    for (const [id, name, lon, lat] of [
      [prior.point, "主梁跨中位移点", 116.3802, 39.9001],
      [prior.second_point, "桥墩沉降点", 116.3805, 39.9002],
    ]) {
      const path = facilityPath + "/points/" + id,
        current = await call(path);
      await call(
        path,
        {
          request_id: randomUUID(),
          expected_revision: current.revision,
          name,
          position: {
            ...position,
            geographic: { crs: "EPSG:4326", longitude: lon, latitude: lat },
          },
        },
        "PATCH",
      );
    }
    const component = await call(facilityPath + "/components", {
      request_id: randomUUID(),
      code: "MAIN-BEAM",
      name: "示例主梁",
      expected_structure_revision: facility.structure_revision,
      category_key: "beam",
      values: {},
    });
    state.component = component.id;
    const noLocation = await call(facilityPath + "/points", {
      request_id: randomUUID(),
      code: "NO-LOCATION",
      name: "仅有文字位置的测点",
      position: { description: "右幅 K0+120；无地理坐标" },
    });
    state.unlocated = noLocation.id;
    const slope = await call(resources, {
      request_id: randomUUID(),
      code: "SLOPE",
      name: "示例边坡 · 非真实工程",
      type_version_id: facility.type_version_id,
      values: {},
      position: {
        ...position,
        geographic: { crs: "EPSG:4326", longitude: 116.385, latitude: 39.904 },
      },
    });
    state.slope = slope.id;
    const slopePoint = await call(resources + "/" + slope.id + "/points", {
      request_id: randomUUID(),
      code: "SLOPE-P1",
      name: "坡面示例测点",
      position: {
        geographic: {
          crs: "EPSG:4326",
          longitude: 116.3852,
          latitude: 39.9042,
        },
      },
    });
    state.slope_point = slopePoint.id;
    await seedSpatialConnections({call,wait,base,prior,state,component:component.id,save});
    const {createAsset,publish}=spatialAssetWriter({state,prior,component,slope,spatial,call,wait,wm,client,api,headers,origin,save});
    for (const [key, kind, fid, name, files, point] of [
      [
        "BRIDGE-PDF",
        "layout",
        prior.facility,
        "桥梁多页布设图",
        ["bridge-layouts-v1.pdf", "bridge-layouts-v2.pdf"],
        prior.point,
      ],
      [
        "BRIDGE-PNG",
        "layout",
        prior.facility,
        "桥梁平面图 PNG",
        ["bridge-plan.png"],
        prior.point,
      ],
      [
        "BRIDGE-JPEG",
        "layout",
        prior.facility,
        "桥梁立面图 JPEG",
        ["bridge-elevation.jpg"],
        prior.point,
      ],
      [
        "BRIDGE-GLB",
        "model",
        prior.facility,
        "桥梁轻量模型",
        ["bridge-v1.glb", "bridge-v2.glb"],
        prior.point,
      ],
      [
        "BRIDGE-GLB-MM",
        "model",
        prior.facility,
        "毫米与 Z 轴转换样本",
        ["bridge-mm-z-up.glb"],
        prior.point,
      ],
      [
        "BRIDGE-IFC2X3",
        "model",
        prior.facility,
        "桥梁 IFC2X3 模型",
        ["bridge-ifc2x3-v1.ifc"],
        prior.point,
      ],
      [
        "BRIDGE-IFC4",
        "model",
        prior.facility,
        "桥梁 IFC4 模型",
        ["bridge-ifc4-v1.ifc", "bridge-ifc4-v2.ifc"],
        prior.point,
      ],
      [
        "SLOPE-PDF",
        "layout",
        slope.id,
        "边坡监测剖面",
        ["slope-layouts-v1.pdf", "slope-layouts-v2.pdf"],
        slopePoint.id,
      ],
      [
        "SLOPE-GLB",
        "model",
        slope.id,
        "边坡轻量模型",
        ["slope-v1.glb"],
        slopePoint.id,
      ],
      [
        "SLOPE-IFC4",
        "model",
        slope.id,
        "边坡 IFC4 三角化模型",
        ["slope-ifc4-v1.ifc"],
        slopePoint.id,
      ],
      [
        "REGION",
        "geolayer",
        undefined,
        "区域线面示意 GeoJSON",
        ["reference.geojson", "reference-v2.geojson"],
        undefined,
      ],
    ]) {
      if (options.minimal && !["BRIDGE-PDF", "BRIDGE-GLB", "REGION"].includes(key)) continue;
      await createAsset(key, kind, fid, name);
      let previous;
      for (const file of files)
        previous = await publish(key, file, fid, point, previous);
    }
    // Two independent projects share the same immutable originals with different business grants.
    const membership = (await call(base + "/context")).membership_id;
    for (const key of ["A", "B"]) {
      const project = await call(base + "/projects", {
          name: `空间示例项目 ${key}`,
          code: `SPATIAL-${key}`,
          purpose: "合成权限边界验收",
          description: "固定资料版本与业务、原件、数据分别授权",
          owner_tenant_membership_id: membership,
          administrator_tenant_membership_id: membership,
          client_request_id: randomUUID(),
          reason: "空间合成演示",
        }),
        pid = project.project_id,
        pb = base + "/projects/" + pid;
      state.projects[key] = {
        id: pid,
        url: `${origin}/t/${prior.tenant}/p/${pid}/spatial`,
      };
      const path = pb + "/resource-access/facilities/" + prior.facility,
        current = await call(path),
        add = [
          { kind: "facility", capabilities: ["read"] },
          {
            kind: "channel",
            channel_id: prior.channels.A.id,
            capabilities: ["read"],
          },
        ];
      if (key === "A")
        add.push(
          {
            kind: "component",
            component_id: component.id,
            capabilities: ["read"],
          },
          { kind: "point", point_id: prior.point, capabilities: ["read"] },
          {
            kind: "point",
            point_id: prior.second_point,
            capabilities: ["read"],
          },
          ...Object.values(prior.channels)
            .filter((c) => c.id !== prior.channels.A.id)
            .map((c) => ({
              kind: "channel",
              channel_id: c.id,
              capabilities: ["read"],
            })),
        );
      const input = {
          request_id: randomUUID(),
          reason: "空间示例资源范围",
          action: "create_link",
          expected_access_revision: current.policy.revision,
          add,
        },
        preview = await call(path + "/preview", input);
      await call(path + "/changes", {
        ...input,
        preview_hash: preview.preview_hash,
      });
      const sg = pb + "/spatial-access",
        g = await call(sg),
        changes = {
          ...wm(g.revision),
          add: Object.entries(state.assets)
            .filter(([key]) => key.startsWith("BRIDGE"))
            .map(([, a]) => ({
              version_id: a.versions[0].id,
              source_download: false,
              is_default: true,
            })),
          end: [],
        },
        gp = await call(sg + "/preview", changes);
      await call(sg, { ...changes, preview_hash: gp.preview_hash }, "PUT");
      const resource = await call(path + "?candidate_kind=channel");
      for (const grant of resource.grants.filter(
        (g) => g.kind === "channel" || g.resource_kind === "channel",
      )) {
        const mg = pb + "/monitoring-access",
          current = await call(mg);
        await call(mg, {
          ...wm(current.access_revision),
          channel_id: grant.resource_id,
          resource_grant_id: grant.id,
          range: { from: "2023-12-31T00:00:00Z", to: "2024-01-03T00:00:00Z" },
          capabilities: ["read", "history"],
        });
      }
      if(key==='A')await grantSpatialDeviceOutputs(call,base,pid,state);
      save();
    }
    // A saved analysis exists in the tenant and project A; project B cannot discover either.
    const definition = (
      await call(
        base +
          "/monitoring/derived/channels/" +
          prior.channels.A_MINUS_B.id +
          "/configs",
      )
    ).items[0];
    for (const [name, scope] of [
      ["tenant", base],
      ["projectA", base + "/projects/" + state.projects.A.id],
    ]) {
      const d = scope + "/monitoring/derived",
        config = (
          await call(d + "/analysis/configs", {
            ...wm(),
            code: "SPATIAL-ANALYSIS",
            name: "桥梁 A−B 固定分析",
            body: {
              rule: definition.rule,
              input_bindings: definition.input_bindings,
              parameters: {},
            },
          })
        ).config,
        validation = await call(
          d + "/analysis/configs/" + config.id + "/validate",
          { draft_revision: config.draft.revision },
        ),
        published = await call(
          d + "/analysis/configs/" + config.id + "/versions",
          {
            ...wm(config.revision),
            draft_revision: config.draft.revision,
            validation_hash: validation.validation_hash,
          },
        );
      const run = await call(d + "/analysis/runs", {
        ...wm(),
        config_version_id: published.version.id,
        config_hash: published.version.config_hash,
        output_range: prior.range,
      });
      const completed = await wait(
        d + "/runs/" + run.id,
        (v) => v.job.state,
        "succeeded",
      );
      state.expected[name + "_artifact"] = completed.analysis;
      save();
    }
    await call("/auth/logout", {});
    requireAuthenticatedLogin(await call("/auth/login", {
      username: "e2e_owner",
      password: "Real browser 河流 secret 592",
    }));
    const account = (
        await call("/access/accounts?q=" + login.username)
      ).items.find((a) => a.username === login.username),
      second = await call("/platform/tenants", {
        name: "空间示例 · 独立租户乙",
        code: "spatial-second",
        administrator_account_id: account.id,
        client_request_id: randomUUID(),
        reason: "跨租户边界样本",
      });
    state.second_tenant = second.tenant_id;
    await call("/auth/logout", {});
    requireAuthenticatedLogin(await call("/auth/login", {
      username: login.username,
      password: login.password,
    }));
    const forbidden = await client.get(
      api +
        `/tenants/${second.tenant_id}/spatial/versions/${state.assets["BRIDGE-PDF"].versions[0].id}/manifest`,
    );
    if (forbidden.status() !== 404) throw Error("Cross-tenant boundary failed");
    state.expected.cross_tenant_status = 404;
    const root = state.assets["BRIDGE-PDF"];
    state.url = root.versions[0].url;
    state.completedAt = new Date().toISOString();
    save();
    return {
      url: state.url,
      tenant: state.tenant,
      directory,
      projects: state.projects,
    };
  } finally {
    await client.dispose();
  }
}

// Share the normal multipart, real converter, association and publication
// workflow between browser acceptance and isolated architecture comparisons.
export function spatialAssetWriter({state,prior,component,slope,spatial,call,wait,wm,client,api,headers,origin,save,mainChannels,processVersion,beforePublish}){
  const createAsset = async (key, kind, fid, name) => {
    const a = await call(spatial + "/assets", {
      request_id: randomUUID(),
      kind,
      facility_id: fid,
      code: key,
      name,
      category: kind === "layout" ? "plan" : "general",
      source_note:
        "InfraPro 自制合成验收资料，CC0；不是实际工程、测绘或安全评价依据。",
    });
    state.assets[key] = { id: a.id, versions: [], name };
    save();
    return a.id;
  };
  const publish = async (key, filename, fid, point, baseVersion) => {
    const a = state.assets[key],
      asset = await call(spatial + "/assets/" + a.id),
      created = await call(spatial + "/assets/" + a.id + "/versions", {
        ...wm(asset.revision),
        base_version_id: baseVersion,
        new_source: true,
        change_note: filename + " · synthetic CC0",
      }),
      vid = created.version_id || created.id;
    const bytes = readFileSync(
        new URL("../../samples/spatial/" + filename, import.meta.url),
      ),
      hash = createHash("sha256").update(bytes).digest("hex");
    let v = await call(spatial + "/versions/" + vid + "/manifest");
    const started = await call(spatial + "/uploads", {
        ...wm(v.revision),
        version_id: vid,
        filename,
        size: bytes.length,
        sha256: hash,
      }),
      upload = await call(spatial + "/uploads/" + started.id);
    for (
      let offset = 0, part = 1;
      offset < bytes.length;
      offset += upload.part_size, part++
    ) {
      const data = bytes.subarray(offset, offset + upload.part_size),
        r = await client.put(
          api + spatial + `/uploads/${upload.id}/parts/${part}`,
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
      if (r.status() !== 200)
        throw Error("Spatial multipart failed: " + (await r.text()));
    }
    const uploaded = await call(spatial + "/uploads/" + upload.id);
    await call(
      spatial + "/uploads/" + upload.id + "/complete",
      wm(uploaded.revision),
    );
    v = await call(spatial + "/versions/" + vid + "/manifest");
    const job = processVersion ? await processVersion(vid,v) : await call(
      spatial + "/versions/" + vid + "/process",
      wm(v.revision),
    );
    await wait(spatial + "/jobs/" + job.id, (v) => v.state, "succeeded");
    const draft = await call(spatial + "/versions/" + vid + "/draft"),
      manifest = draft.version.manifest;
    const placements = [],
      links = [];
    if (point && manifest.pages.length) {
      for (const page of manifest.pages) {
        placements.push({
          id: "",
          point_id: point,
          page_id: page.id,
          anchor: [0.52, 0.45],
          element_key: "",
          label_offset: [12, -18],
          basis_hash: "",
          basis_changed: false,
        });
      }
      if (fid === prior.facility) {
        placements.push({
          id: "",
          point_id: prior.second_point,
          page_id: manifest.pages[0].id,
          anchor: [0.3, 0.54],
          element_key: "",
          label_offset: [12, 16],
          basis_hash: "",
          basis_changed: false,
        });
        links.push({
          id: "",
          component_id: component.id,
          page_id: manifest.pages[0].id,
          element_key: "",
          region: [
            [0.18, 0.34],
            [0.82, 0.34],
            [0.82, 0.55],
            [0.18, 0.55],
            [0.18, 0.34],
          ],
          basis_hash: "",
          basis_changed: false,
        });
      }
    }
    if (point && manifest.elements.length) {
      const beam =
        manifest.elements.find(
          (e) =>
            e.fragments.length &&
            (/beam/i.test(e.name) || e.type === "IfcBeam"),
        ) ?? manifest.elements.find((e) => e.fragments.length);
      placements.push({
        id: "",
        point_id: point,
        page_id: "",
        anchor: fid === slope.id ? [8, 3, 0] : [18, 5, 0],
        element_key: beam.key,
        label_offset: [12, -18],
        basis_hash: "",
        basis_changed: false,
      });
      if (fid === prior.facility)
        links.push({
          id: "",
          component_id: component.id,
          element_key: beam.key,
          page_id: "",
          region: [],
          basis_hash: "",
          basis_changed: false,
        });
    }
    const edit = {
      ...wm(draft.version.revision),
      placements,
      component_links: links,
      feature_links: manifest.features.map((f) => ({
        feature_key: f.id,
        facility_id: f.id.includes("slope") ? slope.id : prior.facility,
      })),
      settings: {
        source_properties: true,
        main_channels: fid===prior.facility?mainChannels??{['point:'+prior.point]:prior.channels.A.id,['facility:'+prior.facility]:prior.channels.A_MINUS_B.id,['component:'+component.id]:prior.channels.COMPONENT_DIFF.id}:{},
        page_id: manifest.pages[0]?.id,
        basemap: "local",
      },
      clear_candidates: !baseVersion,
    };
    await call(spatial + "/versions/" + vid + "/draft", edit, "PATCH");
    v = await call(spatial + "/versions/" + vid + "/manifest");
    await beforePublish?.(vid,v);
    const input = wm(v.revision),
      preview = await call(
        spatial + "/versions/" + vid + "/publish-preview",
        input,
      );
    await call(spatial + "/versions/" + vid + "/publish", {
      ...input,
      preview_hash: preview.preview_hash,
    });
    a.versions.push({
      id: vid,
      file: filename,
      sha256: hash,
      pages: manifest.pages.length,
      elements: manifest.elements.length,
      features: manifest.features.length,
      url: `${origin}/t/${prior.tenant}/spatial?asset=${a.id}&version=${vid}&owner=${point ?? fid ?? prior.facility}&owner_kind=${point ? "point" : "facility"}`,
    });
    save();
    console.log(`空间样本已发布：${filename}`);
    return vid;
  };
  return {createAsset,publish};
}
