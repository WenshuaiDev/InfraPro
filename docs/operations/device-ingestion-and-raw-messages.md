# 设备接入与原始报文：运行、模拟和恢复

状态：Awaiting acceptance。功能自验和用户人工接受分别记录在 [逐项验证报告](../verification/device-ingestion-and-raw-messages.md)。需求依据为 [开发规格](../development/device-ingestion-and-raw-message-spec.md)。本手册中的端口和账号均属于隔离合成环境；没有部署真实云服务器或接入现场设备。

## 一键启动本机人工验收环境

在本工作副本根目录执行，要求 Go、Node、pnpm、Docker Desktop 和 OpenSSL 可用：

```sh
pnpm install --offline
pnpm ingestion:lab
```

第一次会构建前端、API、独立解析服务、云中转和内网 worker，并创建专用业务 schema、云端 schema、私有 S3 桶和两天有效的测试证书。浏览器打开 `http://127.0.0.1:15579`；API 使用本机 18579。控制口和 E1—E5 监听口自动分配。账号及模拟发送配置保存在 `.artifacts/ingestion/manual-lab/accounts.json`、`acceptance.json`、`simulator.json`，目录仅限本机，配置权限 0600。不要将这些文件加入 Git、粘贴进日志或共享证书私钥。

`Ctrl-C` 停止本次启动的进程，清理其专用业务 schema 和桶；云端 schema 保留作为云缓冲证据，旧源证书到期后不能继续使用。再次启动建立全新隔离数据，旧业务 URL 不再有效。脚本不会清理已有业务数据库，也不会关闭共享测试依赖。若端口已占用，启动拒绝并要求先停止同一个验收实例；不会强占或杀死其他进程。已构建的前端可用 `INFRAPRO_INGESTION_SKIP_BUILD=1 pnpm ingestion:lab` 跳过重复 UI 构建。

测试依赖为：接入业务 PostgreSQL 56533、Redis 56379；云端独立 PostgreSQL 56532；监测契约环境的 S3 58333／备份 S3 58334 及其他既有依赖。两套接入测试 PG 使用独立持久卷，避免通用测试库 tmpfs 容量限制；容器由 `deploy/ingestion-lab/compose.yaml` 管理。真实平台数据库与验收库分开。

## 人工验收顺序和预期结果

1. 用 `accounts.json` 的 platform 登录“协议与解析方法”。原三种固定测量版本及新增 Reading 测量方法仍在测量目录；“接入能力目录”独立显示协议、识别／解复用和精确兼容组合。打开产品配置和设备实际采用，确认 V1 为 1.0.0、V2 为 1.1.0；识别专用方法不能选作设备测量方法。
2. 打开平台“接入入口管理”。应有 E1—E5，各有不同入口身份、命名空间、监听端口、厂家／租户／项目范围，状态为已生效，准确修订已由租户确认。打开详情查看确认历史、应用修订和状态观测时间。尝试用已占用监听建立入口应失败，不能出现两个已生效入口。
3. 使用 owners[0] 进入甲租户“设备接入与原始报文”。在“终端与关联”查看 E1 单路终端、00000000003 网关的 A/B 两个关联，以及机器识别／测量授权。E2 的两个终端分别对应 V1、V2。owners[1] 只能看到乙租户 E3；项目只读账号无租户接入维护权限。
4. 脚本启动时已发送一组样例。再次执行以下命令，查看自动接收、识别和解析，不需要逐条创建人工任务：

   ```sh
   pnpm ingestion:simulate .artifacts/ingestion/manual-lab/simulator.json --source E1 --split --coalesced
   pnpm ingestion:simulate .artifacts/ingestion/manual-lab/simulator.json --source E2 --duplicate
   ```

   E1 注册／注销是控制事实，不产生测量值；两次实际相同 DDP 上报保留两个接收事实。E2 同一幂等键重复发送仍为同一收据，288 字节厂家根中两个 81 字节帧分别解析；HTTP 接收传输与 TCP 帧解释类型独立展示。
