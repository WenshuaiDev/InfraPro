# 工程结构与设备分类示例

此独立程序使用 `@infrapro/display-sdk` 的命名读取视图，展示完整混合树、孔位适用性、设备分类、断面观测统计及固定查询下钻。真实运行通过 InfraPro 实例授权、发布和终端配对；开发模拟页面会明确显示模拟状态。

运行 `pnpm display:pack` 会从工作区打出 SDK／devtools 包，并在外部临时目录完成三种模板的离线安装、类型检查、构建、验证和 ZIP 打包。当前示例交付包位于 `.artifacts/display/engineering-structure-example.zip`；依赖包位于 `.artifacts/display/`。

在项目大屏工作区上传 ZIP，创建 `scope_format: 2` 授权及 `config_format: 2` 配置。以下视图均明确指定 `structure_model: 2`：

| 视图 | 领域 | 建议字段 |
| --- | --- | --- |
| `project_resources` | resources | resource.metadata、resource.attributes、resource.position、statistics.counts |
| `project_devices` | devices | device.metadata、device.classification、device.outputs、device.installations、statistics.counts、statistics.classification |
| `project_raw` | channels | resource.metadata、source.binding、monitoring.values、monitoring.quality、statistics.counts |
| `project_spatial` | spatial（可选） | spatial.metadata、spatial.geometry、spatial.properties |

按清单显式授予所用能力，再映射这些视图、预览范围并发布。设备名单需要 `devices.read` 及设备字段；只授权分类汇总时不能通过下钻取得名单。来源设备统计还需相应通道和输出的独立权限。实例父级、设施父级或空间权限均不会自动授予下级工程对象。

结构统计支持最近归属与子树包含，后者可能重叠。无孔类型、允许但零孔、旧类型未声明分别显示 `not_applicable`、`applicable` 和 `undeclared`。未归属断面的观测保留在 `unassigned`，上下文祖先不计实体。

下钻从原查询取得 opaque selection，继续读取该选择以保持原分类、成员和结构路径。交付时仍验证当前权限；撤权、过期、释放或环境恢复可使原结果失效。资源详情传入 `{id, kind}`，避免跨种类 UUID 重合歧义。

真实端到端自验运行 `node scripts/sensor-structure-browser.mjs`，使用独立测试数据库与本机测试存储，生成第 9.5 节合成样本和浏览器证据。该脚本不把模拟连接当作权限证明。
