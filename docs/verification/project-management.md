# 项目管理实现与自验证据

日期：2026-11-10。对应[已确认方案](../requirements/project-management-plan.md)和[启动及人工验收手册](../operations/project-management.md)。

- 方案及 D-01–D-16：用户已确认并授权实施。
- 功能状态：**Awaiting acceptance**；2026-11-10 实施及自验通过，人工验收待用户确认。自验与用户人工验收分别记录。
- 基线：`feature/account-access-console`，HEAD `776663fc125223a02df6f46a24e5d0cf7663a8da` 加原有未提交租户功能。文件哈希见 `.artifacts/project-management/baseline-manifest.json`，原有变更已保留。
- 本轮实现 PJ-01–PJ-09，迁移 v7；本机 v6→v7 已完成。未提交、推送或部署生产；用户人工验收未执行。

## 1 交付与实现位置

| 交付 | 实现 |
| --- | --- |
| 创建、资料、编号、个人列表与治理摘要 | `packages/go/platform/projects/{service,queries,model}.go` |
| 成员、多角色、管理员/负责人及归档恢复 | `projects/{changes,roles}.go`、`access/project_{authorization,policy,catalog}.go` |
| 上下层保护、应急和受控空缺 | `access/project_protection.go`、tenancy/identity 共用事务入口 |
| 数据约束、历史及幂等收据 | `migrations/00007_projects.sql` |
| API 和客户端 | `transport/http/projects`、`contracts/openapi/api.yaml`、`api-client/src/projects.ts` 及生成类型 |
| 工作空间、范围清理、危险操作及响应式页面 | `apps/console/src/features/projects`、tenancy context/editor、workspace shell 和实际路由 |
| 术语、架构、运行与人工验收 | `GLOSSARY.md`、`docs/architecture.md`、本记录及操作手册 |

Go/Gin、PostgreSQL/pgx/Goose、Redis、React/TypeScript、TanStack Router/Query、共享 shadcn 和 OpenAPI 沿用现有工程。所有权限判断在服务端按当前范围和状态复验。

## 2 环境、命令及实际结果

本机开发与 Docker；Go 1.27.0、Node 26.10.0、pnpm 12.18.0，真实 PostgreSQL/Redis。Go 使用隔离 schema，带 `-race -count=1`；浏览器使用 Chromium 桌面/移动配置；HTTP 使用独立 Nginx/Compose。下表日志均位于忽略目录 `.artifacts/project-management/`。

| 命令/检查 | 结果 | 证据文件 |
| --- | --- | --- |
| 实施前 Go 基线回归 | 通过 | `baseline-go.log` |
| `pnpm check` | 通过 | `check-complete.log` |
| `pnpm build` | 通过 | `build-complete.log` |
| `pnpm test:identity` | 全部通过 | `go-full-final-2.log` |
| 项目全部业务与数据库测试 | 14 项通过 | `projects-complete.log` |
| 补充候选/交接/编号竞争 | 2 项通过；治理列表空集合契约复核通过 | `acceptance-edges.log`、`collection-contract.log` |
| `pnpm test:e2e` | 69 通过、2 开发模式跳过 | `e2e-full-final.log`；跳过项为仅构建模式适用的路由块检查 |
| `pnpm test:e2e:preview` | 71 通过，无跳过 | `e2e-preview-final.log` |
| 最后范围清理及取消操作回归 | 12 项通过 | `e2e-scopes-final.log`，项目与租户角色流程 |
| `pnpm test:http` | 6 项通过 | `http-complete.log`、`http-final.log` |
| 真实本机 v6 备份恢复 | 20 表逐行摘要一致 | `local-backup-check.json` |
| 本机 v6→v7、sync/check | 通过；0 个项目、71 权限、13 内置角色 | `local-upgrade-result.json`、`local-migrate.log`、`local-sync.log`、`local-check.log` |
| 重复 migrate/sync/check | 25 表完全不变 | `local-repeat-check.json`、`local-repeat.log` |
| 原自定义角色逐字段核对 | 1 个角色保持不变，原 role_permissions 无缺失 | `custom-role-preservation.json`、`local-upgrade-result.json` |
| 含 v7 项目和历史的备份恢复 | 25 表数量/摘要一致、约束和访问复核通过 | G12、`restore-and-publication-final.log` |
| 界面观察 | 375/768/1440，亮暗主题无横向溢出；已查看截图 | `screenshots/project-archive-{375,768,1440}-{light,dark}.png` |

