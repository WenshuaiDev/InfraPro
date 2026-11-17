# @infrapro/display-devtools 1.1

Node.js 22+ 的独立大屏工具，无运行时 npm 依赖。包含可信本机壳、同版 SDK 测试夹具与最小 TypeScript 工程。安装交付的 tgz；CLI 为 `infrapro-display`。

```sh
npm install /path/to/infrapro-display-devtools-1.1.0.tgz
npx infrapro-display init my-display
cd my-display
npm install /path/to/infrapro-display-sdk-1.1.0.tgz
npm install --save-dev typescript@5.9.3
npm run build
npx infrapro-display validate dist --manifest display.manifest.json
npx infrapro-display pack dist --manifest display.manifest.json --out display.zip
npx infrapro-display validate display.zip
npx infrapro-display dev dist --scenario normal
```

内网可使用预先缓存的 TypeScript 包或内部 registry；SDK 和工具自身不联网安装依赖。模板采用 tsc 与相对 ESM/import map，方便检查；可自行换成框架或 bundler，所有运行依赖都需打入包中。

`dev` 默认监听本机 5190（壳）和 5191（不可信静态应用），`--port` 修改前者并使用后续端口。模拟数据始终有标识，场景支持 normal/empty/pending/partial/disconnected/revoked/unavailable/overlap/scope-changed。修改后重新 build 并刷新页面，不运行上传包的安装脚本。

`init my-structure-display --template engineering-structure` 创建设备分类与混合工程结构示例，包括断面／孔位统计、分类去重、70 条观测示例口径和固定查询下钻。正式运行需显式配置 `structure_model: 2` 的 resources／devices／channels 视图及相应能力；模拟模式不证明项目授权。完整配置和操作见[示例说明](../../../examples/display-engineering-structure/README.md)。

## 真实预览

先在项目创建实例、上传兼容包并保存草稿配置及读取授权：

```sh
npx infrapro-display login dist --console http://console.infra.lan/console --runtime http://display.infra.lan --instance INSTANCE_UUID --config CONFIG_UUID
```

在已经登录控制台的浏览器打开工具打印的配对链接，并点击继续。一次性码绑定本机端口、随机校验及指定实例/配置；工具内存持有短期运行凭据，应用只通过隔离 Bridge 使用当前人员资格。只读预览不能扩大人员或项目范围。结束时 Ctrl-C；再次使用重新配对，不向磁盘保存密码、cookie 或访问令牌。

生产 console／runtime 支持 HTTPS，二者须使用同一协议。内部 CA 需安装到浏览器信任库，并在启动 Node 工具前设置 `NODE_EXTRA_CA_CERTS=/受限目录/ca-chain.pem`；Node 按证书链、主机名和有效期正常校验，不能使用 `NODE_TLS_REJECT_UNAUTHORIZED=0`。例如：

```sh
NODE_EXTRA_CA_CERTS=/etc/infrapro/ca-chain.pem npx infrapro-display login dist --console https://10.23.52.152:18443/console --runtime https://10.23.52.152:18444 --instance INSTANCE_UUID --config CONFIG_UUID
```

本机回调和本地预览仍只监听 loopback HTTP，不能将这个例外扩展为任意 HTTP 远端。输入 console 不得带凭据、查询和片段；runtime 必须是精确 origin。

## 上传与继续上传

```sh
npx infrapro-display upload display.zip --console http://console.infra.lan/console --runtime http://display.infra.lan --instance INSTANCE_UUID
# 网络中断后，用错误输出的 upload_id 继续相同 ZIP：
npx infrapro-display upload display.zip --console http://console.infra.lan/console --runtime http://display.infra.lan --instance INSTANCE_UUID --resume UPLOAD_UUID
```

上传配对只授予此实例的包上传操作，即使实例没有发布/草稿也能使用。工具按 4 MiB 分片，校验本地整包及远端已收分片摘要；重复请求保持幂等，暂时故障最多重试 3 次。恢复时必须使用完全相同的 ZIP。工具完成后仍需独立 worker 校验；在管理页查看 ready/failed、问题路径、重试或主动取消。工具不能配置、授予业务读取、发布或回退。

## 包规范

1.1 同时验证 Manifest 1 和 2。Manifest 2 增加 `read_views`，各条声明 domain、purpose、required、field_groups 和 allow_dynamic；未知版本、未知字段、重复键、错误领域均拒绝。旧包不会自动获得项目视图。`init my-display --template monitoring-read` 创建独立的六面板验收工程，包含完整分页、统计下钻、来源目录和图纸／Three.js 模型查看器；构建依赖在模板内声明，运行资源全部打包到 ZIP。模板文档说明授权配置、计数口径和释放流程。

包根 `manifest.json` 由 pack 从 `display.manifest.json` 和构建目录生成。ZIP 不包含 node_modules、源码声明或服务端程序。入口为相对 HTML；资源、模块、字体、Worker/WASM 使用包内相对路径。路由可用 hash，history 仅支持 manifest 明列的 fallback 路径。没有全局任意路径回退。

manifest_version=1、sdk_protocol.major=1；声明 app_key/version、required_capabilities/optional_capabilities、config_schema、binding_schema。config_schema 支持有限 object/array/string/number/integer/boolean、enum、默认值、字符串长度、数值上下限、required；最多 6 层，数组必须指定 items/maxItems，不允许远程引用、脚本或 additionalProperties=true。bindings 为 resources/channels/spatial/artifacts，可声明 required、数量、通道语义（指标版本、单位维度、方向、基准和值类型）。配置里不要放秘密。

ZIP 上传大小使用服务器 `uploads.display_max_package_bytes`（默认 128 MiB）；离线 validate/pack 不固定此部署限额。解压 256 MiB、2048 个文件、单文件 32 MiB、压缩比 200、路径 12 层/240 字符。服务端是最终权威；大小写重名、穿越、链接、加密/特殊文件、未列资源、哈希错误和不兼容能力会拒绝。相同应用版本的新哈希是不同不可变构建，不覆盖旧包。

正式运行的 iframe 为 opaque sandbox，只允许脚本；没有持久存储、任意弹窗、表单、顶层导航或公共 CDN。Worker 通过 Blob 创建，WASM 由包内 fetch/编译使用。业务文件使用 SDK 分块读取。技术样例只证明接入，不是项目大屏模板。

许可证 MIT。API 类型见 SDK 包；完整管理、部署与恢复指南随平台交付。
