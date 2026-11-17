# 生产版本基线（2026-11-17）

状态：**Awaiting acceptance**。生产应用代码与本次 Git 版本的对应关系已经自检确认；业务功能的人工验收单独记录。

**Git 标签 `production-2026-11-17` 指向本次提交的全部工作区改动，其应用代码就是生产服务器正在运行的版本。** 生产使用先前从同一工作区构建的发行包 `20261117-802a92f81b63`，本次通过源码逐文件摘要、发行文件摘要和运行中程序摘要建立提交与生产的对应关系。

## 版本标识

| 项目 | 固定值 |
| --- | --- |
| Git 仓库／分支 | `WenshuaiDev/InfraPro`／`main` |
| 生产基线标签 | `production-2026-11-17`；准确提交号以 `git rev-parse 'production-2026-11-17^{commit}'` 为准 |
| 服务器发行目录名 | `20261117-802a92f81b63` |
| 构建目标 | Linux amd64，`GOAMD64=v1` |
| 生产数据库版本 | schema **55**；从业务 PostgreSQL 的 `goose_db_version` 实际查询确认 |
| 原始源码清单 SHA-256 | `802a92f81b63c5161284ba526458d31b1d405d28b7bdb92548efc6e2bebd4a48` |
| 发行文件清单 `SHA256SUMS` 的 SHA-256 | `037c90be23d436e6562bbda5820e6e3425f406534a1d303d86195a5742a2b6c1` |
| 构建快照非文档路径集合摘要 | `035d25595cb60c8b5475e2e5080840d70848a64c6abf230b3a5498b9177edcda`，计算规则见下文 |

源码清单记录的旧 HEAD `9c744acfa26a3f754eb0710cd9052d5e5e8be9a7` 是构建时的父提交；当时发行包含尚未提交的工作区改动，因此旧 HEAD 不能单独代表完整生产代码。此次生产标签及服务器 `source-commit.json` 补全正式提交映射，原始 `release.json` 和 `SHA256SUMS` 保持可校验。

服务器发行目录：37／38 为 `/home/gzzx/env/infrapro/releases/20261117-802a92f81b63`；151／152 为 `/home/uadmin/env/infrapro/releases/20261117-802a92f81b63`。每台目录下的 `source-commit.json` 保存本次完整提交号、Git 树、标签和原始发行摘要。

## 自检证据

2026-11-17 16:59—17:01（Asia/Shanghai）通过既有 SSH 登录实机检查：

| 检查 | 结果 |
| --- | --- |
| 源码对应 | 原始清单含 2,292 个文件；除根 `README.md` 和 `docs/` 外的 **2,233 个文件逐项一致**，无新增、缺失或内容差异 |
| 四台发行文件 | 37、151、152、38 各 **578 个文件**全部通过 SHA-256 校验，清单本身也与本机构建包一致 |
| 152 业务进程 | API、monitoring／notification／task-index／spatial／display-package／files／audit／ingest worker、display-runtime、files-checker、parser，共 **12 个**处于 active/running；逐个核对 `/proc/<pid>/exe` 的真实路径与文件摘要 |
| 151 接入进程 | ingest-relay 处于 active/running；运行中二进制与发行清单一致 |
| 数据依赖 | 37 的 PostgreSQL／Redis／业务 S3、151 的 ClickHouse／Keeper／relay PostgreSQL／历史 S3、38 的备份 S3 进程处于运行状态 |
| HTTPS 入口 | 校验证书链后，`/healthz`、`/readyz`、`/console/` 返回 200；首页及其直接引用的 69 个 JS／CSS 文件与发行摘要一致 |
| 当前源码静态检查 | `pnpm check` 通过：Go 格式／vet、ESLint、TypeScript、接口与权限契约 |
| JS 单元测试 | `node --test tests/unit/*.test.mjs`：**103 通过，0 失败、0 跳过** |
| 差异格式检查 | 暂存文本差异检查通过；排除 PDF 样本，其原始字节由源码摘要校验保留，不按文本清理空格 |

本次核对开始时，工作区与发行源码仅有 `docs/README.md`、`docs/development/mfa-development-spec.md`、`docs/development/mfa-proposal.md` 三份文档差异；此次另增加生产版本说明和历史文档指引。文档更新不改变生产应用程序，MFA 规格不代表 MFA 已实现或上线。

Git 现有 `core.autocrlf=input` 配置将 `third-party/document-preview/docx-preview-LICENSE` 的 CRLF 换行保存为 LF；暂存树核对确认只有这一份上游许可证存在换行归一化，其文本内容不变。上述快照摘要采用原始工作区字节，其余非文档路径与暂存 Git 内容逐字节一致。

静态检查触发路由生成器调整 `routeTree.gen.ts` 的排序；确认行内容集合未变化后，恢复为检查前与生产源码逐字节一致的版本。没有修改业务代码，也没有重新启动生产服务。

源码集合摘要的计算规则：读取原始清单，排除 `README.md` 与 `docs/` 下文件，按路径排序，将每项拼成 `sha256 + "  " + path + "\n"`，连接后取 SHA-256。该规则固定对应上表 2,233 个文件。原始源码清单与构建日志位于本机 `.artifacts/production-deployment-20261117/`；本次实机 JSON、静态检查和单元日志位于 `.artifacts/production-version-20261117/`。这些本机证据不是随仓库发布的附件；服务器保留发行文件、清单和提交映射。

完整本机构建、全模块回归与专项测试的既有结果见[全仓回归记录](../verification/repository-regression-2026-11-17.md)。本次源码无业务差异，未重跑全套浏览器、负载、恢复和现场设备验收，也不将已有测试当作本次重新执行。

## 访问与人工核验

生产管理端：<https://10.23.52.152:18443/console/>。使用已建立的网络访问及内部 CA 信任链，以既有生产账号登录；不要重复 bootstrap。

1. 运行 `git rev-parse 'production-2026-11-17^{commit}'`，与四台发行目录 `source-commit.json` 的 `git_commit` 比较，预期完全相同。
2. 在发行目录运行 `sha256sum -c SHA256SUMS --quiet`，预期退出 0；检查 152 的业务服务及 151 的 ingest-relay，预期 active/running，启动路径指向上述发行目录。
3. 打开生产管理端，预期登录页面可用；登录后按实际业务范围验收项目、工程资源、设备、监测、大屏及文件流程。

版本一致性已经验证，不等于全部部署与业务验收完成。核验时独立 diagnostics-server 和 observer 尚未运行；主备、自动切换、真实设备联调及完整生产恢复演练也不由本次版本冻结证明。后续生产升级必须建立新的提交／发行包映射，不能移动本标签覆盖历史基线。
