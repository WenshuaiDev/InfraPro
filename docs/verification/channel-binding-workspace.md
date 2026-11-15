# 测点与设备绑定正式实施自验

日期：2026-11-15。状态：**Awaiting acceptance**（自验完成，待用户人工验收）。

## 实施结果

- 租户侧栏「监测业务 → 测点与设备绑定」为正式 C 双栏配对入口，路径 `/t/:tenantId/bindings`。选择结果可刷新和深链接定位；跨设施搜索、设施筛选、状态筛选及游标分页由真实接口支持。
- 租户设施及测点详情新增「设备绑定」按钮和页签，采用 B 的测点分组与设计/设备对照卡片；资源树切换设施或测点时可保持设备绑定视图。
- 两处共用真实候选、原因校验、变更预览、提交、冲突恢复、回执查询及历史查看。直接业务通道绑定到具体设备输出代次。派生通道继续使用派生计算，项目原有来源入口及授权规则保留。
- 新增 `GET /api/v1/tenants/{tenant_id}/monitoring-source-bindings`；OpenAPI、生成类型、权限策略及客户端失效边界同步。目录批量读取当前关系，不再逐通道加载，也不截断首 50 项。
- 已移除绑定原型入口、模拟数据状态、A/B/C 切换器及启动脚本。原型源文件和选择结论保存在原评审产物目录的 `binding-prototype/archive`，未创建分支/工作树。

## 检查结果

| 检查 | 结果与边界 |
| --- | --- |
| Console TypeScript、相关文件 ESLint | 通过 |
| OpenAPI 生成类型检查、权限目录与路由覆盖检查 | 通过 |
| 正式 Vite 生产构建 | 通过；现有大分块体积提示仍存在 |
| 前端/客户端单元测试 4 项 | 通过；筛选编码、权限变化中止旧响应、权限事件、双端修订号与受限来源保护、链接输入校验 |
| 新增 Go 目录集成测试 | 通过；跨设施/测点/通道筛选、数量、分页上下文、设备信息隐藏、停用、退役、删除及租户边界 |
| 既有来源集成测试 | `TestSourceInvalidationBothCommitOrders`、`TestSourceAtomicSwitchOccupancyAndRollback` 通过 |
| 项目来源权限回归 | `TestProjectOutputOnlyGrantsAndTwoEndedSourceAuthority` 通过；未把项目资源读取误当成设备输出使用或来源维护权 |
| HTTP 来源处理器、通用设备请求处理器 | 通过 |
| 真实 Chromium 操作 | C、B 各自绑定/换绑/解绑共六种组合均通过；读取真实 API 核对最终关系 |
| 时间与历史 | 更换的旧关系结束时间等于新关系开始时间；解除后历史仍在；历史游标可继续读取 |
| 并发冲突 | 浏览器预览后另一真实 API 请求先提交；旧提交返回 409，原因保留，重新读取后阻止过期绑定操作 |
| 响应丢失 | 真实服务端提交后模拟网络响应丢失；按原请求回执恢复成功，只发生一次写入 |
| 展示与可访问性 | 375px、812px 横屏、大字号无页面横向溢出；窄屏确认按钮可见；关闭弹窗焦点回到原按钮；暗色检查通过；无运行时 JS 错误 |
| 空态与候选 | 空设施、无兼容输出有解释；不兼容、占用/停用候选不能选中 |

这是本次绑定改动及相关授权/来源行为的自验，不作为整个项目的全量验收。实际用户验收尚未进行。

## 数据和截图

真实写入仅使用新增的虚构自验样本「绑定自验·示范桥」「绑定自验·跨中测点」及主/备采集仪。12 个业务通道用于验证分页。青澜大桥原 CH-X 关系和原设备安装记录在操作前后核对一致。

- 自验设施：`c72475e7-a563-4201-97cb-189a1a4f7233`
- 自验测点：`03be8704-990c-4c00-a8f4-67e330b92791`
- 样本清单：`/Users/hijacker/.local/share/infrapro/manual-acceptance/data/binding-workspace-fixture.json`
- 证据目录：`/Users/hijacker/.codex/visualizations/2026/11/15/01a1ddc9-4fec-7f50-a7bd-a3b9ef693d72/binding-implementation`
- 核心截图：`C-unified-binding.png`、`B-facility-binding.png`、`binding-confirm.png`、`conflict-recovery.png`。
- 结构化自验记录：同目录 `verification.json`；生产构建日志：`production-build.log`。

## 访问及启动

现有本机验收环境已更新并启动，数据和账号保留：

- C：[租户统一绑定入口](http://localhost:25173/t/a78cdb12-b224-4869-9cb7-c7c1f76d0c8b/bindings)
- B：[青澜大桥设备绑定](http://localhost:25173/t/a78cdb12-b224-4869-9cb7-c7c1f76d0c8b/resources/facilities/8705da75-b421-447d-8d6e-35711d930617?view=bindings)
- 可写验收：[独立自验设施](http://localhost:25173/t/a78cdb12-b224-4869-9cb7-c7c1f76d0c8b/resources/facilities/c72475e7-a563-4201-97cb-189a1a4f7233?view=bindings)

使用现有 `accept_admin` 账号。需要恢复本机验收服务时执行：

```sh
node /Users/hijacker/.local/share/infrapro/manual-acceptance/scripts/restart.mjs
```

本次新增权限策略路由会改变权限目录摘要。部署其他环境时，API、worker 和 CLI 应使用同一版本，按既有发布流程执行 `infrapro access sync` / `infrapro access check` 后启动。本机已完成同步和相关二进制更新；此次无新增数据库迁移。

## 人工验收步骤

1. 进入租户工作台，直接点击侧栏「测点与设备绑定」。应看到 C 双栏，不需要先进入某个通道的深层详情。
2. 选择「绑定自验·示范桥」，查看第一页和下一页。应能找到全部 12 个通道；搜索名称及状态数量一致。
3. 选一个未绑定通道，搜索「绑定自验」，选择未占用的采集仪输出，填写原因、预览并确认。应看到真实成功反馈；刷新后关系保留。
4. 点击「在设施中查看」，或从设施详情点击「设备绑定」。B 应显示同一通道、同一设备；可更换为另一个可用输出。
5. 打开「设计要求与绑定历史」。应能看到历史期间；更换前后边界相邻。解除后，当前通道显示待绑定，历史仍可查。
6. 返回青澜大桥，查看「零点位移」和「停用设备来源保留样例」。应分别出现无兼容候选和需处理提示，不应把它们显示为可用设备。

请使用独立自验设施进行写入验收，避免改动原有演示业务关系。用户确认后再将状态改为 Accepted。
