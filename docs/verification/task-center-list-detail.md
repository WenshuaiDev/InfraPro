# 统一任务中心验收

2026-11-18 · 本机开发工作区 · 方案 B「列表与详情」 · 状态为待用户验收

## 业务与成果

使用江南交通基础设施监测中心现有分析配置，通过正式界面提交了两次计算。浏览器逐项核对了提交响应中的任务、作业和运行编号，任务详情返回的目标，以及成果版本的来源运行。桌面使用 1440 × 1000，手机使用 Chromium 的 390 × 844 视口。

| 提交 | 任务编号 | 运行编号 | 成果版本 | 结果行数 |
| --- | --- | --- | --- | ---: |
| 桌面新建成果 | `bc80655d-8b20-5bff-8cc7-3a6bf809d7e9` | `700c2195-1026-4723-928e-1cb88bf91b43` | V1 · `19ebbe9b-2567-428a-9830-7bab3173c381` | 36 |
| 手机重算同一成果 | `98aa595b-02c2-595a-b55d-a1a20748a89c` | `28eeea1f-ab76-4b7d-9301-3d31e28e4816` | V2 · `000df7ac-123b-458b-8907-906a7a8f4ace` | 36 |

两次发布共用成果 `908e7d70-195b-43d5-83d7-a62a1732543b`。发布 V2 后，V1 的旧作业地址仍然打开第一条任务及第一版成果。已完成的导出 `1AB92E82` 同时显示「结果访问受限」，目标接口也未返回文件位置。[本机业务链记录](../../.artifacts/task-center/list-detail/business-flow.json) · [桌面布局](../../.artifacts/task-center/list-detail/桌面-任务.png) · [手机详情](../../.artifacts/task-center/list-detail/手机-任务.png)

截图中的历史回算 `711DA8F4` 也已通过旧运行链接、任务详情和派生结果明细三处核对。结果页读取的是与运行 `febf8df1-39da-4a46-ac6e-18d5da2bc0d1` 关联的发布代次。[历史回算旧链接记录](../../.artifacts/task-center/list-detail/local-backfill.json)

## 手动复验

1. 登录本机开发账号后打开[任务中心](http://localhost:25173/tasks?mode=mine)。桌面点左列两条不同的任务，核对右侧编号、状态、进度和结果。把浏览器切至手机宽度，点列表项进入详情，关闭后可回到列表。
2. 从[历史分析](http://localhost:25173/t/64a4ebf9-c549-4706-94a9-117333b62d4b/monitoring/analysis)选择已有配置，填写时段、成果名称和原因，点「运行并生成完整成果」「查看本次任务」「查看结果」。在成果页选「换时段再算」可生成 V2。也可以直接对照保留下来的[桌面 V1 任务](http://localhost:25173/tasks/bc80655d-8b20-5bff-8cc7-3a6bf809d7e9?execution=1)和[手机 V2 任务](http://localhost:25173/tasks/98aa595b-02c2-595a-b55d-a1a20748a89c?execution=1)。
3. 打开[V1 的旧作业地址](http://localhost:25173/t/64a4ebf9-c549-4706-94a9-117333b62d4b/monitoring/jobs?job=14941737-6dc2-4393-8472-119763586f95)，点「查看结果」，应看到「固定成果 V1」。在[受限导出](http://localhost:25173/tasks/1ab92e82-4d39-5fdf-8866-4a571656ed4a?execution=1)可以看到已完成状态与结果权限提示。
4. 在租户或项目的「历史分析」「派生监测」「导入与批次」点「查看本空间任务」，检查工作空间选择。切换「我的任务」「管理视图」、类型、状态和搜索，翻页核对可见记录。统计跟随当前授权、工作空间和搜索条件。
5. 从顶栏「任务」进「运行概况」，选择工作空间和「查看自动运行」。[本机一条已完成的自动运行](http://localhost:25173/tasks?mode=mine&scope_kind=tenant&tenant_id=64a4ebf9-c549-4706-94a9-117333b62d4b&operations=open&automatic=64a4ebf9-c549-4706-94a9-117333b62d4b&run=7e8fbbff-a927-42a2-806f-aa1780f4d1b0)可打开本次发布的 37 行派生结果。真实失败、取消和重试可用下方的隔离浏览器回归复现。[自动运行与发布代次](../../.artifacts/task-center/list-detail/local-automatic.json)

## 自验与复跑

- 本机业务链使用当前开发服务、正式登录和真实计算。复跑 `node scripts/task-center-flow-check.mjs` 会在本机已有分析工作空间新建一个验收成果并生成两个版本。先用 `pnpm local:start` 启动本机服务。
- `pnpm tasks:browser` 启动独立的本机测试环境、管理端、API、数据库、对象存储和工作进程。覆盖租户和项目范围、权限变更、历史通知、执行代次、翻页、键盘、375 像素手机和 200% 缩放。额外通过业务界面完成批次采用回执、派生试算输出和固定导出文件下载。失败验证只在新建的测试通道中模拟执行租约丢失，使用正式接口重试、取消、再次重试并核对发布代次。默认浏览器证据目录为 `.artifacts/task-center/browser`。本次干净环境的 17 项检查全部通过。[浏览器检查结果](../../.artifacts/task-center/list-detail/review/result.json)

  自动运行 `146f1ee4-994c-45aa-8e4c-588622ef2cc2` 出现 `job_lease_lost` 后，依次经历重试排队、取消、手动重试排队、成功。取消期间连续 5 次读回同一状态与修订号。另一个无本空间权限的账号无法读取、取消或重试该运行。恢复后从手机打开发布代次 `72defd2a-834d-4bae-8d5f-a17dd6bc5c48` 的明细，并分别验证了旧作业和旧运行地址。[运行记录](../../.artifacts/task-center/list-detail/review/automatic-check.json) · [失败时的桌面](../../.artifacts/task-center/list-detail/review/自动计算-失败.png) · [恢复后的手机](../../.artifacts/task-center/list-detail/review/自动计算-恢复.png)
- `pnpm test:tasks` 已通过。覆盖真实来源、分租户授权、迁移保留历史、通知恢复、命令幂等、客户端授权变化等。运行得到 32 个资源集成用例通过，并通过任务、通知与 HTTP 包的相关检查及 3 个客户端用例。[测试日志](../../.artifacts/task-center/list-detail/domain-tests.log)
- 派生发布回归覆盖 ClickHouse 和对象存储发布、并发修正、主时钟时段和取消范围。已取消的原时段保留待计算输入，后续时段仍会正常入队。手动重试处理原运行。[发布回归](../../.artifacts/task-center/list-detail/publication-regression.log) · [取消区间回归](../../.artifacts/task-center/list-detail/cancelled-range.log)
- `pnpm lint`、`pnpm typecheck`、`pnpm contracts:check`、`pnpm permissions:check`、管理端生产构建及 `git diff --check` 通过。
