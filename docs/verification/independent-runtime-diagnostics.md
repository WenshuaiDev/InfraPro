# 独立运行诊断实施与自验

状态：**Awaiting acceptance**。人工接受：尚未进行。
本次 run_id：`diag-3dfcba7763695a52`。软件自验与用户人工接受分开记录。

完整依据：[方案](../requirements/independent-runtime-diagnostics-proposal.md)。[启动与人工步骤](../operations/independent-runtime-diagnostics.md)；[结构化台账](../../.artifacts/diagnostics/diag-3dfcba7763695a52/acceptance.json)。

环境：darwin/arm64；go version go1.27.0 darwin/arm64；Node v26.10.0；pnpm 12.18.0。容器具体镜像和资源界限见同目录 container-versions.json。

## DG-01～DG-60

| 编号 | 门槛 | 结果 | 证据 |
| --- | --- | --- | --- |
| DG-01 | L1 | passed | [native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json) |
| DG-02 | L1+L2 | passed | [faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-03 | L1 | passed | [faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json)、[native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-04 | L1 | passed | [deployment.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/deployment.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json)、[release.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/release.json) |
| DG-05 | L1 | passed | [faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-06 | L1 | passed | [native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-07 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[contracts.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/contracts.json) |
| DG-08 | L1+L3 | passed | [native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-09 | L1 | passed | [contracts.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/contracts.json) |
| DG-10 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-11 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json) |
| DG-12 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json) |
| DG-13 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json) |
| DG-14 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json) |
| DG-15 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json) |
| DG-16 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json) |
| DG-17 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[checker-sandbox.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/checker-sandbox.json) |
| DG-18 | L1+L2 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[business-regression.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/business-regression.json) |
| DG-19 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-20 | L1 | passed | [owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-21 | L2 | passed | [docker-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/docker-contract.json) |
| DG-22 | L2 | passed | [docker-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/docker-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[deployment.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/deployment.json) |
| DG-23 | L4 | passed | [linux-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/linux-contract.json) |
| DG-24 | L2 | passed | [fixed-protocols.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/fixed-protocols.json) |
| DG-25 | L1 | passed | [native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[linux-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/linux-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-26 | L2 | passed | [docker-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/docker-contract.json)、[deployment.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/deployment.json)、[linux-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/linux-contract.json) |
| DG-27 | L1 | passed | [faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-28 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-29 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-30 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-31 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-32 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-33 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-34 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json) |
| DG-35 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-36 | L3 | passed | [network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json) |
| DG-37 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-38 | L1+L3 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[docker-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/docker-contract.json) |
| DG-39 | L1 | passed | [native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-40 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-41 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-42 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-43 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-44 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-45 | L1+L3 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json) |
| DG-46 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-47 | L1+L3 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json) |
| DG-48 | L1+L2 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-49 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-50 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-51 | L1+L3 | passed | [faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-52 | L1 | passed | [core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json)、[native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json) |
| DG-53 | L1 | passed | [browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-54 | L1 | passed | [browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json)、[native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[core-tests.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/core-tests.json) |
| DG-55 | L1 | passed | [browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |
| DG-56 | L1+L2 | passed | [deployment.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/deployment.json)、[native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[release.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/release.json) |
| DG-57 | L1 | passed | [contracts.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/contracts.json)、[release.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/release.json) |
| DG-58 | L1+L2 | passed | [business-regression.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/business-regression.json)、[owning-processes.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/owning-processes.json)、[checker-sandbox.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/checker-sandbox.json) |
| DG-59 | L1～L4 | passed | [faults.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/faults.json)、[native-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/native-contract.json)、[network-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/network-contract.json)、[linux-contract.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/linux-contract.json) |
| DG-60 | L1～L4 | passed | [contracts.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/contracts.json)、[release.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/release.json)、[browser.json](../../.artifacts/diagnostics/diag-3dfcba7763695a52/browser.json) |

每行 JSON 记录实际命令、证据类型、run_id、环境、恢复结果、限制及原方案可观察条件。通过不以代码存在或 skipped 代替。报告快照、浏览器截图和实际网络／容器结果分别保存；密码、key、cookie、业务配置只留在 0600 私有实验目录，未复制进证据目录。

## 证明范围

- L5 real independent servers and networks unverified
- Actual Linux systemd units unverified; private D-Bus contract only
- Native PG/Redis/CH/S3 installation and other vendor/distribution combinations unverified
- Load/stress/long-duration production operation unverified

L1 原生 lab 不配置 Docker Adapter 或业务存储；L2 使用真实服务和 owning process；L3 两个原生及一个容器观察器通过私有测试 CA 验证 TLS；L4 是实际 Linux 容器资源采集和私有 D-Bus 契约，不能写成真实 systemd unit 已通过。

S3 固定 HEAD 的 403 按拒绝访问记录；CH 的常量查询只证明连接及固定查询，不能证明业务表授权。资源阈值、时钟与重放故障均明确标记注入方式。

## 人工确认

按运维说明打开 demo，核对页面、故障恢复、实际 lane、历史与资源作用域。只有用户明确确认后才把状态从 Awaiting acceptance 改为 Accepted。
