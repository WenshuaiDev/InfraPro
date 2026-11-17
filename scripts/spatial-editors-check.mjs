import assert from "node:assert/strict";
import { chromium, request, expect as baseExpect } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
const expect = baseExpect.configure({ timeout: 20000 }),
  sessions = ".artifacts/spatial-tests",
  dir =
    process.env.SPATIAL_SESSION ??
    join(sessions, readdirSync(sessions).sort().at(-1)),
  s = JSON.parse(readFileSync(join(dir, "spatial-acceptance.json"))),
  login = JSON.parse(readFileSync(join(dir, "login.json"))),
  origin = "http://127.0.0.1:15473",
  sp = `/tenants/${s.tenant}/spatial`,
  root = `${origin}/t/${s.tenant}/spatial`,
  out = ".artifacts/spatial-redesign";
const api = await request.newContext(),
  csrf = await (await api.get(origin + "/api/v1/auth/csrf")).json(),
  headers = { Origin: origin, "X-CSRF-Token": csrf.token },
  checks = [],
  errors = [],
  drafts = [];
const wm = (revision) => ({
  request_id: randomUUID(),
  expected_revision: revision,
  reason: "空间编辑器真实交互回归",
});
async function call(path, data, method = data === undefined ? "GET" : "POST") {
  const r = await api.fetch(origin + "/api/v1" + path, {
    headers,
    data,
    method,
  });
  assert(r.ok(), `${path}: ${r.status()} ${await r.text()}`);
  return r.json();
}
await call("/auth/login", {
  username: login.username,
  password: login.password,
});
const browser = await chromium.launch({
    args: ["--use-angle=swiftshader", "--enable-webgl"],
  }),
  ctx = await browser.newContext({ viewport: { width: 1720, height: 1080 } });
await ctx.addCookies((await api.storageState()).cookies);
const page = await ctx.newPage();
page.on("pageerror", (e) => errors.push(e.message));
const button = (name) => page.getByRole("button", { name, exact: true }),
  field = (name) => page.getByLabel(name, { exact: true });
