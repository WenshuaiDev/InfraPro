import { requireAuthenticatedLogin } from './login-result.mjs';
import assert from "node:assert/strict";
import { chromium, request, expect as baseExpect } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { readFileSync, readdirSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
const expect = baseExpect.configure({ timeout: 15000 });
const sessions = ".artifacts/spatial-tests",
  dir =
    process.env.SPATIAL_SESSION ??
    join(sessions, readdirSync(sessions).sort().at(-1)),
  state = JSON.parse(readFileSync(join(dir, "spatial-acceptance.json"))),
  login = JSON.parse(readFileSync(join(dir, "login.json"))),
  origin = "http://127.0.0.1:15473",
  base = `/tenants/${state.tenant}`,
  sp = base + "/spatial",
  root = `${origin}/t/${state.tenant}/spatial`,
  out = ".artifacts/spatial-redesign";
mkdirSync(out, { recursive: true });
const api = await request.newContext(),
  csrf = await (await api.get(origin + "/api/v1/auth/csrf")).json(),
  headers = { Origin: origin, "X-CSRF-Token": csrf.token };
async function call(path, data, method = data === undefined ? "GET" : "POST") {
  const r = await api.fetch(origin + "/api/v1" + path, {
    headers,
    data,
    method,
  });
  assert(r.ok(), `${method} ${path}: ${r.status()} ${await r.text()}`);
  return r.json();
}
requireAuthenticatedLogin(await call("/auth/login", {
  username: login.username,
  password: login.password,
}));
const browser = await chromium.launch({
    args: ["--use-angle=swiftshader", "--enable-webgl"],
  }),
  ctx = await browser.newContext({ viewport: { width: 1600, height: 1000 } });
await ctx.addCookies((await api.storageState()).cookies);
const page = await ctx.newPage(),
  errors = [],
  checks = [],
  created = {};
page.on("pageerror", (e) => errors.push(e.message));
const query = () => Object.fromEntries(new URL(page.url()).searchParams),
  button = (name, scope = page) =>
    scope.getByRole("button", { name, exact: true }),
  field = (name, scope = page) => scope.getByLabel(name, { exact: true });
async function check(name, fn) {
  const start = performance.now();
  await fn();
  checks.push({
    name,
    passed: true,
    duration_ms: Math.round(performance.now() - start),
  });
  console.log("PASS " + name);
}
try {
  await check(
    "A01 six task entries, URL navigation, real facility scope, library search and empty states",
    async () => {
      await page.goto(root);
      await expect(
        page.getByRole("heading", { name: "从哪里开始查看？", exact: true }),
      ).toBeVisible();
      await expect(page.getByLabel("空间对象详情")).toHaveCount(0);
      for (const label of [
        "空间总览",
        "GIS 地图",
        "布设图",
        "BIM / 三维",
        "测点标注",
        "资料与版本",
      ])
        await expect(
          button(label, page.getByRole("navigation", { name: "工程空间功能" })),
        ).toBeVisible();
      await button("全部设施").click();
      let panel = page.getByRole("dialog", { name: "选择设施范围" });
      await field("搜索设施名称或编码", panel).fill("示例桥梁");
      await panel.getByRole("button").filter({ hasText: "示例桥梁" }).click();
      await expect.poll(() => query().facility).toBe(state.facility);
      await expect(panel).toBeHidden();
      await button(
        "资料与版本",
        page.getByRole("navigation", { name: "工程空间功能" }),
      ).click();
      await field("资料类型筛选").selectOption("layout");
      await field("搜索资料名称或编码").fill("BRIDGE-PNG");
      await expect(page.getByRole("table")).toContainText("桥梁平面图 PNG");
      assert.equal(
        await page.getByRole("table").locator("tbody tr").count(),
        1,
      );
      await field("搜索资料名称或编码").fill("NO-SUCH-SPATIAL");
      await expect(
        page.getByText("没有符合当前范围的空间资料。可调整设施或筛选条件。", {
          exact: true,
        }),
      ).toBeVisible();
      await page.reload();
      await expect(field("搜索资料名称或编码")).toHaveValue("NO-SUCH-SPATIAL");
      await page.screenshot({
        path: join(out, "library-empty.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "A02 real UI registration, draft creation, source upload, conversion and task details",
    async () => {
      await page.goto(root + `?section=assets&facility=${state.facility}`);
      await button("导入空间资料").click();
      await expect(
        page.getByRole("dialog", { name: "导入空间资料" }),
      ).toBeVisible();
      const code = "UI-A-" + Date.now();
      created.code = code;
      created.name = "A方案自验·真实布设图 " + code;
      await field("资料名称").fill(created.name);
      await field("资料编码").fill(code);
      await field("所属设施").selectOption(state.facility);
      await field("来源、版权及用途说明").fill(
        "系统合成样本；工程空间 A 方案真实操作自验。",
      );
      await button("保存资料信息").click();
      await expect.poll(() => query().asset).toBeTruthy();
      created.asset = query().asset;
      await field("版本变更说明").fill("浏览器导入、处理、标注和发布闭环");
      await button("创建草稿").click();
      await expect.poll(() => query().version).toBeTruthy();
      created.version = query().version;
      await field("选择空间原件").setInputFiles(
        "samples/spatial/bridge-plan.png",
      );
      await button("上传并校验").click();
      await expect(button("开始处理原件")).toBeEnabled({ timeout: 90000 });
      await button("开始处理原件").click();
      await expect(button("编辑挂载、关联与默认视图")).toBeVisible({
        timeout: 120000,
      });
      const v = await call(sp + `/versions/${created.version}/manifest`);
      assert(v.manifest_hash);
      assert.equal(v.state, "draft");
      assert.equal(v.source.name, "bridge-plan.png");
      assert(v.latest_job_id);
      created.job = v.latest_job_id;
      await page.screenshot({
        path: join(out, "import-ready.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "A03 edit existing point, independent anchor and label, saved draft, navigation guard",
    async () => {
      await button("编辑挂载、关联与默认视图").click();
      await expect(page.getByLabel("编辑空间挂载")).toBeVisible();
      await field("业务对象").selectOption(state.point);
      await field("锚点 x,y（0–1，左上角为原点）").fill("0.31,0.47");
      await field("标签偏移 dx,dy（像素）").fill("32,-24");
      await button("应用到草稿").click();
      await button("保存草稿").click();
      await expect(button("保存草稿")).toBeDisabled();
      const d = await call(sp + `/versions/${created.version}/draft`);
      assert.equal(d.placements.length, 1);
      assert.deepEqual(d.placements[0].anchor, [0.31, 0.47]);
      assert.deepEqual(d.placements[0].label_offset, [32, -24]);
      assert.equal(d.placements[0].point_id, state.point);
      await page
        .getByRole("button", { name: /主梁跨中位移点 · page-1/ })
        .click();
      await field("标签偏移 dx,dy（像素）").fill("88,11");
      await button("应用到草稿").click();
      await button(
        "资料与版本",
        page.getByRole("navigation", { name: "工程空间功能" }),
      ).click();
      await expect(page.getByRole("alertdialog")).toBeVisible();
      await button("继续编辑").click();
      await expect(page.getByLabel("编辑空间挂载")).toBeVisible();
      await button("取消编辑").click();
      await button("放弃修改并离开").click();
      await expect(page.getByLabel("编辑空间挂载")).toHaveCount(0);
      assert.equal(
        (await call(sp + `/versions/${created.version}/draft`)).version
          .revision,
        d.version.revision,
      );
      await button("编辑挂载、关联与默认视图").click();
      await button("核对与发布").click();
      await expect(
        page.getByRole("dialog", { name: created.name + " · 资料与版本" }),
      ).toBeVisible();
    },
  );
  await check(
    "A04 validate, masked publish preview, publish and set tenant current version",
    async () => {
      await field("本次变更原因").fill("图面、来源内容、锚点和标签已核对");
      await button("校验草稿").click();
      await expect(
        page.getByText("当前草稿校验通过。", { exact: true }),
      ).toBeVisible();
      await button("预览发布").click();
      let preview = page.getByRole("dialog", { name: "确认空间资料变更" });
      await expect(preview).toContainText("项目继续使用各自获准的版本");
      await button("确认变更", preview).click();
      await expect(preview).toBeHidden();
      await expect
        .poll(
          async () =>
            (await call(sp + `/versions/${created.version}/manifest`)).state,
        )
        .toBe("published");
      await button("设为当前版").click();
      await button(
        "确认变更",
        page.getByRole("dialog", { name: "确认空间资料变更" }),
      ).click();
      await expect
        .poll(
          async () =>
            (await call(sp + `/assets/${created.asset}`)).current_version_id,
        )
        .toBe(created.version);
      await page.screenshot({
        path: join(out, "published.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "A05 grant fixed version, independent source download, project read, then end grant",
    async () => {
      await page.goto(
        `${origin}/t/${state.tenant}/projects/${state.projects.A.id}/spatial-access`,
      );
      await button("授权空间资料").click();
      await field("搜索资料").fill(created.code);
      await expect(
        field("资料").locator("option", { hasText: created.name }),
      ).toHaveCount(1);
      await field("资料").selectOption(created.asset);
      await expect(field("明确选择已发布版本").locator("option")).toHaveCount(
        2,
      );
      await field("明确选择已发布版本").selectOption(created.version);
      await field("设为此项目的该资料默认版本").check();
      await field("本次授权变更原因").fill(
        "A方案自验授权图模，保持原件不可下载",
      );
      await button("预览授权或调整").click();
      await expect(
        page.getByRole("dialog", { name: "空间授权变更预览" }),
      ).toContainText("不允许");
      await button("确认授权变更").click();
      await expect(
        page.getByText("空间资料授权已更新。", { exact: true }),
      ).toBeVisible();
      const grants = await call(
          base + `/projects/${state.projects.A.id}/spatial-access`,
        ),
        g = grants.grants.find(
          (g) => g.version_id === created.version && !g.ended_at,
        );
      assert(g);
      assert.equal(g.source_download, false);
      created.grant = g.id;
      const projectVersion = await call(
        base +
          `/projects/${state.projects.A.id}/spatial/versions/${created.version}/manifest`,
      );
      assert.equal(projectVersion.can_download_source, false);
      await page.screenshot({
        path: join(out, "project-access.png"),
        fullPage: true,
      });
      await page.goto(
        `${origin}/t/${state.tenant}/p/${state.projects.A.id}/spatial?version=${created.version}&owner=${state.point}&owner_kind=point`,
      );
      await expect(
        page
          .getByLabel("空间对象详情")
          .getByRole("heading", { name: "主梁跨中位移点", exact: true }),
      ).toBeVisible({ timeout: 30000 });
      await expect(button("导入空间资料")).toHaveCount(0);
      await expect(button("编辑挂载、关联与默认视图")).toHaveCount(0);
      await page.goto(
        `${origin}/t/${state.tenant}/projects/${state.projects.A.id}/spatial-access`,
      );
      const row = page.getByRole("row").filter({ hasText: created.name });
      await button("结束授权", row).click();
      await field("本次授权变更原因").fill("结束本次自验授权");
      await button("预览结束授权").click();
      await button("确认授权变更").click();
      await expect
        .poll(
          async () =>
            !!(
              await call(
                base + `/projects/${state.projects.A.id}/spatial-access`,
              )
            ).grants.find((g) => g.id === created.grant)?.ended_at,
        )
        .toBe(true);
      await field("显示已结束授权").check();
      await expect(
        page.getByRole("row").filter({ hasText: created.name }),
      ).toContainText("已结束");
    },
  );
  await check(
    "A06 withdraw current version, retire and restore, real history preserved",
    async () => {
      await page.goto(
        root +
          `?section=assets&mode=manage&asset=${created.asset}&version=${created.version}`,
      );
      await field("本次变更原因").fill("自验撤回，不保留当前版本");
      await button("预览撤回").click();
      await expect(
        page.getByRole("dialog", { name: "确认空间资料变更" }),
      ).toContainText("相关项目授权同时结束");
      await button(
        "确认变更",
        page.getByRole("dialog", { name: "确认空间资料变更" }),
      ).click();
      await expect
        .poll(
          async () =>
            (await call(sp + `/assets/${created.asset}/versions`)).items.find(
              (v) => v.id === created.version,
            )?.state,
        )
        .toBe("withdrawn");
      assert.equal(
        (
          await api.get(
            origin + "/api/v1" + sp + `/versions/${created.version}/manifest`,
          )
        ).status(),
        404,
      );
      assert(!(await call(sp + `/assets/${created.asset}`)).current_version_id);
      await field("本次变更原因").fill("A方案生命周期自验");
      await button("预览退役资料").click();
      await button(
        "确认变更",
        page.getByRole("dialog", { name: "确认空间资料变更" }),
      ).click();
      await expect(button("恢复资料")).toBeVisible();
      await button("恢复资料").click();
      await expect(button("预览退役资料")).toBeVisible();
      assert.equal(
        (await call(sp + `/assets/${created.asset}`)).status,
        "normal",
      );
      await button(
        "变更记录",
        page.getByRole("navigation", { name: "资料管理内容" }),
      ).click();
      await expect(
        page.getByRole("heading", { name: "资料变更记录", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByText("查看记录字段", { exact: true }).first(),
      ).toBeVisible();
      await page.screenshot({ path: join(out, "history.png"), fullPage: true });
    },
  );
  await check(
    "A07 empty draft deletion confirms and preserves published version history",
    async () => {
      await page.goto(
        root +
          `?section=assets&mode=manage&asset=${created.asset}&version=${created.version}`,
      );
      await page.getByText("创建下一个草稿", { exact: true }).click();
      await field("版本变更说明").fill("删除空草稿自验");
      await button("创建草稿").click();
      await expect.poll(() => query().version).not.toBe(created.version);
      const empty = query().version;
      await field("本次变更原因").fill("未上传与未使用的临时草稿");
      await button("删除未使用草稿").click();
      await expect(
        page.getByRole("dialog", { name: "删除未使用草稿" }),
      ).toBeVisible();
      await button("确认删除草稿").click();
      await expect(
        page.getByRole("dialog", { name: "删除未使用草稿" }),
      ).toBeHidden();
      const versions = await call(sp + `/assets/${created.asset}/versions`);
      assert(!versions.items.some((v) => v.id === empty));
      assert(versions.items.some((v) => v.id === created.version));
    },
  );
  await check(
    "A08 legacy version-only and owner-only links; cross-carrier identity; invalid version never falls back",
    async () => {
      const version = state.assets["BRIDGE-PDF"].versions[0].id;
      await page.goto(
        root + `?version=${version}&owner=${state.point}&owner_kind=point`,
      );
      await expect(field("图纸页码")).toBeVisible();
      await expect.poll(() => query().section).toBe("layouts");
      await button(
        "BIM / 三维",
        page.getByRole("navigation", { name: "工程空间功能" }),
      ).click();
      await expect.poll(() => query().version).toBeTruthy();
      assert.equal(query().owner, state.point);
      await expect(
        page
          .getByLabel("空间对象详情")
          .getByRole("heading", { name: "主梁跨中位移点", exact: true }),
      ).toBeVisible();
      await page.goto(root + `?owner=${state.point}&owner_kind=point`);
      await expect(field("地理位置图层")).toBeVisible();
      await expect(page.getByLabel("空间对象详情")).toBeVisible();
      const invalid = randomUUID();
      await page.goto(root + `?version=${invalid}`);
      await expect(page.getByRole("alert").first()).toBeVisible();
      assert.equal(query().version, invalid);
      await expect(page.locator("canvas")).toHaveCount(0);
    },
  );
  await check(
    "A09 phone, dark theme, keyboard navigation, masked Drawer focus return",
    async () => {
      await page.goto(root + "?section=assets");
      const trigger = button("导入空间资料");
      await trigger.focus();
      await trigger.press("Enter");
      await expect(
        page.getByRole("dialog", { name: "导入空间资料" }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(
        page.getByRole("dialog", { name: "导入空间资料" }),
      ).toBeHidden();
      await expect(trigger).toBeFocused();
      await page.setViewportSize({ width: 390, height: 844 });
      await page.evaluate(() => document.documentElement.classList.add("dark"));
      for (const section of [
        "overview",
        "gis",
        "layouts",
        "models",
        "placements",
        "assets",
      ]) {
        await page.goto(root + `?section=${section}`);
        await expect(
          page.getByRole("heading", { name: "工程空间", exact: true }),
        ).toBeVisible();
        await page.evaluate(() =>
          document.documentElement.classList.add("dark"),
        );
        assert(
          !(await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth,
          )),
          section + " overflow",
        );
      }
      await button("导入空间资料").click();
      await expect(
        page.getByRole("dialog", { name: "导入空间资料" }),
      ).toBeVisible();
      assert(
        !(await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        )),
      );
      await page.screenshot({
        path: join(out, "phone-dark-drawer.png"),
        fullPage: true,
      });
    },
  );
  assert.deepEqual(errors, []);
} finally {
  writeFileSync(
    join(out, "redesign-flows.json"),
    JSON.stringify(
      { checks, errors, created, completedAt: new Date().toISOString() },
      null,
      2,
    ),
  );
  await page
    .screenshot({ path: join(out, "last-flow-state.png"), fullPage: true })
    .catch(() => {});
  await browser.close();
  await api.dispose();
}