5. “原始报文”按入口、设备、状态、方法版本、接收时间筛选。打开 E2 的 V1／V2 单元，核对两者均为 12.34 mm、采用版本分别为 1.0.0／1.1.0；点击 displacement 定位已核验单元的字节范围。下载只得到对应单元；普通接口不能下载完整厂家批次根。E1 网关 A/B 分别得到 12.34／8.5 mm 的独立 JSON reading，不会把 205 字节根交给浏览器裁切。
6. 在设备解析页修改产品默认，已采用设备保持原版本；显式采用预览展示接入兼容检查。仍有有效关联时清空或改为不兼容采用必须被拒绝。先停用所有冲突关联才能改变采用；原不兼容关联不能直接恢复。
7. 入口详情填写操作理由，暂停“自动规划”。云端仍接收并安全保管；恢复后继续处理。平台暂停“云端接收”须等云确认，断网时显示待核对；内网立即阻断与远端接收暂停有不同作用。
8. 打开已核验单元，选择“固定此单元重新处理”，填写理由，预览固定清单，再提交。应创建一个人员恢复任务并链接到准确执行代次；新识别决定和结果保留，旧结果不覆盖。取消／重试走任务中心；登记未知终端本身不会开放历史正文，须在未匹配区域固定历史接收清单和目标关联后补处理。
9. 使用错误路径或不属于入口的测试凭证发送，应拒绝且没有假成功：

   ```sh
   pnpm ingestion:simulate .artifacts/ingestion/manual-lab/simulator.json --source E2 --wrong-path
   pnpm ingestion:simulate .artifacts/ingestion/manual-lab/simulator.json --source E2 --wrong-credential
   pnpm ingestion:simulate .artifacts/ingestion/manual-lab/simulator.json --source E1 --unregistered
   ```

   错误凭证／未注册用例以非零退出表示预期拒绝；错误路径用例检查 403 后正常退出。`--unknown` 使用已认证的未知终端登记，合法帧进入待关联；不会把数据转给已登记的其他终端。
10. 用平台账号、另一租户账号和 projectReader 访问甲租户的原文深链接，均不能读取。项目设备页仅显示获准摘要。撤销来源项目的设备授权后，历史列表、正文、结果、任务和下载均按当前授权拒绝；不能用另一项目授权补足。
11. 在运行概况／独立诊断中分别检查接收、确认、识别和解析。断开内网 worker 后继续发送，再恢复 worker，检查云缓冲和确认逐步追平。状态观测过期显示未知。诊断报告没有原文、测量值、私钥或数据库密码。
12. 对照 [验证报告](../verification/device-ingestion-and-raw-messages.md) 的 136 项及旧 PAR 回归材料进行人工接受。正式观测、曲线和质量规则不会被本轮生产解析结果自动写入。模拟厂家包装和内层 JSON 都是联调示例，不代表实机协议认证。

## 发行构建、增量迁移与启动

```sh
pnpm ingestion:contracts
pnpm check
pnpm build
```

构建产物在 `build/bin/`，包括 api、ingest-worker、ingest-relay、parser、parser-runner、parser-package、ingestion-operator 和既有 CLI。旧解析方法不能由重编译覆盖：将上一发行包复制为新包基础，再运行新 parser-package，以保留原文件和方法身份：

```sh
cp -R /srv/infrapro/releases/parser-current /srv/infrapro/releases/parser-next
go run ./services/parser/cmd/parser-package build/bin/parser-runner samples/parsing /srv/infrapro/releases/parser-next
```

不同 CPU／OS 的原生可执行产物不能直接跨平台替换。每个平台独立构建并保留该平台历史产物；缺少原产物的迁移／回滚不能冒充兼容发行。平台已经注册的方法和兼容记录依赖精确摘要。

平台运行配置沿用既有 `INFRAPRO_CONFIG`，追加 `ingestion_file: /etc/infrapro/ingestion/worker.json`；可用环境变量 `INFRAPRO_INGESTION_FILE` 指定同一文件。未配置或 disabled 时不自动接入，旧台账、人工解析和历史读取保持可用。对象存储沿用配置中的私有 parsing／monitoring S3 凭证，但 `ingestion/` 对象由接入模块单独保管；不暴露公共桶或签名 URL。

1. 先按既有操作手册备份业务库、对象和旧发行包，停止旧后台写进程；使用同一发行包的 `infrapro database migrate` 增量迁移到该发行的 schema 版本，不运行 bootstrap 重建已有平台。空库首装也先完成迁移和权限目录同步，再按平台手册初始化一次管理员。
2. 在目标新包检查历史覆盖：`INFRAPRO_CONFIG=/etc/infrapro/runtime.yaml build/bin/ingestion-operator release /srv/infrapro/releases/parser-next`。必须包含当前数据库登记的测量、识别、协议、适配器和兼容身份。operator 与 API 均要求数据库版本精确匹配该发行的 [`migrations.Version`](../../packages/go/platform/migrations/migrations.go)，不再单独固定接入最初引入时的版本；缺失能力和不支持 schema 的包不得正常回滚。
3. 云端仅配置独立 relay PostgreSQL。复制 `deploy/ingestion/relay.example.json` 和 `principals.example.json` 到私有目录，替换节点 UUID、DB 地址、证书路径、批准的监听地址／端口与真实指纹，权限 0600。云端不要配置业务 PostgreSQL、S3 或用户权限库。
4. 分配 producer、consumer、control 三类 mTLS 身份。principals.json 键是客户端证书 DER 的 SHA-256 小写十六进制，不含冒号；consumer_id 必须与内网配置一致。producer 还须被准确入口的允许名单引用。端口和 URL 仅来自部署侧批准配置，租户不得填写可执行代码或云服务地址。
5. 执行 `build/bin/ingest-relay migrate /etc/infrapro-relay/relay.json`，再 `build/bin/ingest-relay serve /etc/infrapro-relay/relay.json`。仅此云进程监听设备 TLS／HTTPS 入口；控制 HTTPS 端口仅向内网允许出口开放。内网 worker 主动出站连接云控制端，不要求云访问业务内网。
6. 启动独立 parser、平台 API 和 `INFRAPRO_CONFIG=/etc/infrapro/runtime.yaml build/bin/ingest-worker`。parser 的原 token/TLS 配置不变。云节点登记时引用 worker 配置里的 configuration_ref，目录同步并启用后创建入口；租户确认准确修订，发布成功后才显示已生效。
7. systemd 样例见 `deploy/ingestion/systemd/`，替换用户和路径后安装。先停止旧进程再运行迁移，服务／迁移／new-epoch 互斥。正常重启保留 epoch；不得将 new-epoch 放到日常启动脚本。

