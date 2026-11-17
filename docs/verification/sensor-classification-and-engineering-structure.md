# 设备分类与工程结构验收账本

状态：**Awaiting acceptance（待人工验收）**。开发范围为方案的 SBM-01—11、SBM-A01—60；自验证据和用户人工接受分别记录。用户尚未验收，所有人工验收项均为「待验收」。

依据：[完整方案](../development/sensor-classification-and-engineering-structure-proposal.md)、[实施规格与技术预算](../development/sensor-classification-and-engineering-structure-implementation.md)。2026-11-17 已完成全部开发、自验及本地运行更新；本页的通过仅指软件自测，不代表用户已接受。

## 证据索引

源文件均在仓库内；运行输出位于 `.artifacts/sensor-structure/`。代码测试启用真实 PostgreSQL／Redis，带标签的监测、空间及恢复测试另启用 ClickHouse 和对象存储。普通无数据库 `go test` 的 skip 不作为通过证据。

| 编号 | 实际检查 |
| --- | --- |
| C1 | `classification/tree_test.go`：`TestCategoryTreeIdentityDepthAndNoImplicitLeafMigration` |
| C2 | `devicecatalog/classification_integration_test.go`：`TestDeviceClassificationReleasesInheritanceAndFixedBatch` |
| E1 | `engineering/structure_test.go`：元模型兼容、12 组合、最近构件约束、模板真实祖先、两个公共依赖读取器 |
| E2 | `engineering/structure_template_integration_test.go`：`TestStructureTemplatesPublicTenantCopyAndCategoryBasis`；`template-db3.log` |
| R1 | `resources/structure_model_test.go`：`TestStructureModelAdoptionMixedTreeMovesAndProjectContext` |
| R2 | 同文件 `TestBoreholePositionReferenceAndMoveEvidence`；定位、参照与定位历史 |
| R3 | `resources/structure_authorization_test.go`：隐藏后代、定位权限及并发相向移动 |
| R4 | resources 原有 lifecycle／project access／project boundaries／project races 全量数据库回归 |
| I1 | `resources/structure_installations_test.go`：安装、新实体 SDK 统计及旧安装文字核对；`installations-db3.log` |
| D1 | `resources/display_structure_test.go`：`TestDisplayStructureCanonicalCountsAndClassification`；`structure-authorization.log` |
| D2 | `resources/display_structure_monitoring_test.go`：`TestDisplayStructureSeventyFormalObservations`；`structure-seventy.log` |
| D3 | `resources/display_structure_edges_test.go`：typed identity、同 UUID 不同 kind、真实派生所有者、旧范围拒绝；`typed-owners-db.log` |
| D4 | resources 现有 fixed query／worker lifecycle／recursive revocation 检查；监测恢复演练也复核过期游标与终端凭据 |
| D5 | `resources/display_structure_identity_test.go`：同名类型分开、同定义不同版本合组、兼容采用历史及旧下钻；同名分类、父级直接归类、互斥前沿和字典移动后的固定查询；`identity-and-snapshots-final.log` |
| S1 | `resources/structure_spatial_test.go`：PDF／GLB 显式区域、路径、平面，发布／复制／SDK／撤权；`spatial-db2.log` |
| F1 | `resources/structure_files_test.go`：固定附件版本及交付块重新授权 |
| M1 | `resources/structure_migration_test.go`：从实际 v46 历史 schema 升级到 v54；`scale-and-upgrade-db.log` |
| M2 | `TestDisplayReadMigrationFormatsAndDowngrade`：实际历史 v44 二进制拒绝新库，旧格式与降级阻止；`old-binary-compatibility.log` |
| M3 | `TestMonitoringIndependentRestore`：源存储暂停后独立恢复 PG／CH／对象存储，重新配对终端复核分类、结构、定位、参照、来源和观测；`joint-restore.log` |
| M4 | 本地实际 v44→v54 升级；36 张业务表共 8,029 行保留字段摘要相同，包含授权、来源、安装、空间及附件；`local-upgrade/result.json` |
| B2 | `scripts/sensor-structure-ui.mjs`：新建／移动、分类覆盖和清除、孔位安装、平台混合样例、公共／租户模板；`browser/ui-result.json` 通过 |
| B3 | 持久本地环境真实管理员登录和 SDK 计数验证；`local-acceptance/result.json` 通过 |
| L1 | `structure_scale_test.go`：623 个资源、620 个断面、三种适用性及 512 层／附属通道；`domain-database.log`、`query-plans.log`、`plans-final/` |
| B1 | `scripts/sensor-structure-browser.mjs`：通过正式 HTTP 创建 9.5 合成场景，再以真实 Console／SDK／Bridge／终端读取；`browser/result.json` 通过 |

