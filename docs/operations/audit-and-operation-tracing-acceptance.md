# 审计与操作追踪人工验收

软件自验见[70 项账本](../verification/audit-and-operation-tracing.md)，人工状态只有用户明确确认后才能标记 Accepted。此环境为合成数据；现有持久人工验收环境、正式数据库和设备不参与本轮演示。

## 启动与账号

```sh
pnpm audit:demo
```

启动独立 schema、私有审计／附件桶、监测数据库、API、控制台、审计／任务索引／通知 worker，以及真实文件检查、监测处理、空间转换、大屏包检查。入口为 `http://127.0.0.1:15576/access/audit`，API 为本机 18576。大屏独立域使用 15860／15861；脚本遇到占用会退出，不结束已有进程。合成样本不需要用户上传资料或提供密钥。

本地私有材料：

| 文件 | 内容 |
| --- | --- |
| `.artifacts/audit/lab/accounts.json` | owner、platform、tenant、project、exporter、manager、business、rejoined 的合成登录资料；勿提交仓库或分享密码 |
| `.artifacts/audit/lab/audit-acceptance.json` | 两租户、各两个项目、归档／禁用空间、角色和重入成员 ID |
| `.artifacts/audit/lab/business-acceptance.json` | 真实监测／派生／空间／大屏／附件来源及固定修订；派生业务账号也在该文件中 |
| `.artifacts/audit/lab/session.json` | 本次进程、入口和运行状态 |
| `.artifacts/audit/lab/server.log` | 本地诊断；不要复制连接配置到工单 |

`owner` 有平台租户查看及来源管理资格；`platform` 没有任何租户成员关系；`tenant`／`project` 是分别授权的审计员；`exporter` 可申请租户导出；`manager` 有 read＋exports.manage、无 export；`business` 只有业务资料权限；`rejoined` 已移除再加入，具有新的成员关系。

Ctrl+C 关闭本次进程并清理所属隔离数据库与对象；不要删除公共 Docker 数据卷。重启会重新生成 ID 和密码，旧演示链接不再适用。首次启动需要仓库常规依赖及 `pnpm install`；空间 Python 环境沿用 `.artifacts/spatial-venv/bin/python`，具体准备按[空间运行手册](spatial-visualization.md)。

## 页面与业务核对

1. 用 owner 打开平台审计，按“创建角色”查看分页、详情、实际变更及历史名称来源。分页后刷新或浏览器返回，记录顺序及筛选保持。中文、引号、公式前缀、500 字原因样本能阅读。
2. 进入租户 1 的审计与项目 1 的审计，比较 scope；平台列表不出现租户／项目完整业务事实。租户审计员不能直接打开项目审计。归档项目 2 仍可按原资格读审计并导出，禁用租户 2 拒绝入口。
3. 打开账号／角色、租户资料／成员／角色／邀请、项目资料／成员／角色，点击“操作历史”（租户资料 B 方案与项目资料 A 方案中为“变更记录”）。URL 带固定对象筛选，从事件的关联来源返回原对象。
4. 用 business 登录，正常资料页可使用，没有“操作历史”或“变更记录”入口；直接输入审计地址也看不到旧账号的列表或 CSV。
5. 用 business-acceptance 中的来源 ID 打开工程定义、设施／构件／测点／通道、厂商／产品／设备及项目授权集合。历史中的对象身份明确，设备输出或安装可返回已授权父设备。
6. 打开监测批次／任务、派生通道／执行／分析成果，核对提交、实际终态、原发起人和 worker。任务链接带 execution，旧代不显示新代成果。没有逐行监测值审计。
7. 打开空间资料／版本和项目大屏实例／包，核对版本发布、授权与处理终态；包只有既有管理入口，没有新增包下载按钮。空间原件下载仍需原权限。
8. 打开普通附件及两个固定修订，核对真实上传、检查、绑定和替换事实。历史入口可返回同附件固定 revision，审计不能代替文件下载资格。

第 15.2 节入口清单、源码位置和自动检查见[验收账本](../verification/audit-and-operation-tracing.md)的 AT-60。无独立详情页的对象使用现有面板或固定筛选页，不新增空页面。

## 导出、权限与任务

1. 用 platform 提交平台导出，填写原因；应立即进入“已接受／排队”任务。离开页面后可从任务中心找到同代任务，完成后下载带 BOM 的中文 CSV。
2. 用 exporter 在租户 1 申请导出；manager 可看有限任务元数据并取消活动任务，没有他人 CSV 下载或代办 retry。所有人 retry 产生新 generation。
3. 在已完成导出的账号上修改角色或精确成员关系后，再打开旧下载。旧成果应明确失效；恢复权限后仍需重新生成。旧游标要求按当前权限回第一页。
4. 取消排队任务应实际取消；取消正在执行的任务先显示“取消请求已接受／正在取消”，只有来源终态确认才显示实际取消。完成与取消竞争时显示真实状态或冲突，不伪造取消成功。
5. 24 小时到期后正文不可用，任务仍为成功且结果到期。自动自验用可控时钟验证边界，不要求人工等待，也不修改系统时间。

## 停机、通知和缓存

创建 `.artifacts/audit/lab/worker.paused` 暂停审计 worker；删除它恢复。`index.paused` 控制任务索引 worker，`notifications.paused` 控制通知投递。暂停文件只作用于本次实验进程。

暂停通知投递后申请导出：提交及任务执行照常，审计显示待投递；恢复后同代只有一项通知事实。关闭本人任务通知偏好后，同一事实标记抑制。审计摘要不展示他人的正文、偏好、已读或归档状态。

切换账号或空间时列表、详情、在途请求和 blob URL 应清理；返回时重新授权，不能短暂显示旧空间数据。来源故障显示暂不可核实，空筛选显示无可见记录，两者文案不同。移动端 375px、200% 缩放和纯键盘打开／关闭详情，焦点返回原记录且页面可操作。

## 自动复验与维护

```sh
pnpm check
pnpm test:audit
pnpm test:audit:restore
pnpm audit:browser
pnpm test:audit:regression
pnpm test:monitoring:restore
pnpm test:monitoring:restore --legacy
pnpm audit:catalog
```

按顺序执行，不同时启动多个全量 schema 回归。`test:audit:regression` 按 governance、resources、metadata、compatibility、files-restore 顺序运行；也可传对应阶段名定向复验。resources 中需专门恢复实验库的旧监测升级／恢复用例由原 `pnpm test:monitoring:restore` 和 `--legacy` 脚本执行，不能把 skip 当作通过。

`audit:browser` 独占本机 15576／18576，运行前应关闭本次 `audit:demo`。随后重新启动 `pnpm audit:demo`，等待全部合成样本就绪，在另一终端执行 `pnpm audit:history`，逐类点击上述业务详情的历史入口、打开实际事件并返回来源。运行全部检查后执行 `pnpm audit:delivery` 核对交付材料、恢复结果和演示存活，再执行 `pnpm audit:acceptance` 生成并检查 70 项证据账本；缺少任何必需结果会失败。

70 项含 SQL 最小权限、提交响应丢失、进程 SIGKILL、精确边界和真实独立恢复等不宜手工破坏环境的场景，以逐项账本中的实际测试结果为证据。人工重点核对业务可理解性、入口、权限差异和完整链路。运行／清理／备份步骤使用[运维手册](audit-and-operation-tracing.md)中的有限 CLI。

本次不代表生产部署、迁移执行、真实硬件接入、长时间压力运行或用户人工接受；这些状态与本地软件自验分别记录。
