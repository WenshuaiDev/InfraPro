import { request } from "@playwright/test";
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import assert from "node:assert/strict";
const runtime = process.env.HOME + "/.local/share/infrapro/runtime";
const state = JSON.parse(
  readFileSync(runtime + "/engineering/spatial-engineering-v2.json", "utf8"),
);
const source = JSON.parse(
  readFileSync("samples/spatial/engineering/manifest.json", "utf8"),
);
const account = JSON.parse(readFileSync(runtime + "/account.json", "utf8"));
const c = await request.newContext({ baseURL: state.origin, timeout: 30000 });
const csrf = await (await c.get("/api/v1/auth/csrf")).json();
const headers = { Origin: state.origin, "X-CSRF-Token": csrf.token };
await c.post("/api/v1/auth/login", {
  headers,
  data: { username: account.username, password: account.password },
});
const timings = [];
async function get(path) {
  const start = performance.now();
  const r = await c.get("/api/v1" + path);
  assert.ok(r.ok(), `${path}: ${r.status()}`);
  const data = await r.json();
  timings.push({ path, ms: Math.round(performance.now() - start) });
  return data;
}
async function all(path) {
  const items = [];
  let cursor = "";
  do {
    const page = await get(
      path +
        (path.includes("?") ? "&" : "?") +
        "limit=100" +
        (cursor ? "&cursor=" + encodeURIComponent(cursor) : ""),
    );
    items.push(...page.items);
    cursor = page.next_cursor;
  } while (cursor);
  return items;
}
const base = `/tenants/${state.tenant}`,
  spatial = base + "/spatial";
const report = {
  status: "passed",
  checked_at: new Date().toISOString(),
  facilities: [],
  timings,
};
try {
  for (const s of source.facilities) {
    const f = state.facilities[s.key],
      result = {
        name: s.name,
        id: f.id,
        components: Object.keys(f.components).length,
        points: Object.keys(f.points).length,
        assets: [],
      };
    const detail = await get(
      base + "/engineering/resources/facilities/" + f.id,
    );
    assert.equal(detail.name, s.name);
    for (const ext of ["pdf", "geojson", "ifc", "glb"]) {
      const a = f.assets[ext],
        asset = await get(spatial + "/assets/" + a.id),
        v = await get(
          spatial + "/versions/" + asset.current_version_id + "/manifest",
        );
      assert.equal(v.state, "published");
      assert.equal(v.source.sha256, s.files[ext].sha256);
      assert.equal(v.manifest.contract, "infrapro.spatial/1");
      const placements = await all(
          spatial + "/versions/" + v.id + "/placements",
        ),
        links = await all(spatial + "/versions/" + v.id + "/component-links");
      assert.ok(placements.length >= a.counts.placements);
      assert.ok(links.length >= a.counts.links);
      assert.ok(placements.every((p) => p.object?.facility_id === f.id));
      assert.ok(links.every((p) => p.object?.facility_id === f.id));
      if (ext === "pdf") {
        assert.equal(v.manifest.pages.length, 3);
        for (const pg of s.pages)
          for (const p of pg.points) {
            const actual = placements.find(
              (x) =>
                x.point_id === f.points[p.code] &&
                x.page_id === v.manifest.pages[pg.number - 1].id,
            );
            assert.deepEqual(actual?.anchor, p.anchor);
          }
      }
      if (ext === "glb" || ext === "ifc") {
        const elems = await all(spatial + "/versions/" + v.id + "/elements");
        assert.equal(elems.length, a.counts.elements);
        for (const p of s.points) {
          const actual = placements.find(
            (x) => x.point_id === f.points[p.code],
          );
          assert.deepEqual(actual?.anchor, p.xyz);
          assert.equal(
            actual.element_key,
            s.parts.find((x) => x.key === p.part).id,
          );
        }
        if (ext === "ifc")
          assert.ok(
            elems.some(
              (e) => e.type.startsWith("Ifc") && e.properties.GlobalId,
            ),
          );
      }
      if (ext === "glb" && f.acceptance_point) {
        const extra = placements.find((p) => p.point_id === f.acceptance_point);
        assert.deepEqual(extra?.anchor, [-20, 11.4, -6.4]);
        assert.ok(extra?.element_key);
        result.browser_created_binding = true;
      }
      if (ext === "geojson") {
        const features = await all(spatial + "/versions/" + v.id + "/features");
        assert.equal(features.length, s.stats.features);
      }
      result.assets.push({
        format: ext,
        version: v.number,
        pages: v.manifest.pages.length,
        placements: placements.length,
        links: links.length,
        source_sha256: v.source.sha256,
      });
    }
    const first = s.points[0];
    const point = await get(
      base +
        "/engineering/resources/facilities/" +
        f.id +
        "/points/" +
        f.points[first.code],
    );
    assert.ok(point.ancestors.some((x) => x.kind === "component"));
    result.ancestor_depth = point.ancestors.length;
    report.facilities.push(result);
    console.log(s.key + " verified");
  }
  report.maximum_read_ms = Math.max(...timings.map((t) => t.ms));
  report.median_read_ms = timings.map((t) => t.ms).sort((a, b) => a - b)[
    Math.floor(timings.length / 2)
  ];
  mkdirSync(".artifacts/spatial-engineering", { recursive: true });
  writeFileSync(
    ".artifacts/spatial-engineering/api-verification.json",
    JSON.stringify(report, null, 2),
  );
  console.log(
    JSON.stringify({
      status: report.status,
      facilities: report.facilities.length,
      reads: timings.length,
      maximum_read_ms: report.maximum_read_ms,
      median_read_ms: report.median_read_ms,
    }),
  );
} finally {
  await c.dispose();
}