以上 Go 路径均相对 `packages/go/platform/`。最终整批结果记录于下表。早期诊断中的失败已修复并复测，最终判定使用完整通过日志。

## 逐项追溯

| 验收 | 可观察行为 | 实现／自测依据 | 自测状态 | 人工验收 |
| --- | --- | --- | --- | --- |
| SBM-A01 | 任意层分类与稳定身份 | C1、C2 | 通过 | 待验收 |
| SBM-A02 | 循环／跨字典／自引用拒绝，停用保留解释 | C1、C2 | 通过 | 待验收 |
| SBM-A03 | 父类新增子类不自动迁移设备 | C1、C2 | 通过 | 待验收 |
| SBM-A04 | 改名／移动发布与旧版本解释 | C2、D1、D5 | 通过 | 待验收 |
| SBM-A05 | 产品默认独立于输出及产品内容 | C2 | 通过 | 待验收 |
| SBM-A06 | 默认、覆盖、清除覆盖与历史 | C2 | 通过 | 待验收 |
| SBM-A07 | 按设备去重，类别按 ID 区分 | C2、D1、D5 | 通过 | 待验收 |
| SBM-A08 | 未分类、非传感器、不可用分别投影 | C2、D1 | 通过 | 待验收 |
| SBM-A09 | authorized=6/4，monitoring_bound=4/3 | D1、B1 | 通过 | 待验收 |
| SBM-A10 | 直接分类、包含与互斥前沿口径 | D1、D5 | 通过 | 待验收 |
| SBM-A11 | 设施可直接容纳四种结构对象 | E1、R1 | 通过 | 待验收 |
| SBM-A12 | 三种容器与四种下级的 12 组合 | E1 | 通过 | 待验收 |
| SBM-A13 | 嵌套容器；测点／通道不可作为容器 | E1、R1 | 通过 | 待验收 |
| SBM-A14 | 唯一父边、跨范围拒绝、并发防环 | R1、R3 | 通过 | 待验收 |
| SBM-A15 | 最近构件约束及数量限制 | E1、R1 | 通过 | 待验收 |
| SBM-A16 | 独立实体资料、状态、历史及空节点 | R1、B1 | 通过 | 待验收 |
| SBM-A17 | 断面类别源于设施采用类型 | D1 | 通过 | 待验收 |
| SBM-A18 | 设施类型按定义身份分组，保留采用版本 | D1、D5、R1 | 通过 | 待验收 |
| SBM-A19 | 参照／旧文字不制造实体数量 | R2、M1、D1 | 通过 | 待验收 |
| SBM-A20 | 一孔多点，来源替换不复制实体 | R2、I1、D1 | 通过 | 待验收 |
| SBM-A21 | 距离、埋深、高程、基准及 0／未知 | R2、B1 | 通过 | 待验收 |
| SBM-A22 | 未知孔深和超深异常依据 | R2 | 通过 | 待验收 |
| SBM-A23 | 最近孔位变化需明确定位处理 | R2 | 通过 | 待验收 |
| SBM-A24 | 子树移动保留身份／来源／观测 | R1、R2、D2 | 通过 | 待验收 |
| SBM-A25 | 隐藏或无维护委托后代拒绝且不泄露 | R3 | 通过 | 待验收 |
| SBM-A26 | 退役／恢复历史，引用阻止删除 | R1、R4 | 通过 | 待验收 |
| SBM-A27 | 共享身份与项目独立授权 | R1、R4 | 通过 | 待验收 |
| SBM-A28 | context 不计实体且不泄露隐藏兄弟 | R1、D1 | 通过 | 待验收 |
| SBM-A29 | 新种类明确授权，移动不扩权 | R1、D1 | 通过 | 待验收 |
| SBM-A30 | 安装与供数关系独立 | I1、D1 | 通过 | 待验收 |
| SBM-A31 | 直接归测点，派生可归断面／孔位 | D3、D4 | 通过 | 待验收 |
| SBM-A32 | 显式空间几何及授权交付 | S1 | 通过 | 待验收 |
| SBM-A33 | 参照不形成主边或传播权限 | R2 | 通过 | 待验收 |
| SBM-A34 | 9.5：5 断面、4 孔、7 点，类型 4/1 | D1、B1 | 通过 | 待验收 |
| SBM-A35 | nearest 合计 7，S1 subtree=3／nearest=2 | D1、B1 | 通过 | 待验收 |
| SBM-A36 | 70 条正式观测及 unassigned 分组 | D2、B1 | 通过 | 待验收 |
| SBM-A37 | 新查询随变更更新，旧下钻固定且复验权限 | D1、D2、D4 | 通过 | 待验收 |
| SBM-A38 | 汇总权限不提供设备名单，context 输出不计完整设备 | D1 | 通过 | 待验收 |
| SBM-A39 | 旧 ID／路径／授权／来源／空间／文件对账 | M1、M4 | 通过 | 待验收 |
| SBM-A40 | 旧 SDK 合法投影，新所有者明确升级 | D1、D3、M2 | 通过 | 待验收 |
| SBM-A41 | 旧编辑省略父级保留主边，歧义写入拒绝 | R1 | 通过 | 待验收 |
| SBM-A42 | 平台／设备／设施／真实 SDK 和终端操作 | B1、B2、B3 | 通过 | 待验收 |
| SBM-A43 | 归档只读，查询／分页／后台／空间／文件撤权复验 | R3、R4、F1、S1、D4 | 通过 | 待验收 |
| SBM-A44 | 独立恢复与旧写程序降级保护 | M1、M2、M3 | 通过 | 待验收 |
| SBM-A45 | 明确规模预算，623 资源、620 断面分页及 512 层 | L1 | 通过 | 待验收 |
| SBM-A46 | 60 项账本与人工接受分离 | 本账本及完整检查记录 | 通过 | 待验收 |
| SBM-A47 | 类型固定版本显式能力声明 | E1、E2 | 通过 | 待验收 |
| SBM-A48 | 无孔类型禁止孔位；允许零孔有效 | E1、L1、B1 | 通过 | 待验收 |
| SBM-A49 | 三种适用性与项目汇总 | E1、L1 | 通过 | 待验收 |
| SBM-A50 | 样例与实建共用校验，层序不强制 | E1、R1 | 通过 | 待验收 |
| SBM-A51 | 模板锚点与实际测点通道语义 | E1、E2 | 通过 | 待验收 |
| SBM-A52 | 不支持孔位不能被禁用项／漏映射掩盖 | E1、E2 | 通过 | 待验收 |
| SBM-A53 | 逐类型映射、复制、固定依赖及公共更新隔离 | E2 | 通过 | 待验收 |
| SBM-A54 | 模板按真实祖先匹配，允许混合容器 | E1、E2 | 通过 | 待验收 |
| SBM-A55 | 预览无建档／绑定副作用，分类不改输出语义 | E1、E2、C2 | 通过 | 待验收 |
| SBM-A56 | 元模型 1/2、两类依赖读取及旧摘要 | E1、E2、M1 | 通过 | 待验收 |
| SBM-A57 | 显式兼容采用、预览、权限和历史 | R1、D5 | 通过 | 待验收 |
| SBM-A58 | 保留孔位／引用阻止无孔采用，拒绝跨定义 | R1、D5 | 通过 | 待验收 |
| SBM-A59 | 当前位置移除文字断面，旧写入口拒绝 | M1、I1、R2 | 通过 | 待验收 |
| SBM-A60 | 逐条迁移证据与显式核对，同名不合并 | M1、I1、R2 | 通过 | 待验收 |

