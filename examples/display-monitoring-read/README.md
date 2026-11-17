# 独立监测读取验收工程

此工程只依赖公开的 `@infrapro/display-sdk`、`@infrapro/display-devtools` 和打包到应用内的 Three.js。运行时使用隔离 iframe / MessageChannel，不访问控制台内部 API，不从 CDN 加载代码。统计是服务端完整范围结果；浏览器不把当前页相加当作总数。

## 安装与构建

在 InfraPro checkout 执行 `pnpm display:pack`，会生成 SDK / 工具 1.1.0 tgz、Manifest 1 回归 ZIP、此工程的 Manifest 2 ZIP，并在仓库外临时目录安装、构建、校验它们。外部目录记录在 `.artifacts/display/monitoring-read-external.json`。

交付包支持直接在任意目录使用：

```sh
mkdir display-bootstrap
cd display-bootstrap
pnpm add /绝对路径/infrapro-display-devtools-1.1.0.tgz
pnpm exec infrapro-display init ../monitoring-display --template monitoring-read
cd ../monitoring-display
pnpm add /绝对路径/infrapro-display-sdk-1.1.0.tgz /绝对路径/infrapro-display-devtools-1.1.0.tgz
pnpm build
pnpm exec infrapro-display validate dist --manifest display.manifest.json
pnpm run pack
```

路径中的“绝对路径”需要替换为交付 tgz 的目录。构建后验证 ZIP 并启动模拟环境：

```sh
pnpm exec infrapro-display validate display.zip
pnpm exec infrapro-display dev dist --scenario normal
```

模拟场景：`normal`、`empty`、`pending`、`partial`、`disconnected`、`revoked`、`unavailable`、`overlap`、`scope-changed`。页面和外层运行壳均标识模拟数据。模拟结果不能用作真实业务授权、计数或容量证明。

## 范围配置

先上传 ZIP，在项目“大屏”工作区配置实例读取授权，再映射程序的命名视图。授权人须拥有 grant 操作及对应业务资格；配置人须拥有 configure 操作和当前业务读取资格。项目管理员身份本身不替代资源、设备、监测与空间授权。

| 视图名称 | 领域 | 主要字段 | 用途 |
| --- | --- | --- | --- |
| project_resources | resources | resource.metadata、resource.attributes、resource.position、statistics.counts | 树、地图、设施统计 |
| project_raw | channels | resource.metadata、source.binding、monitoring.values、monitoring.quality、statistics.counts | 仅所选原始通道 |
| count_channels（可选） | channels | resource.metadata、statistics.counts | 资源计数中的 12 个原始＋X01/X02，共 14 个通道 |
| focus_derived | channels | resource.metadata、monitoring.values、monitoring.quality、statistics.counts | 明确的派生通道 ID 或固定 derived metric_ref |
| project_devices | devices | device.metadata、device.outputs、device.installations、statistics.counts | 获准设备及来源投影 |
| project_spatial | spatial | spatial.metadata、spatial.geometry；需要属性时加 spatial.properties | 固定已发布图纸、GLB 模型或 GIS 资料 |

视图名是程序的逻辑名称；范围名称由配置人选择。程序没有内置真实对象 ID。`INSTANCE_UUID` / `CONFIG_UUID` 分别替换为当前实例和已保存配置的 UUID，不能填入查询或 selection 句柄。显式通道限制 500 个；完整项目使用命名项目范围。派生视图必须明确选择指定通道或固定指标版本，不能按名称自动扩大。

模板只强制 views.read / queries.read，其余能力按需要明确授权。每个面板会显示缺失能力或视图的错误，不填假零。展示模型还需要 spatial.read、spatial.catalog；关联和相机设置需要 spatial.links。原始详情、派生递归输入、source、历史分别按各自资格核验。

默认固定成员在保存时冻结。只有程序声明、实例授权、配置均允许 include_future 时未来成员才会进入。时间边界使用带时区 ISO 8601；from 包含、to 排除，数值和计数按字符串展示。

## 真实预览、上传与终端

```sh
pnpm exec infrapro-display login dist --console http://你的控制台地址 --runtime http://你的运行域 --instance INSTANCE_UUID --config CONFIG_UUID
pnpm exec infrapro-display upload display.zip --console http://你的控制台地址 --runtime http://你的运行域 --instance INSTANCE_UUID
```

按工具给出的配对入口在已登录浏览器确认；凭证留在工具／隔离壳，不写入应用。未完成上传可使用工具报告的上传编号加 `--resume UPLOAD_UUID` 恢复。不能将这些占位符当作实际 ID。

在管理端保存不可变配置，执行核验、真实预览、发布。核验显示当前匹配数量、字段与未来成员策略，以及可读空间资产的项目默认版本（最多展示 20 项样本和完整资产数）。授权或资料修订变化后需重新核验。创建终端后在运行域 `/terminal` 输入一次性配对码。撤权、撤回或回退后终端必须应用当前授权，旧查询和订阅不能继续读取。

## 面板验收

1. **项目统计**：三类 totals、分组分页、原始与派生分列、组下钻和统计订阅。订阅替换结果，绝不累计重复事件。设备设施分组显示 overlapping 含义。
2. **设施树与地图**：逐层展开，详情和空间位置分页；缺失坐标单列，合法 `(0,0)` 保留。地图以空白经纬底图绘制；可通过 SDK 查看本地合成示意瓦片，始终标注非测绘资料。
3. **设备与来源**：固定采用型号、输出与当前来源投影；“下一批通道来源”继续同一通道快照，覆盖全部分页；restricted 身份只按服务端允许字段展示，来源有效不称为设备在线。
4. **指定指标与时间**：原始／指定派生视图、range / all_history / follow_seconds，精确明细、趋势与质量／计算状态。超过 50 通道时须收窄趋势视图，不能拿前 50 个假装全项目。
5. **图纸与模型**：发现资产、项目默认及固定已发布版本；分块读取并核对完整 SHA256；图纸页、区域与测点叠加、GLB 隐藏元素／相机／测点、GIS 要素、空间关联与设置。挂载和关联会完整取页，关闭查看器会释放自身请求和图形资源。示例查看器单文件预算 256 MiB；SDK 沿用每块 4 MiB。渲染器只接受自包含展示文件，拒绝外部模型 URL。
6. **状态与诊断**：当前能力、视图描述、读取状态与查询 ID。pending 轮询原 query，partial 显示原因。没有无限重试；修正配置后手动刷新面板。

切换面板会中止请求并释放消费者；最后消费者释放服务端查询。停止／关闭会取消订阅、释放 Blob URL 和 WebGL 资源。后台查询最长等待 120 秒；查询、父选择及运行主体到期后应显式重建。
