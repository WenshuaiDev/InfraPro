# 租户角色与成员授权验证记录

日期：2026-11-10。对应[已确认方案](../requirements/tenant-role-authorization-plan.md)及[启动与人工验收手册](../operations/tenant-role-authorization.md)。

- 用户已确认完整方案及第 19 节 D-01–D-12。
- 功能状态：**Awaiting acceptance**，本轮开发及自验通过，等待用户人工验收。
- 人工验收：**尚未进行，未标记 Accepted**。
- 工作区：`feature/account-access-console`，基线 `776663fc125223a02df6f46a24e5d0cf7663a8da`；在原有未提交的成员管理实现上增量开发，保留该迭代修改。
- 本机数据库已实际从 v4 升级到 v5；已恢复开发服务。没有提交、推送或部署生产。

## 1 完成交付

1. 平台统一目录共 45 项权限，其中 19 项租户权限：11 项可组合业务权限，8 项当前管理员保留能力；共 8 个内置角色，其中 6 个租户预置角色。
2. 预置角色直接分配、复制为独立本地角色、租户角色创建/资料/权限/启停/软删除、按租户名称约束、来源追溯及幂等创建。
3. 成员多角色与空集合，当前/已配置/恢复后权限解释及全部来源，暂停和不可用账号限制、目标覆盖、管理员交接兼容。
4. 服务端影响预览、相关状态指纹、版本冲突、逐成员权限并集、最后邀请创建权失去时的原子失效及完整审计。
5. 新迁移 `00005_tenant_role_authorization.sql`、数据库约束、只读 `access sync --dry-run`、按旧/新目录事实同步、无变化同步无副作用。
6. Go API、OpenAPI、生成的 TypeScript 类型与权限常量、console 路由/导航/权限编辑器/成员授权/只读预置目录，以及 375px 和普通 HTTP `/console` 部署支持。

## 2 自验命令和实际结果

环境为本机 macOS、仓库声明的 Go/Node/pnpm 工具链、Docker PostgreSQL 17 与 Redis 7.4。自动测试使用独立测试库或 schema，HTTP 测试使用独立 Compose 项目，不向本机验收库写入测试账号或租户。

| 检查 | 实际结果 | 说明 |
| --- | --- | --- |
| `pnpm check` | 通过 | gofmt、Go vet、ESLint、TypeScript、OpenAPI 和权限目录生成一致性 |
| `pnpm build` | 通过 | API、CLI、console 生产构建 |
| `pnpm test:identity` | 全量通过 | 所有 Go 工作区测试，包含 `-race -count=1`；身份、授权、租户、迁移、HTTP 路由和 CLI |
| 新增授权专项 `go test -race` | 通过 | 9 个租户角色测试、3 个竞争子场景、退役计算测试；后续补充副本/目标覆盖/审计/影响断言分别重跑相关测试 |
| `pnpm test:e2e` | 55 通过，2 跳过 | Chromium 桌面和 375px 手机，通用展示含 1024px；跳过的是仅生产构建可触发的 2 个懒加载错误恢复用例 |
| `pnpm test:e2e:preview` | 57 通过，0 跳过 | 生产构建全量回归；包括前述 2 个懒加载用例 |
| 更新后的角色页面生产构建专项 | 6 通过 | 覆盖审计事件中文筛选、权限选择与影响预览窄屏截图，以及完整角色流程 |
| `pnpm test:http` | 4 通过 | 非 localhost 普通 HTTP、`/console` 深链接登录返回、角色创建/授权/预览、CLI、依赖恢复、真实备份恢复和凭据撤销 |
| 本机 v4 → v5 | 通过 | 停旧 API、备份、独立库实际恢复、增量迁移、dry-run、同步及一致性检查；原业务记录保留 |
| 重复迁移/同步 | 通过 | dry-run 无变化；角色版本、全局修订及审计数均不变 |
| 当前开发服务 | 通过 | API `/readyz` 返回 200 与 `{"status":"ok"}`，console 返回 200 |