## 已完成的专项结果

- v46 历史样本中的 14 条文字断面出现记录全部保留：8 条当前来源、6 条历史来源。重复同名记录不合并，未自动生成断面实体；旧产品内容摘要、通道、安装目标与历史保持一致。
- 独立恢复演练通过：源 ClickHouse／对象存储暂停后，恢复环境仍能读取 5 断面、4 孔位、6 设备、4 传感器、3 个实际使用类别和获准观测；旧游标与终端凭据失效，新配对终端可读。
- 97 项前端／客户端单元检查通过。三个外部 SDK 模板均完成离线安装、构建、校验和打包；工程结构样例包位于 `.artifacts/display/engineering-structure-example.zip`。
- `pnpm check`、最终 ESLint、完整生产构建和最终 Console 构建均通过。
- 本地 v44 数据已先备份，再迁移至 v54。与恢复出的 v44 基线逐表对账，36 张表保留字段摘要一致；原 1 个账号、2 个租户、3 个项目、15 台设备和 15 个文件均保持，之后另外创建独立合成验收租户。

## 完整检查记录

| 检查 | 结果 | 最终证据 |
| --- | --- | --- |
| Go 格式／vet、ESLint、TypeScript、OpenAPI、权限及契约一致性 | 通过 | `static-check.log`、`final-lint.log`、`delivery-check.log` |
| 所有应用及 Go 服务生产构建 | 通过 | `build.log`、`console-build.log` |
| resources／engineering／classification／devicecatalog／devices／display／displayread／spatial 领域包全量数据库回归 | 通过 | `domain-database.log`；无测试的包单独标记，不计为用例 |
| API HTTP 全量回归及子包 DTO／边界检查 | 通过 | `http.log`；真实文件检查器已启用 |
| 6 项 race 联合场景：递归派生撤权、70 观测、隐藏后代、并发移动、附件交付、空间几何与 SDK | 6/6 通过 | `final-integration-race.log` |
| 类型／分类稳定身份、采用历史、分类前沿及跨发布固定查询 | 2/2 race 数据库验证通过 | `identity-and-snapshots-final.log` |
| 前端／SDK／客户端单元检查 | 97/97 通过 | `unit-all.log` |
| 623 资源／620 断面分页与统计，SQL 执行计划 | 通过 | `query-plans.log`、`plans-final/`；未提高正常语句预算 |
| 旧 schema 迁移、旧二进制拒绝、独立联合恢复 | 通过 | `scale-and-upgrade-db.log`、`old-binary-compatibility.log`、`joint-restore.log` |
| 三个外部 SDK 模板离线构建及打包 | 3/3 通过 | `external-sdk.log` |
| 真实浏览器、平台／设备维护、移动端与终端配对 | 通过，无 pageerror | `browser/result.json`、`browser/ui-result.json`、对应 PNG |
| 本地实际历史数据迁移及持久验收项目 | 通过 | `local-upgrade/result.json`、`local-acceptance/result.json` |

