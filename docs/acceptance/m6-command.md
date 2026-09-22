# M6 Command 真实闭环验收

`task m6:command-acceptance` 是 M6-07 / GitHub #40 的显式验收入口，不属于默认 `task check`。脚本只做真实链路验收，不增加生产旁路，也不修改 `../edge-collector`。

## 覆盖边界

脚本临时启动一个隔离 Compose project，并使用：

- PostgreSQL 17：分别建立 Cloud 与 Collector 数据库；
- Eclipse Mosquitto 2：真实 MQTT QoS1 broker；
- sibling `../edge-collector` 的真实 Go API；
- sibling `modbus-simulator` 的真实 PTY/Modbus RTU fixture；
- 当前仓库的真实 Cloud API；
- 当前仓库 Web dev server 与 Playwright/Chromium。

正常控制的唯一入口是 Cloud `POST /api/command`。脚本中的 MQTT 直发只用于明确标记的结果顺序和负向 probe，不作为正常控制成功证据：

- `FINAL-before-ACCEPTED`、重复/冲突 FINAL；
- unknown commandId；
- route mismatch；
- name mismatch；
- Cloud delivery 到期后的显式 Edge `EXPIRED`。

每个正常控制都同时断言 Cloud Command 状态、真实 Collector `device-command-result/v1`、Cloud 时间字段和 simulator 的真实 FC16 日志。脚本还检查 PostgreSQL 的 `command`、`command_delivery`、`sys_oper_log` 计数及冻结 payload 的 SHA-256；不会把密码、JWT、MQTT secret 或完整敏感 payload 打到 stdout。

场景日志使用结构化 JSON，包含：

1. Cloud POST 202、PENDING、真实 Modbus FC16、SUCCEEDED、Edge/Cloud 时间分离；
2. 相同 commandId retry、不同 args 409、单 Command/delivery/audit 和单次 FC16；
3. 无角色用户服务端 403；
4. broker 暂停/恢复；
5. Cloud 停止后使用同一 commandId 和冻结 payload 恢复；
6. FINAL-before-ACCEPTED、重复与 first-terminal-wins 冲突；
7. unknown/route/name negative probe 及 bounded metric；
8. 无结果 TTL 到期保持 PENDING，随后显式 Edge EXPIRED；
9. 已 ACCEPTED 后 broker/Cloud 重启，真实动作只执行一次；
10. Playwright 登录、Device 执行入口、Command detail 轮询从 PENDING 到终态。

## 运行方式

普通本地运行：

```text
task m6:command-acceptance
```

脚本也可直接运行：

```text
node scripts/m6-command-acceptance.mjs
```

依赖缺失时默认输出 `SKIP M6 Command acceptance: missing ...` 并退出 0。CI 或正式验收必须使用：

```text
M6_REQUIRED=1 task m6:command-acceptance
```

此时缺 Docker daemon、Go、uv、sibling fixture、Playwright/Chromium 或启动/断言失败都会退出 1。成功时最后一行输出 `status: "passed"` 的结构化摘要；失败时输出 Cloud、Collector、simulator、Web、MQTT subscriber 和 Compose diagnostics，并清理临时进程及 Compose volumes。

## 可覆盖环境变量

默认端口使用 `18886`（MQTT）、`15434`（PostgreSQL）、`18206`（Cloud）、`18207`（Collector）、`14206`（Web）。可通过下列变量覆盖：

- `M6_BROKER_PORT`、`M6_POSTGRES_PORT`、`M6_CLOUD_PORT`、`M6_COLLECTOR_PORT`、`M6_WEB_PORT`；
- `M6_COMPOSE_PROJECT`、`M6_COLLECTOR_ROOT`；
- `M6_EDGE_ID`、`M6_SOURCE_DEVICE_ID`、`M6_TOPIC_PREFIX`；
- `M6_POSTGRES_USER`、`M6_POSTGRES_PASSWORD`、`M6_CLOUD_DATABASE`、`M6_COLLECTOR_DATABASE`；
- `M6_PLAYWRIGHT_EXECUTABLE_PATH`；
- `M6_CLEANUP=0` 可保留现场供人工诊断，但不会自动清理 Compose project；
- `M6_REQUIRED=1` 将缺依赖从 SKIP 提升为失败。

默认凭据和端口只用于临时本地验收。不要把真实部署 secret 注入日志或提交到仓库。

## 运行边界

该入口不加入 `task check`，不替代 M6 单元/数据库/race 测试，也不声称完成生产压力、浏览器全量回归或非 ADR-0017 的现场协议验收。Cloud 业务仍只通过 Command/MQTT v1 边界与 Collector 交互；脚本不添加 History、HMI、CommandDefinition、cancel、replay、schedule、workflow 或通用 outbox。