完整命令输出保存在忽略目录 `.artifacts/tenant-role-authorization/`；测试源码是可复现证据，不将本机备份、配置或测试会话纳入 Git。

本轮自验发现并修复：普通 HTTP 中 `crypto.randomUUID()` 不可用，角色表单改用已有的安全随机 UUID 工具；补充管理员交接后的迟到预览清理；更新交接候选响应契约断言。生产构建首次全量有 1 个原平台审计用例因并行测试将目标事件挤出第一页而失败，改为按本用例唯一原因筛选，随后全量 57 项通过。未使用自动重试掩盖失败。

## 3 可复现证据索引

| 代号 | 文件及测试 |
| --- | --- |
| R1 | [role_authorization_test.go](../../packages/go/platform/tenancy/role_authorization_test.go)：`TestTenantRoleUnionIsolationAndInvitationLoss` |
| R2 | 同文件：`TestTenantRoleCreationProtectionAndVersions` |
| R3 | 同文件：`TestTenantRoleDisabledSuspendedAndDatabaseGuards` |
| R4 | 同文件：`TestTenantRolePreviewConcurrencyAndAuditRollback` |
| R5 | 同文件：`TestTenantRoleSyncPreviewNoopAndOldFacts` |
| R6 | 同文件：`TestTenantRoleTargetCoverageAndMultiRoleHandover` |
| R7 | 同文件：`TestTenantRoleAssignmentRacesAndRejoin`，含 remove/delete/accept 三类竞争 |
| R8 | 同文件：`TestTenantRoleSyncConflictsAndReadiness` |
| R9 | 同文件：`TestTenantRoleReauthenticationAndLockTimeoutAreAtomic` |
| U1 | [tenant_retirement_test.go](../../packages/go/platform/access/tenant_retirement_test.go)：退役后预置及独立副本均不能提供该能力，配置历史仍保留 |
| M1 | [membership_upgrade_test.go](../../packages/go/platform/migrations/membership_upgrade_test.go)：真实 v3/v4 旧数据升级、凭据及关系保留、重复同步 |
| W1–W3 | [tenant-role-authorization.spec.ts](../../tests/e2e/tenant-role-authorization.spec.ts)：页面闭环、严格 HTTP 契约、交接后迟到预览；各跑桌面和手机 |
| H1 | [tenant-membership.spec.ts](../../tests/http/tenant-membership.spec.ts)：普通 HTTP 的角色流程、CLI dry-run、pg_dump/pg_restore、恢复后全局凭据撤销 |
| B1 | [membership_test.go](../../packages/go/platform/tenancy/membership_test.go)及[tenancy_test.go](../../packages/go/platform/tenancy/tenancy_test.go)：既有成员、邀请、交接及并发回归 |
| B2 | [access_test.go](../../packages/go/platform/access/access_test.go)、[access.spec.ts](../../tests/e2e/access.spec.ts)及全量身份测试：原平台授权、账号及即时拒绝 |
| B3 | [tenant-membership.spec.ts](../../tests/e2e/tenant-membership.spec.ts)及[tenancy.spec.ts](../../tests/e2e/tenancy.spec.ts)：租户切换、重入、迟到读写、复验、503 草稿及直接 URL |
| S1 | [00005_tenant_role_authorization.sql](../../packages/go/platform/migrations/00005_tenant_role_authorization.sql)：scope/归属/保留能力、未撤销绑定、唯一管理员、软删除及停用新增约束 |
| S2 | [tenant_roles.go](../../packages/go/platform/access/tenant_roles.go)、[tenant_role_changes.go](../../packages/go/platform/access/tenant_role_changes.go)、[tenant_authorization.go](../../packages/go/platform/access/tenant_authorization.go)：实时鉴权、白名单、指纹、并集与审计核对 |
| S3 | [tenant_catalog_sync.go](../../packages/go/platform/access/tenant_catalog_sync.go)、[catalog_sync.go](../../packages/go/platform/access/catalog_sync.go)：旧事实同步、只读快照、无变化、就绪核验及审计快照 |

