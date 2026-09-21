# M3 Device Domain 真实链路验收

M3 的显式入口是：

```bash
task m3:device-acceptance
```

它不属于默认 `task check`。脚本会启动唯一的临时 integration Compose project、真实 Cloud Server、两份真实 Edge Collector API 和真实 Modbus simulator，并在正常或失败结束时清理自己创建的进程、容器、数据库和临时配置。为排障可设置 `M3_DEVICE_CLEANUP=0` 保留环境，但此时必须手动执行对应 Compose project 的 `down --remove-orphans --volumes`。

## 前置条件

需要同一台机器提供：

- Docker daemon、Docker Compose plugin、Node.js 20+、Go 1.26+、Task 和 `uv`；
- sibling `../edge-dev-infra`，并已配置 `.env` 的 `POSTGRES_ADMIN_USER` / `POSTGRES_ADMIN_PASSWORD`；
- sibling `../edge-collector`，包括 `edge-collector-api/` 和 `modbus-simulator/`；
- 空闲端口：PostgreSQL `15432`、Mosquitto `18884`、Cloud `18199`、Collector A `18198`、Collector B `18197`。

基础设施目录首次准备：

```bash
cd ../edge-dev-infra
cp .env.example .env
# 填写本机 POSTGRES_ADMIN_PASSWORD；POSTGRES_ADMIN_USER 按 .env.example 配置
task integration:config
```

从 `edge-platform` 根目录执行：

```bash
task m3:device-acceptance
```

常用覆盖变量：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `M3_DEVICE_INFRA_ROOT` / `M3_DEVICE_COLLECTOR_ROOT` | `../edge-dev-infra` / `../edge-collector` | sibling 路径 |
| `M3_DEVICE_POSTGRES_PORT` / `M3_DEVICE_MQTT_PORT` | `15432` / `18884` | integration Compose 端口 |
| `M3_DEVICE_CLOUD_PORT` / `M3_DEVICE_COLLECTOR_A_PORT` / `M3_DEVICE_COLLECTOR_B_PORT` | `18199` / `18198` / `18197` | 三个真实 API 端口 |
| `M3_DEVICE_CLEANUP` | `1` | 设为 `0` 保留环境排障 |
| `M3_DEVICE_EDGE_A` / `M3_DEVICE_EDGE_B` | `m3-acceptance-edge-a` / `m3-acceptance-edge-b` | 两个真实 Edge identity |
| `M3_DEVICE_SOURCE_ID` | `m3-source-device` | 两个 Edge 共用的 source Device ID |
| `M3_DEVICE_COMPOSE_PROJECT` | 当前进程生成 | 唯一 Compose project 名称 |

## 验收实际覆盖

脚本通过 Collector 正式登录、MQTT 配置 API、采集通道/Device API 和脚本绑定 API 建立真实配置：

1. Collector A 先完成 M2 Edge 注册；先通过真实 Collector 配置一个尚未启用寄存器读取的 Device，观察真实 `DeviceStatus INITIAL`，再补充正式 Modbus 寄存器块。
2. 启动真实 Modbus simulator 后，真实采集产生 `ONLINE` 和 `raw-register-snapshot/v1`；绑定真实采集脚本后，真实 Collector 产生 `device-event/v1`。
3. 重启 Cloud，确认 retained DeviceStatus 恢复既有 current projection，Cloud `deviceId` / `registeredAt` 不变且没有重复行。
4. 强制终止 Collector A，确认真实 LWT 让 Edge 变为 `OFFLINE`，但 Device 保留最后 Collector 断言；同一 Edge 重连后 identity 保持稳定。
5. 保持真实 Modbus simulator 和 Unit 2 通道在线，通过真实 Collector Channel API 将 Unit 1 切换到不可用串口，验证真实 Collector/Cloud 的 `DEGRADED`、threshold `OFFLINE` 和恢复 `ONLINE`；Cloud 只投影 Collector 状态，不自行计算 threshold。
6. 通过真实采集流程观察 raw 和 DeviceEvent；再用仅针对未知 raw/event 的负向 broker probe 证明未知身份不创建 Device、已知 Device projection 不被这些消息改变。正常 Edge/Device lifecycle 从未由脚本手写 MQTT payload 代替。
7. 启动 Collector B，使用不同 `edgeId`、相同 `sourceDeviceId`，确认产生第二个独立 Cloud `deviceId`。
8. 查询认证 Device REST page/detail 与 PostgreSQL 行，并检查 Device 表字段、PK/FK、`UNIQUE(edge_id, source_device_id)`、查询索引及无 DataPoint/CurrentValue/raw/event/history/staging 表。

脚本不假设 retained EdgeStatus 与 DeviceStatus 的跨 topic 固定到达顺序；DeviceStatus-before-EdgeStatus 的确定性 replay 与合并上限由 #22 单元/集成测试覆盖。

## 失败排查

失败输出会保留 Cloud、两个 Collector、simulator 的最近日志，且会打印 Compose project 名称。常见检查：

```bash
docker info
docker compose --project-name m3-device-acceptance-<pid> --file compose.yaml --file compose.integration.yaml ps
```

如果设置了 `M3_DEVICE_CLEANUP=0`，完成排障后在 `../edge-dev-infra` 执行：

```bash
docker compose --project-name m3-device-acceptance-<pid> --file compose.yaml --file compose.integration.yaml down --remove-orphans --volumes
```

不要对长期开发 Compose project 或共享数据库执行上述清理命令。