`manifest` 的 `schema_version` 与 `release` 的 `schema` 均报告本次已核验的数据库版本。旧库或未来版本库会使 `manifest`、`release`、`objects` 返回退出码 1，并提示 `schema_incompatible` 和本发行要求的版本；不会生成成功清单。检查命令本身不执行迁移，也不修改版本记录。使用匹配的完整发行包，或按升级流程迁移后重新核验；不能修改 `goose_db_version` 绕过检查。版本号指业务库的迁移版本，不是 PostgreSQL 软件主版本，也不是独立 relay 库的版本。

证书轮换先签发新的受信证书，在指纹登记中添加稳定主体，再切换 producer／consumer／control 私有文件，最后撤销旧指纹。新握手重新加载服务端证书和 CA，每个 DDP 帧和 HTTP 请求再次验证当前客户端登记及有效期；撤销旧凭证不会等到连接重建才生效。测试证书有效两天只适合隔离验收；生产证书期限和信任域由现场运维确定。

## 容量、暂停和诊断

云端预算计算原文和持久包装的保守占用；80% 告警，90% 停止新接纳。HTTP 超限 503／413，DDP 不回成功 ACK。已提交未确认原文不删；确认后满七天可清理正文，收据、摘要、幂等身份仍保留并计入预算。同一来源跨 epoch 的存量也占其预算。无效片段有独立小预算及速率限制，不能吃满正常数据额度。物理 PostgreSQL/WAL、备份和对象副本的额外容量仍由数据库／存储运维监测，不将逻辑预算等同于文件系统可用空间。

内网 storage_budget_bytes、queue_budget 和每个来源／终端机器授权 budget 都有边界；达到预算暂停拉取或规划，让未接管数据保留在云端。生产解析全局最多 4、每租户 2，机器调用共用严格输出验证；独立解析进程给人工请求保留执行空间。暂时失败最多五次并退避；确定性坏报文不热重试。扩大存储容量不能自动替代长期吞吐评估。

ingest-relay 报告 receivers、control、spool-maintenance，依赖独立云 PG；ingest-worker 报告 pull、relay-status、confirmation、identify、planning、parse、source-publication、association-watermark、recovery、object-maintenance。报告文件路径由 INFRAPRO_DIAGNOSTICS_REPORT_FILE 配置，纳入既有观察器部署清单。运行报告按真实调用记录 PG、S3、parser 依赖；识别／解析远端失败即使已保存重试，也报告对应 lane 失败。空闲时对象维护 lane 每分钟复查存储与 schema。运行概况保留接收、持久确认、识别、解析四类状态，不能从最新一次成功推出当前确定在线。

## 联合备份与恢复

1. 冻结入口配置、关联和采用变更，暂停云接收并等待实际应用；停止内网 worker，等待在途提交结束。不得用先后不同的运行中快照声称已经实现原子联合备份。
2. 分别备份业务 PostgreSQL、独立 relay PostgreSQL、私有 S3 全部 `ingestion/` 及既有 `parsing/` 对象、完整解析发行包、私有运行配置和指纹登记；记录各快照时间与边界。PG 用 `pg_dump -Fc`／`pg_restore`，对象复制后校验 manifest 里的长度和 SHA-256。配置和证书备份加密保管。
3. 导出保护清单，shell 预设 `umask 077`：

   ```sh
   build/bin/ingest-relay manifest /etc/infrapro-relay/relay.json > /srv/backup/cloud-manifest.json
   INFRAPRO_CONFIG=/etc/infrapro/runtime.yaml build/bin/ingestion-operator manifest /srv/backup/inner-manifest.json
   INFRAPRO_CONFIG=/etc/infrapro/runtime.yaml build/bin/ingestion-operator release /srv/backup/parser-release
   ```

   cloud manifest 不含 payload，但包含完整路由和身份，仍是受保护运维材料。inner manifest 记录当前 schema、所有固定能力和可用对象。生产后台全停后快照才能冻结接管与确认边界。