保留的非阻断提示：Vite 主入口压缩体积约 510 kB，触发 500 kB 体积提示；TanStack 路由生成器有已有循环依赖提示。静态检查和构建仍成功。本轮未修改共享 UI 原语，未另跑 `test:ui`，应用展示与主题包含在完整 E2E。

本机升级核对：账号、密码记录、会话、账号角色、初始化记录、租户、编号、成员、成员角色、邀请均未丢失或改写；原审计保留。变化限定为迁移记录、目录新增/元数据、固定租户管理员版本及权限、5 个新项目预置角色、授权修订和发布审计。未重置账号，未生成演示项目。备份和原始配置权限为 0600，不提交到 Git。

## 3 证据索引

以下 G 前缀函数均以 `TestProject` 开头，位于 `packages/go/platform/projects`。映射表示核心规则的自动自验，不表示所有账号/数据排列均穷举，也不代替用户人工验收。

| 索引 | 测试/证据 |
| --- | --- |
| G1 | [projects_test.go](../../packages/go/platform/projects/projects_test.go) · `CreationIsolationAtomicityAndCodes`：创建、无隐式成员、编号、字段与审计故障回滚 |
| G2 | 同文件 · `MembersRolesUnionHistoryAndParentCascade`：并集、空角色、暂停恢复、重入及父关系结束 |
| G3 | 同文件 · `ArchiveRevocationTransferAndRecovery`：归档、合法撤权、交接、应急停用及修复 |
| G4 | 同文件 · `ConcurrentCreationPreviewAndConstraints`：并发、过期预览/版本、SQL 约束、会话复验 |
| G5 | 同文件 · `AuditIsolationAndTenantRoleIndependence`：审计隔离、租户空角色独立项目授权、目录数量 |
| G6 | [boundaries_test.go](../../packages/go/platform/projects/boundaries_test.go) · `ControlledTenantRepairAndRecoveryChain`：D-12 完整链与旧会话撤销 |
| G7 | 同文件 · `RoleBoundariesCopyAndCompleteImpact`：多成员并集、暂停成员潜在影响、复制、软删除 |
| G8 | 同文件 · `UpperEligibilityCoverageAndCandidateRecheck`：授予覆盖、父资格、预配置及提交时候选复验 |
| G9 | 同文件 · `CatalogPublicationAuditCleanupAndPaging`：归档发布预检、逐成员审计、分页和清理 |
| G10 | 同文件 · `SerializedRacesAndReplayAfterRevocation`：归档/加人、移除/授权、交接/父移除、同步竞争及超时回滚 |
| G11 | 同文件 · `ArchiveFullOperationMatrixAndIdentityConstraints`：归档操作矩阵、SQL、所有项目策略受租户停用限制 |
| G12 | [backup_restore_test.go](../../packages/go/platform/projects/backup_restore_test.go) · `BackupRestoreWithAllHistory`：实际 pg_dump/pg_restore |
| G13 | [acceptance_test.go](../../packages/go/platform/projects/acceptance_test.go) · `CandidateHandoverAndIndependentPresetCopy`：普通预置复制、多角色接任、候选边界、过去计划日期、长字段 |
| G14 | 同文件 · `DistinctRequestsCompeteForSameCode`：不同请求竞争编号，无部分残留 |
| M | [membership_upgrade_test.go](../../packages/go/platform/migrations/membership_upgrade_test.go)，v3/v4/v5/v6 保留与 v7 目标；既有升级回归 |
| B1 | [projects.spec.ts](../../tests/e2e/projects.spec.ts) · complete UI：创建、成员、并集、归档、恢复和取消删除 |
| B2 | 同文件 · stale responses：真实延迟写响应、临时故障、密码复验不重放、现有标签失权 |
| B3 | 同文件 · HTTP strict bodies：直接请求跨范围、CSRF、重复请求、归档限制 |
| H | [HTTP 项目部署测试](../../tests/http/projects.spec.ts) 及完整 HTTP 维护/账号/租户回归 |
| S | Go vet/格式、ESLint、TS、生成权限/OpenAPI 同步、路由策略与安全方案引用检查 |
| L1/L2/L3 | 上表本机备份恢复 / 升级保留 / 重复执行及自定义角色核对 |

## 4 PJA-01–PJA-80 映射

每行核心自验证据见第 3 节，人工验收统一待用户确认。PJA-68 的并发覆盖移除/绑定，角色启停状态边界单独验证；PJA-79 的长文本输入边界由 Go 验证，浏览器覆盖窄屏、主题、键盘、故障和对话框。其余操作体验按手册人工确认。

