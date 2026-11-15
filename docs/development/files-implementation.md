# 文件与附件实施记录

状态：**In progress**。开始：2026-11-14。依据：[详细开发方案](../requirements/files-and-attachments-development-plan.md)。在当前 checkout/main 增量实施，保留既有未提交变更。

实现采用现有 Go/pgx/Gin、PostgreSQL、blobstore 私有 S3、React/TypeScript/TanStack。文件元数据组合不初始化对象存储；可信 worker 负责 IO 和事务，独立无凭据 checker 负责有限内容检查。普通文件与专业文件只共享客户端交互，不混用存储或授权。

实施账本（通过完整自验后才改为 Awaiting acceptance）：

| 范围 | 实现／证据 | 状态 |
| --- | --- | --- |
| FD-01–03 元数据、权限、宿主 | 增量迁移、files Interface、runtime | 进行中 |
| FD-04 传输和读取 | 严格 HTTP、分片、完整性、逐块授权 | 待实施 |
| FD-05–06 任务和通知 | 同事务事实、固定代次、当前授权 | 待实施 |
| FD-07–08 页面及专业适配 | 两资料页、三页签、四 Adapter、专业导出 | 待实施 |
| FD-09–10 契约和运行 | 生成类型、配置、worker/checker、维护和恢复 | 待实施 |
| FD-11–12 验收和材料 | DA-01–DA-44、回归、真实浏览器和恢复 | 待实施 |

软件自验和人工接受分开记录。所有 DA 条件的实际结果及证据在最终验收记录逐项列出；不将尚未执行的检查标为通过。人工流程依次为资料附件、多账号权限、专业流程、任务通知、清理恢复。