专项复现（从仓库根目录，先运行一次 `pnpm test:identity` 启动隔离依赖）：

```bash
node --input-type=module - <<'JS'
import { execFileSync } from 'node:child_process';
import { testEnv } from './scripts/test-environment.mjs';
execFileSync('go', ['test', '-race', './packages/go/platform/tenancy', './packages/go/platform/access', '-run', 'TestTenantRole|TestTenantPermissionRetirement', '-count=1', '-v'], {
  stdio: 'inherit', env: { ...process.env, ...testEnv },
});
JS
pnpm test:e2e:preview tests/e2e/tenant-role-authorization.spec.ts
```

## 4 TRA-01–TRA-57 证据映射

以下逐项关联运行测试、数据库约束和源码核对；并非 57 条独立浏览器测试，也不是用户人工验收结论。具体手工操作及预期见运行手册第 4 节。

| 编号 | 自验依据与结果 |
| --- | --- |
| TRA-01 | R1、W1：19 项目录中 11 项可配置；保留能力单独只读展示 |
| TRA-02 | W2、S2、HTTP 路由/OpenAPI 核对：无租户新增权限或修改权限编码接口，未知字段拒绝 |
| TRA-03 | R2、W2、U1、S2：跨 scope、未知、重复及保留权限整体拒绝；退役定义不再提供能力，新增配置须通过 active 白名单 |
| TRA-04 | W2、B2、S1：平台只读租户预置目录；账号与成员绑定的 scope 分离 |
| TRA-05 | R1、W1：直接授资料维护员，同一会话获得本租户修改权 |
| TRA-06 | R2、S2：预置定义写入口统一保护；页面仅提供合法复制动作 |
| TRA-07 | R2、W1：副本保存 source_role_id/source_role_version，没有自动绑定，权限独立 |
| TRA-08 | R5：模拟旧共享定义后同步，直接使用者更新、副本版本及权限不变；S3 按成员所在租户写审计 |
| TRA-09 | M1、本机升级、S3：新增四个业务预置不创建成员绑定；自定义角色未自动扩权 |
| TRA-10 | R2、W2：空权限数组可创建；创建成功不写成员绑定 |
| TRA-11 | R2、R8：跨租户同名允许，同租户重名及预置名称冲突拒绝 |
| TRA-12 | R2：名称说明修改保持 ID/权限，精确断言审计仅包含实际字段及前后版本 |
| TRA-13 | R1：A/B 直接共用预置，A 解绑后 B 权限及邀请保持 |
| TRA-14 | R1、W1：多角色并集，资料查看记录全部三个来源 |
| TRA-15 | R1、W1：撤销一个重复来源后保留重叠权限，只撤独有能力 |
| TRA-16 | R1、W1：空集合保留关系、本人最小上下文可读，资料及目录拒绝；S2 读取不隐式补角色 |
| TRA-17 | R1、W1：无 role.read 时仍可看本人授权，完整目录返回 403 |
| TRA-18 | R6、S2：member.read 可读同租户目标，结果不含账号级角色；目标查询强制 tenant_id |
| TRA-19 | R3、W1：停用保留绑定、原 Cookie 新请求无独有权限；S2 逐成员计算并集 |
| TRA-20 | R1、R3、B1：恢复按账号/成员/角色当前状态计算；已失效邀请不会复活 |
| TRA-21 | R3、S1：停用旧绑定可保留或撤销，新增停用绑定拒绝 |
| TRA-22 | R3、S1：有效及暂停成员的在用角色不能删除；约束统计所有未撤销绑定，包含不可用账号 |
| TRA-23 | R3：暂停中改配置，当前权限为空、恢复后使用新配置及原关系；旧成员版本冲突 |
| TRA-24 | R3：不可用账号只能减绑定，新增返回 target_account_unavailable |
| TRA-25 | R7、B1、B3：移除再加入使用新关系、仅默认普通角色，旧绑定不恢复 |
| TRA-26 | R2、S1、S2：管理员不可复制/常规分配/解绑，数据库保护唯一管理员结构 |
| TRA-27 | B1：当前管理员暂停/移除及常规账号停用保护仍通过 |
| TRA-28 | R6、B1：多角色接任者复用关系，全部原角色撤销，仅固定管理员；原管理员关系结束 |
| TRA-29 | R6：暂停接任者明确拒绝，候选返回将替换角色而不隐式恢复 |
| TRA-30 | R6、S2：目标配置超出成员管理者覆盖范围时拒绝，补齐能力后可暂停/恢复 |
| TRA-31 | R6：成员管理者移除自己拒绝；暂停/移除共享自保护判断 |
| TRA-32 | R1：保留另一 create 来源时邀请仍可领取，不按单一角色失权误撤 |
| TRA-33 | R1、R4、R7、S2：解绑、改权限及启停共同比较实际 create 并集，失权及邀请失效同事务 |
| TRA-34 | R1：从完整邀请预置改为仅保留 create 的本地角色，移除 read/regenerate 不撤邀请 |
| TRA-35 | R1：同账号 A 失权后 A 邀请失效，B 邀请不变 |
| TRA-36 | R7 accept、B1 受控竞争：领取与撤权只有合法先后结果，无残缺关系；目录检查通过 |
| TRA-37 | B1、S2：失权处理仅命中 pending 邀请，不追溯撤销已接受邀请创建的关系 |
| TRA-38 | R3、R5、W1：暂停成员绑定数为 1、当前/实际失权为 0、恢复后影响为 1；邀请计数及分页明细分别计算 |
| TRA-39 | R4、W2：相关邀请产生后旧指纹返回 impact_changed；S2 指纹覆盖绑定/成员/账号/角色状态 |
| TRA-40 | R4：其他租户发生授权修订后原预览仍可提交 |
| TRA-41 | R2、R3、R4：旧角色/成员版本拒绝，两个并发角色提交只有一个成功 |
| TRA-42 | R7 remove/delete：授权与移除、授权与删除竞争，未产生 ended 关系新绑定或已删除角色绑定 |
| TRA-43 | R1、R3、B3、S1：租户参数、角色归属和数据库复合关系阻止串用 |
| TRA-44 | W2、S2：旧平台角色 PATCH 操作本地空权限角色返回 404，先检查 scope/owner |
| TRA-45 | W2、B2：builtin_key/owner_tenant_id/binding_scope/直接权限等不在 DTO 的字段返回 400 |
| TRA-46 | W1、B2、H1：原 Cookie 即时读取新授权，无需重新登录；服务端从数据库计算，不信任前端/Redis 旧权限 |
| TRA-47 | W3、B3：交接、切租户、重入及迟到响应清理旧资料与草稿；无关变更和暂时故障保留可用草稿 |
| TRA-48 | R4、R9、W2、B3：审计故障回滚，复验缺失拒绝，真实授权锁等待返回 operation_busy，CSRF 失败拒绝，复验不自动重放 |
| TRA-49 | R2、W2：同键同内容返回同一 ID，仅一份成功审计；不同内容返回 request_conflict；复制复用同一幂等入口 |
| TRA-50 | R2、W1、S2、S3：审计精确字段/身份/版本/目录说明快照，角色/成员事件按本租户查询；请求 DTO 不含密码或 token |
| TRA-51 | M1、本机升级：旧业务行及角色 ID 保留，重复 migrate/sync 无审计或版本噪声 |
| TRA-52 | R3、R5、S1：直接 SQL 跨租户、受保护权限、普通成员管理员绑定、在用删除及 builtin 归属伪造均拒绝 |
| TRA-53 | R5、U1、H1：dry-run 不写修订，按旧事实撤权；退役后副本也不提供能力，保留历史解释 |
| TRA-54 | R5、R8、M1：无变化同步不增版本/审计；目录校验值不匹配拒绝授权及就绪；迁移版本核验阻止混跑 |
| TRA-55 | R8：存量本地名与新预置名冲突出现在预览，真实同步中止、不重命名本地角色 |
| TRA-56 | W1–W3、B3：375px 纵向布局、权限搜索保留选择、确认影响、取消焦点、键盘、复验和错误反馈；截图人工查看 |
| TRA-57 | 全部 Go、开发/生产浏览器和 H1：原登录、本人账号、平台授权、成员/邀请/交接及 HTTP 部署回归通过 |