复测脚本：`node scripts/sensor-structure-browser.mjs` 创建隔离验收环境；需要接着执行 `node scripts/sensor-structure-ui.mjs` 时先以 `INFRAPRO_SBM_KEEP=1` 保留该环境。`node scripts/sensor-structure-local-verify.mjs` 只验证现有本地验收项目；`sensor-structure-local.mjs` 仅用于明确准备独立合成验收租户，会拒绝覆盖已有分类字典。

## 人工验收路径

本地服务已启动，使用现有 `accept_admin` 账号及原密码。新增租户「设备分类与工程结构 · SBM 验收」，新增项目「SBM 完整验收项目」，不会混入原业务项目。

| 入口 | 操作与预期 |
| --- | --- |
| [设备分类字典](http://localhost:25173/platform/device-categories) | 平台发布维护，查看稳定类别与发布历史 |
| [设施 A](http://localhost:25173/t/281f7c81-7c0d-47ba-8f1c-a074cd2eeddf/resources/facilities/139d4244-86b5-4b7a-b28b-325bd368a59f) | 查看断面／孔位／测点的混合父链和定位 |
| [设备台账](http://localhost:25173/t/281f7c81-7c0d-47ba-8f1c-a074cd2eeddf/devices) | 六台设备、双输出、产品默认／设备覆盖 |
| [真实大屏](http://localhost:25180/display/07f21895-a9f2-4246-83cd-624473e5ea81) | 登录后点击继续，读取该项目正式授权数据 |

大屏实例授权有效至 2026-12-17；到期可在该项目的大屏实例管理页明确续期。服务停止后，在仓库运行 `pnpm local:start`；状态检查为 `pnpm local:status`。仅更新代码时先按项目运维流程构建、迁移及重启，不对现有库重新 bootstrap。验收场景状态位于运行目录的 `engineering/sensor-structure-acceptance.json`，不含登录凭据。


1. 平台设备分类：查看多层分类，改名／调整父级，保存草稿、预览后发布；原类别身份保持，已有设备不会因新增子类迁移。
2. 平台设施类型和模板：查看明确支持孔位与明确不支持孔位的类型；混合样例允许断面／孔位／构件穿插。公共模板逐类型声明适用性，租户副本仍保留固定版本。
3. 设施 A：核对 S1→H1→P1/P2、S1→S1a→P3、C1→S2→H3→P6、H2→S3→P4，以及设施直属 P5；设施 B 有 S4→H4→P7。
4. P1 孔内定位：沿孔距离 0 与未知垂直深度分别显示；移动导致最近孔位改变时必须明确重新定位或保留历史待补。
5. 设备目录：六台设备按产品默认与覆盖解释；分类调整保留双输出与来源。安装到断面／孔位不会自动绑定测点通道。
6. 大屏工程结构页：设施 2、断面 5、孔位 4、测点 7；nearest 与 subtree 明确不同。分类页完整授权设备 6／传感器 4，供数设备 4／传感器 3；观测页 70 条正式合成观测。
7. 移动、类型采用、项目权限与旧文字核对：先预览影响，再提交；旧分类／结构下钻保持固定依据，撤权后的读取失效。历史文字逐条可保留依据、映射或待核对。

## 验证边界

合成业务资料来自真实服务、权限和数据库链路，用于验收行为；并非现场实测数据。已验证超过旧 500 目标的分页及 512 层深度边界，不将其称为生产 100,000 节点容量认证。未开展长期生产浸泡或现场设备实接测试。用户人工接受必须另行明确记录。