4. 在隔离目标还原两套 PG、S3 和完整包；先使用与备份 schema 匹配的完整发行运行工具，跨发行恢复须另按增量升级流程处理。业务 PG 的 `btree_gist` 扩展先安装；对象逐个核对长度和摘要。还原云库后、开始任何新接收前执行 `ingest-relay new-epoch CONFIG`，正常重启禁止这样操作。旧收据原 epoch 不变；新接收使用新 epoch、独立序号，绝不能复用旧接收身份。
5. 在恢复目标执行 `ingestion-operator manifest RESTORED_MANIFEST.json`、`ingestion-operator release PACKAGE`、`ingestion-operator objects REPORT.json`、`ingestion-operator reconcile CLOUD_MANIFEST.json`。清单输出使用尚不存在的受保护路径；核对恢复前后的版本、能力、对象与接收事实。`objects` 会写入核验报告，`missing_or_corrupt` 非零时退出码为 1；缺失或同长度但摘要错误的对象均不得通过。最后一项比较冻结的云导出与业务库：matched 正常，awaiting_transfer 仅用于云有正文且尚未确认的接收，gaps／conflicts 非零返回失败，不能静默丢失或改写第一次事实。
6. 缺原文先恢复可信副本并重新校验；缺历史产物先补完整发行包；云已确认而业务快照缺接收时明确报告缺口，依据联合备份恢复业务或由专门受控恢复流程重新建立投递，不修改原接收身份来绕过对账。本期 CLI 不提供自动改写云确认的快捷命令。
7. 恢复 worker 后未知 epoch 先受限保管。平台通过准确云节点／epoch 创建确认任务，租户重新确认关联边界后建立新时间线关联。旧计划继续使用旧快照，超时租约通过 fence 重新领取；迟到旧 worker 不得覆盖新结果。
8. 对账无缺口并验证关联边界后再恢复新接收和规划。保留恢复报告、补处理任务和人工接受记录。不得删除新表、重建方法或覆盖旧结果来“回滚”。

`pnpm test:ingestion` 包含数据库版本回归、管理员初始化前的空库 CLI 核验及联合恢复测试。联合恢复使用真实两套 PostgreSQL dump、独立私有桶和复制的解析发行包，实际执行 `manifest`、`release`、`objects`；故意缺对象、篡改对象、缺收据、缺识别产物都会验证失败路径。[部署前 B1 修复与复验](../verification/deployment-schema-consistency.md)记录本轮范围和人工验收步骤。

## 重跑自动验证和持续负载

```sh
pnpm test:ingestion
pnpm test:ingestion:load
pnpm ingestion:browser
```

本轮实际负载已按用户要求在82,031个结果时停止剩余解析；实际范围和原始退出状态见验证报告。以下是独立重跑完整负载的标准命令与完成条件，不表示本轮已经完成18万结果排空。

持续负载使用两租户、两项目、真实解析服务，同时以 DDP 和 HTTPS 厂家批次报送，总计每秒 100 个测量帧，持续 30 分钟；第 120 秒停止内网 worker，第 165 秒恢复，报送线程持续运行。报送结束后继续等待积压处理和逐条确认，最终必须得到 180,000 个结果、135,001 个接收事实，失败计划、重复默认计划、跨租户单元和待确认均为零。整个测试可能显著超过 30 分钟，接收能力与解析吞吐分别记录，不以发送结束代替完成。

每次结果位于 `.artifacts/ingestion/test-*/`；`.artifacts/ingestion/latest-tests.json` 指向最近运行，`load.jsonl` 保存发送速率、积压、延迟和最终对账，`result.json` 保存测试退出状态。完整负载默认保留自己的业务 schema 和私有 S3 桶以便复核，`retained-fixture.json` 记录其准确身份；云缓冲和证据文件也保留。数据均为合成样例，仍须计入本机磁盘用量。若主动设置 `INFRAPRO_INGESTION_KEEP_LOAD=0`，测试在对账后自动清理自己的业务数据，清理可能需要额外较长时间。

复核结束后，在仓库根目录用准确的成功会话目录执行：

```sh
go run ./tools/infrapro/cmd/ingestion-lab-cleanup .artifacts/ingestion/test-SESSION
```

此命令仅接受已成功完整对账的合成负载会话，固定连接本机验收业务库 56533 和对象服务 58333，按会话清单删除其专属 schema／桶；不会连接正式配置，也不删除云缓冲或本地证据。失败或中断的会话不由该命令清理，应先核查其故障证据和归属。短时调试可设置 `INFRAPRO_INGESTION_LOAD_SECONDS=60`；它不能替代规格要求的完整 30 分钟负载。