## 5 本机升级与恢复证据

停止原开发 API/console 后，对实际 v4 数据库生成自定义格式备份并保存匹配 `.env`（权限 0600）。使用临时独立数据库实际 `pg_restore`，比对 13 张业务表原字段的逐行摘要一致，再删除临时恢复库。

迁移与真实同步后：

| 本机对象 | 升级前 → 后 | 核验 |
| --- | --- | --- |
| 账号 / 密码记录 / 会话 | 1 / 1 / 2 → 1 / 1 / 2 | 原字段逐行一致，未重设密码或撤销会话 |
| 账号角色绑定 | 3 → 3 | 原行全部保留 |
| 租户 / 成员关系 / 租户角色绑定 | 1 / 1 / 1 → 1 / 1 / 1 | ID、状态及历史全部保留 |
| 租户邀请 | 0 → 0 | 未生成演示邀请 |
| 角色 | 5 → 9 | 原 5 个 ID、builtin_key 与 scope 保留；新增 4 个预置，8 个内置角色合计 |
| 权限目录 | 37 → 45 | 8 项新增管理员保留能力，11 项原租户业务权限补显式可配置属性 |
| 审计 | 12 → 14 | 保留全部原记录，新增平台同步及原租户管理员授权更新各 1 条 |
| 重复 migrate / sync | 无变化 | 角色版本、授权修订、审计数完全不变；dry-run changed=false |