async function clone(key, newSource = false) {
  const a = s.assets[key],
    current = await call(sp + "/assets/" + a.id),
    r = await call(sp + `/assets/${a.id}/versions`, {
      ...wm(current.revision),
      base_version_id: a.versions[0].id,
      new_source: newSource,
      change_note: "浏览器编辑器回归，结束后删除",
    }),
    id = r.version_id || r.id;
  drafts.push(id);
  return { asset: a.id, id, base: a.versions[0].id };
}
async function check(name, fn) {
  await fn();
  checks.push({ name, passed: true });
  console.log("PASS " + name);
}
try {
  await check(
    "E01 unprocessed draft cannot publish; replacement candidates require explicit adoption on their page",
    async () => {
      const d = await clone("BRIDGE-PDF", true);
      await page.goto(
        root + `?mode=manage&section=assets&asset=${d.asset}&version=${d.id}`,
      );
      await button("校验草稿").click();
      await expect(page.getByRole("alert")).toContainText(
        "版本尚无完整且一致的封存处理结果",
      );
      await field("选择空间原件").setInputFiles(
        "samples/spatial/bridge-layouts-v2.pdf",
      );
      await button("上传并校验").click();
      await expect(button("开始处理原件")).toBeEnabled({ timeout: 90000 });
      await button("开始处理原件").click();
      await expect(button("编辑挂载、关联与默认视图")).toBeVisible({
        timeout: 120000,
      });
      const before = await call(sp + `/versions/${d.id}/draft`);
      assert.equal(before.placements.length, 0);
      const index = before.version.candidates.findIndex(
        (c) => c.placement?.page_id === "page-2",
      );
      assert(index >= 0);
      await button("编辑挂载、关联与默认视图").click();
      await button("载入核对").nth(index).click();
      await expect(field("编辑图页")).toHaveValue("page-2");
      await button("应用到草稿").click();
      await button("将当前图页设为默认").click();
      await button("保存草稿").click();
      await expect(button("保存草稿")).toBeDisabled();
      let saved = await call(sp + `/versions/${d.id}/draft`);
      assert.equal(saved.placements.length, 1);
      assert.equal(saved.placements[0].page_id, "page-2");
      assert.equal(saved.version.settings.page_id, "page-2");
      assert.equal(
        saved.version.candidates.length,
        before.version.candidates.length,
      );
      await button("已核对全部候选，清空候选列表").click();
      await button("保存草稿").click();
      await expect(button("保存草稿")).toBeDisabled();
      saved = await call(sp + `/versions/${d.id}/draft`);
      assert.equal(saved.version.candidates.length, 0);
      assert.equal(saved.placements.length, 1);
      assert(
        (await call(sp + `/versions/${d.base}/placements`)).items.length > 1,
      );
      await page.screenshot({
        path: join(out, "candidate-reviewed.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "E02 real model anchor, label, source-property visibility and component unlink only change the draft",
    async () => {
      const d = await clone("BRIDGE-IFC4"),
        before = await call(sp + `/versions/${d.id}/draft`),
        original = before.placements.find((p) => p.point_id === s.point),
        anchor = original.anchor.map((n, i) => (i === 0 ? n + 0.01 : n));
      assert(before.component_links.length);
      const obj = await call(sp + `/objects/${s.component}?kind=component`);
      await page.goto(
        root + `?mode=edit&section=placements&asset=${d.asset}&version=${d.id}`,
      );
      await field("业务对象").selectOption(s.point);
      await field("锚点 x,y,z（视图坐标，米）").fill(anchor.join(","));
      await field("标签偏移 dx,dy（像素）").fill("44,-16");
      await button("应用到草稿").click();
      await field("发布源属性（包含源文件中的信息）").uncheck();
      await button("解除").first().click();
      await button("保存草稿").click();
      await expect(button("保存草稿")).toBeDisabled();
      const saved = await call(sp + `/versions/${d.id}/draft`),
        point = saved.placements.find((p) => p.point_id === s.point);
      assert.deepEqual(point.anchor, anchor);
      assert.deepEqual(point.label_offset, [44, -16]);
      assert.equal(saved.version.settings.source_properties, false);
      assert.equal(
        saved.component_links.length,
        before.component_links.length - 1,
      );
      assert.deepEqual(
        (await call(sp + `/versions/${d.base}/placements`)).items.find(
          (p) => p.point_id === s.point,
        ).anchor,
        original.anchor,
      );
      assert.equal(
        (await call(sp + `/objects/${s.component}?kind=component`)).facility_id,
        obj.facility_id,
      );
      await page.screenshot({
        path: join(out, "model-draft.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "E03 GeoJSON reference association is independent from facility geographic coordinates",
    async () => {
      const d = await clone("REGION"),
        before = await call(sp + `/versions/${d.id}/draft`),
        f = before.version.manifest.features[0],
        obj = await call(sp + `/objects/${s.slope}?kind=facility`);
      await page.goto(
        root + `?mode=edit&section=placements&asset=${d.asset}&version=${d.id}`,
      );
      await field("图层图形").selectOption(f.id);
      await field("业务对象").selectOption(s.slope);
      await button("应用到草稿").click();
      await button("保存草稿").click();
      await expect(button("保存草稿")).toBeDisabled();
      const saved = await call(sp + `/versions/${d.id}/draft`);
      assert.equal(
        saved.feature_links.find((l) => l.feature_key === f.id).facility_id,
        s.slope,
      );
      const after = await call(sp + `/objects/${s.slope}?kind=facility`);
      assert.equal(after.longitude, obj.longitude);
      assert.equal(after.latitude, obj.latitude);
      await page.screenshot({
        path: join(out, "geolayer-editor.png"),
        fullPage: true,
      });
    },
  );
  assert.deepEqual(errors, []);
} finally {
  await page
    .screenshot({ path: join(out, "last-editor-state.png"), fullPage: true })
    .catch(() => {});
  await browser.close();
  const cleanupErrors = [];
  for (const id of drafts) {
    try {
      const d = await call(sp + `/versions/${id}/draft`),
        r = await call(
          sp + `/versions/${id}/draft`,
          {
            ...wm(d.version.revision),
            remove_placements: d.placements.map((p) => p.id),
            remove_component_links: d.component_links.map((p) => p.id),
            remove_feature_links: d.feature_links.map((f) => f.feature_key),
            clear_candidates: true,
          },
          "PATCH",
        );
      await call(sp + `/versions/${id}/draft`, wm(r.revision), "DELETE");
    } catch (e) {
      cleanupErrors.push(String(e));
    }
  }
  writeFileSync(
    join(out, "editors.json"),
    JSON.stringify(
      { checks, errors, cleanupErrors, completedAt: new Date().toISOString() },
      null,
      2,
    ),
  );
  await api.dispose();
  if (cleanupErrors.length) {
    console.error("Editor draft cleanup failed");
    process.exitCode = 1;
  }
}
