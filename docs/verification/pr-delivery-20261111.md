# 2026-11-11 PR 交付与统一复验

日期：2026-11-11。交付整理状态：**Awaiting acceptance**。本记录汇总当前功能快照的复验及提交边界；各迭代的用户人工验收结果仍以对应验证文档为准。

## 1. 范围与基线

- 原账号基础 PR：[#1](https://github.com/WenshuaiDev/InfraPro/pull/1)，远端分支 `feature/account-access-console`，提交 `6997829`。
- 后续交付分支：`feature/tenant-project-engineering`，包含已存在的租户基础提交 `776663f` 及本次全部后续源码、测试、脚本和文档。
- 后续 PR 以 `feature/account-access-console` 为目标；先合并 #1，再将后续 PR 改为以 `main` 为目标并核对 diff。若 #1 使用 squash/rebase 合并，需先把后续提交重新基于 `main`，避免重复包含账号基础提交。提交、推送和 PR 创建不代表合并、生产部署或人工验收。
- 2026-11-10 因另一个聊天正在写入而中止的检查没有计入本次结果。2026-11-11 在实现完成后重新检查最终代码。

| 范围 | 主要交付与操作手册 |
| --- | --- |
| 租户治理 | 档案、管理员交接、编号历史、范围隔离；[租户基础](../operations/tenant-foundation.md) |
| 成员与角色 | 定向邀请、本人确认、暂停/结束/再加入、多角色、复制与影响预览；[成员](../operations/tenant-membership.md)、[授权](../operations/tenant-role-authorization.md) |
| 租户生命周期 | 平台启停、内部/公开说明、关系保留与实时限制；[启停恢复](../operations/tenant-lifecycle.md) |
| 项目 | 资料、负责人、唯一管理员、成员角色、交接、归档与恢复；[项目管理](../operations/project-management.md) |
| 工程定义 | 元模型、八类属性、指标单位、公共/私有模板、版本、固定依赖与样例；[工程定义](../operations/infrastructure-type-definition.md) |
| 工程资源 | 设施、构件、测点、通道、位置、固定定义、退役/软删除、独立历史与收据；[资源管理](../operations/engineering-resources.md) |
| 控制台 | 工作空间、菜单、角色/审计/租户列表、响应式抽屉、分页、权限说明、原文展示与焦点恢复 |
| 审查修复 | 草稿故障保留、邀请隐私、数值输入、树按钮、已删历史解释、Nginx 正文限制；[六项修复](code-review-fixes-20261110.md) |
| 人工验收工具 | 独立持久化数据库、虚构数据、随机凭据、正式接口准备与验证；[27 步验收手册](../operations/manual-acceptance.md) |

## 2. 本次复验

工具链：Go 1.27.0、Node.js 26.10.0、pnpm 12.18.0、Docker 29.9.3。Go 与浏览器使用专用 PostgreSQL/Redis、随机 schema 和独立缓存命名空间；HTTP 使用专用 Compose 栈。

| 命令 / 检查 | 本次结果 | 本机日志 |
| --- | --- | --- |
| `pnpm check` | 初次及最终修复后均通过：gofmt、Go vet、ESLint、TypeScript、OpenAPI 与权限生成一致性 | `check-complete.log`；此前记录保留 |
| `pnpm build` | 初次及最终修复后均通过：全部 Go module、API/CLI 及 console 生产构建 | `build-complete.log`；此前记录保留 |
| `pnpm test:identity` | 四个 Go module 全部通过，`-race -count=1`；覆盖身份、授权、租户、项目、工程定义、资源、迁移、API 与 CLI | `identity.log` |
| `pnpm test:e2e --reporter=list --output=…` | 113 项通过、2 项仅生产预览适用的路由分包故障用例按配置跳过；8.2 分钟，无失败 | `e2e.log` |
| `pnpm test:e2e:preview --reporter=list --output=…` | 最终生产构建 115 项全部通过，0 失败、0 跳过（7.1 分钟）；此前问题及修复见第 3 节 | `preview-complete.log`；此前失败记录见第 3 节 |
| 修复后的数值输入重复回归 | 最终构建桌面/手机各重复 5 次，共 10 项通过（55.6 秒），验证取消菜单、清空、逐键输入、错误反馈和面板关闭后的焦点返回 | `numeric-handoff.log` |
| 修复后的资源开发模式回归 | 中间修复时桌面/手机完整资源流程共 14 项通过；最终修复继续由完整生产预览覆盖，开发模式不冒充最终构建复验 | `e2e-resources-final.log` |
| `pnpm test:ui --reporter=list --output=…` | 6 项通过；桌面、平板、手机 | `ui.log` |
| `pnpm test:http` | 最终构建 9 项全部通过，0 失败；实际 API/CLI/Web 镜像、Nginx、普通 HTTP、请求边界、恢复及资源流程；容器与卷已清理 | `http.log`、`http-final.log`、`http-handoff.log` |
| 测试同步重点重复回归 | 账号故障注入、邀请撤销两例，桌面/手机各重复 5 次，共 20 项通过（1.4 分钟）；两处测试 ESLint 通过 | `e2e-sync.log`、`e2e-sync-lint.log` |
| 验收脚本 | 6 个 JavaScript 文件的语法和 ESLint、Python bootstrap 的 AST 语法检查通过 | 本次命令输出 |
| Git / 文档 | 通过：暂存区空白检查、45 份 Markdown 的 235 个本地链接目标、原始源码清单及本机产物排除检查 | 本次命令输出 |

日志及浏览器产物位于忽略目录 `.artifacts/pr-delivery-20261111/`。仓库没有 GitHub Actions 工作流，本表是本机执行证据。`--reporter` 和 `--output` 仅改变报告方式与存放位置，不改变测试范围；未启用自动失败重试。

验收数据原准备记录为 120 项检查通过，详见[验收数据自验](manual-acceptance-data.md)。本轮未重新执行准备脚本、灌入数据或重置已存在的验收账号；该历史结果没有计入本轮测试数量。

## 3. 提交整理

保留全部原有实现与文档；运行生成工具同步路由树。README 链接到本次记录，移除过时的“未提交、未推送”描述，修正租户权限目录入口；迁移 README 补齐 v4–v7 说明。其他阶段验证文档保留当时事实，不覆盖其人工验收状态。

### 提交前新增修复：资源菜单与编辑面板争抢焦点

完整生产预览暴露手机数值输入失败：预期未完成的 `1e` 应报错，实际旧值没有清空而形成 `1e1` / 10。原场景定向 5 次有 3 次失败，缩小到一次已有数值编辑后，3 次有 2 次失败。第一次仅取消菜单关闭时的焦点返回，定向 10 次虽通过，但下一轮完整预览仍失败，新增的“清空后为空”断言准确拦住了问题。

进一步用浏览器内存事件及焦点调用栈定位，5 次观测中复现 1 次失败：输入框获得焦点后，尚未卸载的菜单执行 `onItemLeave`，立即把焦点拉回菜单，随后面板初始焦点又接管；清空的 Delete 没有送达输入框。不是数值解析或服务端保存错误。

最终修复在选择操作时只记录意图，等菜单关闭/卸载的 `onCloseAutoFocus` 回调再打开资源操作面板并取消旧菜单的自动焦点返回。菜单和面板按顺序交接，避免退出期间的指针事件干扰新表单。未选择操作而取消菜单时仍按原行为返回触发按钮。回归断言覆盖 Esc 取消、清空输入、逐键 `1e` 保留与报错、关闭面板后焦点返回。该路径也适用于编辑、退役、恢复和删除。

重点回归可重复执行：

```bash
pnpm test:e2e:preview tests/e2e/resources.spec.ts --grep 'numeric resource inputs' --repeat-each=5
```

两轮全量失败日志为 `preview.log`、`preview-final.log`；定位证据包括 `numeric-repro.log`、`numeric-minimal.log`、`numeric-events.log`、`numeric-focus-full-probe.log` 及其 `focus-events.json`。临时诊断代码及测试文件已移除。菜单交接修复后的检查/构建、数值重点回归和 HTTP 使用 `*-handoff.log`；完整最终检查/构建使用 `check-complete.log`、`build-complete.log`；数值规则和 Go 后端没有改变，已通过的完整 Go race 结果继续适用。

### 提交前修正：测试前置条件同步

菜单交接修复后的完整预览为 113 通过、2 失败，记录于 `preview-handoff.log`；资源数值用例的桌面/手机均通过。其余两处失败来自测试前置条件没有同步：

- 账号故障注入：浏览器 trace 显示故障 mock 安装前发出的 `/auth/me` 仍返回 200；身份检查对正在进行的刷新合并请求，单次模拟 focus 没有命中 503。测试现在等待故障路由确实收到读取，再断言不可用界面和草稿保留。仍检查密码确认、CSRF 失败不自动重放写入，未修改身份合并逻辑。
- 租户邀请撤销：管理员点击撤销后，测试立即在另一个浏览器接受，造成两笔写入竞争。测试现在等待撤销响应 200 和确认对话框关闭，再检验接受已撤销邀请会失败、查验显示已撤销且没有成员关系。未改变服务端事务或邀请规则。

两处只修改测试同步，不增加固定休眠、放宽断言或自动失败重试；其余最终生产代码与已通过的 HTTP 镜像一致。桌面/手机各对两例重复 5 次，日志为 `e2e-sync.log`；最终完整生产预览另存 `preview-complete.log`。

本机配置、凭据、邀请链接、备份、测试报告与构建产物按既有 Git 忽略规则保留。提交前核对源码清单与捕获快照，推送后核对当前提交与远端引用、PR 头提交和工作区状态。

## 4. 启动与人工验收

当前数据库目标版本为 v9，权限目录为 90 项。已有环境升级顺序：备份并验证恢复 → 停止旧 API 写入 → 发布匹配代码 → 迁移 → 权限同步及检查 → 启动并检查 readiness。

```bash
make install
pnpm infrapro database migrate
pnpm infrapro access sync --dry-run
pnpm infrapro access sync
pnpm infrapro access check
make dev
```

开发入口为 `http://127.0.0.1:5173/`；容器入口为 `/console/`。仅全新环境按 README 执行 `pnpm setup:local` 和交互 `account bootstrap`；已有账号不重复初始化。

持久化人工验收环境入口为 `http://localhost:25173/login`。更新构建和启动方式见[手册第 5 节](../operations/manual-acceptance.md#5-重启维护与数据保留)；账号和精确数据链接存放于本机 `.artifacts/manual-acceptance/账号与验收入口.local.md`。按手册的 27 步依次检查身份隔离、租户成员和项目、定义发布及版本固定、真实资源与历史、并发/故障、移动端和六项审查修复；每一步已有对应预期。

侧栏导航和工作空间切换保留原 Accepted 记录；其他功能按各自记录保持 Awaiting acceptance。只有用户明确确认相应人工验收后，才更新为 Accepted。

## 5. 迁移和验证边界

v3–v9 涉及租户、成员、角色、生命周期、项目、工程定义与资源，不支持破坏性 Down。数据库、权限目录和应用版本须匹配；旧服务不能与新数据库混跑。需要恢复时使用经过实际恢复核对的备份和匹配程序，并按账号手册撤销恢复前凭据。既有自定义角色不会因目录同步自动扩权。

本轮不执行生产部署、生产迁移或 PR 合并。浏览器验证为 Chromium，未覆盖 Safari/Firefox、实体手机、生产容量、长期稳定性或现场采集。构建仍有大于 500 kB 的 chunk 提示，以及既有路由工具循环依赖/Node 弃用提示，构建和检查成功。设备采集、运行监测、曲线告警、地图、图形绑定、审批和计费均不在当前交付范围。