备份、恢复比对和升级结果位于 `.artifacts/tenant-role-authorization/local-*`。这些文件包含私有环境数据，不上传或提交。隔离 HTTP 测试另外验证：恢复备份后执行 `account revoke-access`，旧会话、开户/重置及租户邀请不能继续使用，成员和角色仍保留。**没有对本机验收库执行该全局撤销操作。**

已重启 `make dev`，console 为 `http://127.0.0.1:5173/`，API 为 `http://127.0.0.1:8080/readyz`。继续使用原账号及密码；本轮不提供、复制或重置密码。

## 6 待人工验收与范围限制

- 用户按[运行手册第 3–4 节](../operations/tenant-role-authorization.md#3-人工验收准备)准备合法账号并执行 13 步验收；只有用户明确确认后才更新为 Accepted。
- 尚未进行用户人工验收或生产部署。没有在本机用原密码重新登录，保留凭据的证据来自原行比对；实际登录由用户验证。
- 自动浏览器覆盖 Chromium；未覆盖 Safari/Firefox、真实手机系统、辅助技术全量审计或大规模租户性能压测。
- 权限退役通过明确构造的旧/新目录计算测试、共享定义变化集成测试和源码核对验证；未对实际发布目录人为退役业务权限。旧版本拒绝服务通过版本/校验值门禁验证，未部署旧生产二进制混跑。
- 预置角色由代码发布维护；不包含平台网页动态发布、跨租户发布调度、项目角色、嵌套角色或单成员直接权限。前端以响应、聚焦及轮询更新；服务端新请求即时生效。
- 上一成员管理迭代的人工验收状态仍以[成员验证记录](tenant-membership.md)为准，本记录不代替用户接受上一迭代。