| 编号 | 验收场景 | 自验证据 |
| --- | --- | --- |
| PJA-01 | 有权租户成员创建，负责人和管理员不同 | G1 / B1 |
| PJA-02 | 创建者未被选为管理员或成员 | G1 / B1 |
| PJA-03 | 同键同内容双击/超时重试创建 | G1 / G4 |
| PJA-04 | 同键不同请求内容 | G1 |
| PJA-05 | 同租户并发创建相同编号 | G14 |
| PJA-06 | 两租户使用相同编号 | G1 / B2 / B3 |
| PJA-07 | 改编号后另一项目尝试使用旧编号 | G1 |
| PJA-08 | 非法字段、空目的、倒置日期和超长内容 | G1 / G13 / B3 / H |
| PJA-09 | 候选人在选择后被停用/暂停/移除 | G8 / B2 |
| PJA-10 | 创建任一步或审计写入故障 | G1 |
| PJA-11 | 我的项目与租户治理列表比较 | G1 / B1 |
| PJA-12 | 平台管理员、租户管理员未加入目标项目 | G1 / G6 / B1 |
| PJA-13 | 项目角色不同的同一成员切换 A/B | B2 / B3 / G7 |
| PJA-14 | 篡改 tenant/project/member/role ID 组合 | G1 / G7 / G11 / B3 |
| PJA-15 | 切换项目时旧响应延迟到达 | B2 |
| PJA-16 | 已清空租户业务角色但租户关系有效 | G5 |
| PJA-17 | 添加有效租户成员 | G2 / B1 |
| PJA-18 | 选择外租户/平台非成员 | G13 / B3 |
| PJA-19 | 重复添加已有 active/suspended 成员 | G2 / G13 |
| PJA-20 | 暂停项目成员 | G2 / B2 |
| PJA-21 | 恢复项目成员 | G2 / G8 |
| PJA-22 | 移除后再次添加项目成员 | G2 |
| PJA-23 | 清空普通成员角色 | G2 |
| PJA-24 | 普通成员暂停/移除自己或管理员 | G8 / G13 |
| PJA-25 | 管理人员处置超出本人覆盖范围的目标 | G8 |
| PJA-26 | 成员被暂停时预配置/上层失效时扩权 | G8 |
| PJA-27 | 租户成员暂停与恢复 | G8 |
| PJA-28 | 租户成员移除后重新加入 | G2 / G6 |
| PJA-29 | 项目权限目录与可配置列表 | G5 / S |
| PJA-30 | 直接分配普通预置项目角色 | G2 / G13 |
| PJA-31 | 复制普通预置/本项目自定义角色 | G7 / G13 |
| PJA-32 | 复制管理员或其他项目/租户/平台角色 | G7 |
| PJA-33 | 角色同项目重名、跨项目同名 | G7 |
| PJA-34 | 给自定义角色加入保留/退役/其他范围权限 | G4 / G7 |
| PJA-35 | 多角色共同提供同一权限后撤销一角色 | G2 / G7 / B1 |
| PJA-36 | 停用角色及再启用 | G2 / G11 |
| PJA-37 | 新增绑定到停用或删除角色 | G2 / G7 |
| PJA-38 | 删除有当前绑定的角色，包括暂停成员 | G7 |
| PJA-39 | 角色或成员授权预览后数据变化 | G4 / G6 / B2 |
| PJA-40 | 一个角色作用于多个成员，只有部分实际失权 | G7 / G9 |
| PJA-41 | 普通 API 给第二人授管理员或撤当前管理员 | G5 / G13 / G4 |
| PJA-42 | 交接给现有 active 多角色成员 | G13 |
| PJA-43 | 交接给未加入项目的有效租户成员 | G3 |
| PJA-44 | 交接给暂停项目成员、自己或失效候选 | G8 / G13 |
| PJA-45 | 修改负责人 | G5 |
| PJA-46 | 负责人失效但不是项目管理员 | G8 |
| PJA-47 | 常规停用账号或暂停/移除租户成员影响管理员 | G3 / G6 |
| PJA-48 | 维护 CLI 应急停用项目管理员 | G3 / G6 / H |
| PJA-49 | 无治理权限调用失管恢复；对可用管理员调用恢复 | G3 / B3 |
| PJA-50 | 有权租户管理员修复失管项目 | G3 / G6 |
| PJA-51 | 正常租户管理员交接且原任负责项目 | G6 |
| PJA-52 | 不可用租户管理员修复、租户恢复、项目恢复链 | G6 |
| PJA-53 | 归档项目 | G3 / G11 / B1 |
| PJA-54 | 归档后读取各业务对象 | G3 / G9 / G11 |
| PJA-55 | 归档后编辑资料/负责人、加人、恢复成员 | G11 / B3 |
| PJA-56 | 归档后新建/复制/启用角色或增加权限/绑定 | G3 / G9 / G11 |
| PJA-57 | 归档后暂停/移除成员、减绑定、减角色权限 | G3 / G7 / G11 |
| PJA-58 | 归档后交接或失管恢复 | G3 / G6 |
| PJA-59 | 恢复项目 | G2 / G3 / G11 / B1 |
| PJA-60 | 管理员不可用或空缺时恢复项目 | G6 / G8 |
| PJA-61 | 修改计划日期或到达计划结束日期 | G13 |
| PJA-62 | 租户停用/恢复含多个项目 | G6 / G11 / H |
| PJA-63 | 预置发布给归档项目带来新增权限 | G9 |
| PJA-64 | 账号恢复及重新登录 | G6 |
| PJA-65 | 并发编辑资料及过期 expected_version | G4 / B2 |
| PJA-66 | 归档与加人/扩权并发 | G10 |
| PJA-67 | 交接与上层停用/成员移除并发 | G10 / G6 |
| PJA-68 | 成员移除与角色绑定、角色启停并发 | G10 / G2 |
| PJA-69 | 同键重放非创建写入 | G1 / G3 / G10 |
| PJA-70 | 跨项目/租户操作和目录同步争用锁 | G10 |
| PJA-71 | 项目审计各类事件、前后快照和原因 | G1 / G3 / G6 / G7 / G9 |
| PJA-72 | 平台/租户审计搜索、筛选及分页 | G5 / G9 / B3 |
| PJA-73 | 已有审计清理任务处理过期项目事件 | G9 |
| PJA-74 | 数据库直接写跨范围关系/保留能力/非法管理员 | G4 / G11 / G6 |
| PJA-75 | 从空库和上一轮真实基线升级 | M / L1 / L2 |
| PJA-76 | access sync/check 及既有账号租户回归 | S / M / G9 / L2 / L3 |
| PJA-77 | 在隔离环境实际恢复备份 | G12 / L1 |
| PJA-78 | 多标签、旧页面、重新聚焦及授权减少 | B2 |
| PJA-79 | 窄屏、键盘、长名称、故障及确认弹窗 | B1 / B2 / G13 |
| PJA-80 | HTTP 部署深链接及范围外功能检查 | H / B3 / S |

## 5 失败修复及验证边界

自验中修复了目录发布审计暴露项目成员细节、编号永久保留与 no-op upsert 冲突、严格 JSON null 输入、开发 StrictMode 范围取消导致的初次读取 AbortError、取消删除误触发页面成功状态，以及上层切换未清理新增治理缓存。分别由业务、HTTP、浏览器回归验证。

完整旧测试中 4 处仍断言 19 项租户权限/审计定义；本轮明确新增 3 项治理权限后调整为 22 项（13 普通/9 保留），未放宽权限校验。保留初次失败日志 `go-full-final.log` 和修复后完整日志。运行手册使用实际存在的 `access sync --dry-run`，而非不存在的 `--preview`。

尚未验证或不属于本轮：生产部署、生产容量、长期压力/稳定性、现场设备集成、Safari/Firefox、用户真实工作流人工验收。自动化及截图观察不替代用户体验确认。范围排除配额/用量、资源目录/资源使用授权及设备监测业务。

## 6 交付及用户验收

按[运行手册第 1 节](../operations/project-management.md#1-当前本机与启动)启动，按第 4–6 节准备账号并执行 17 步验收。用户确认前保持 Awaiting acceptance；自验通过不能记为 Accepted。

- 自验完成日期：2026-11-10；完整回归及最后改动定向回归均通过。
- 用户人工验收：未执行 / 未确认。
- 本机运行：最新 `make dev` 服务已启动；`/healthz`、`/readyz` 均为 200 / `status=ok`。`/projects` 入口返回 200，实际浏览器进入登录页且无运行错误。证据：`local-ready.json`、`local-browser.json`、`screenshots/local-login.png`。
- Git 提交、推送：未执行。
- 生产部署：未执行。
