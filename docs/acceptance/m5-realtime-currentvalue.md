# M5 CurrentValue / WebSocket 真实链路验收

入口：

```bash
task m5:realtime-acceptance
```

这是显式验收任务，不属于默认 `task check`。脚本要求真实环境已经准备好；缺少前置条件会立即失败并打印原因，不会输出伪造的 PASS。

## 真实环境

M5 复用 M3 的真实集成环境。建议先启动一次隔离环境并保留它：

```bash
cd ../edge-dev-infra
cp .env.example .env       # 首次执行时填写 PostgreSQL admin 凭据
task integration:config
cd ../edge-platform
M3_DEVICE_CLEANUP=0 task m3:device-acceptance
```

这一步启动真实 PostgreSQL、Mosquitto、Cloud、Edge Collector 和 Modbus simulator，并通过真实 Collector 产生 Edge/Device 事实。M5 只调用 Collector 的配置/状态 API 和 Cloud REST/WebSocket，不向 Mosquitto 发布正常 raw；正常 CurrentValue 必须来自 Collector 采集链路。

Cloud 重启需要一个显式、可审计的 hook。它应停止并重新启动同一个 Cloud 进程，等待 `/health`，成功退出 0。例如可以使用本地维护的 `scripts/m5-restart-cloud.mjs`；脚本不会执行 shell 字符串，也不会猜测如何管理外部进程。

## 一条命令

```bash
M5_CLOUD_URL=http://127.0.0.1:18199 \
M5_COLLECTOR_URL=http://127.0.0.1:18198 \
M5_DEVICE_ID=<M3 真实 Cloud deviceId> \
M5_POSTGRES_URL='postgres://edge_platform:edge-platform-db-dev@127.0.0.1:15432/edge_platform?sslmode=disable' \
M5_CLOUD_RESTART_SCRIPT=/absolute/path/to/m5-restart-cloud.mjs \
M5_COLLECTOR_SOURCE_EXTERNAL_ID=m3-source-device \
M5_POSTGRES_CONTAINER=m5-m3-acceptance-postgres-1 \
task m5:realtime-acceptance
```

## 环境变量

| 变量 | 默认/要求 | 作用 |
| --- | --- | --- |
| `M5_CLOUD_URL` | 必填 | 正在运行的 Cloud URL |
| `M5_COLLECTOR_URL` | 必填 | 正在运行的真实 Collector URL |
| `M5_DEVICE_ID` | 必填 | M3 真实 Device 的 Cloud UUID |
| `M5_POSTGRES_URL` | 必填 | 真实 Cloud PostgreSQL DSN；脚本用 `psql` 查询 schema |
| `M5_POSTGRES_CONTAINER` | 主机无 `psql` 时必填 | 精确的真实 PostgreSQL 容器名；脚本通过 `docker exec` 查询 schema |
| `M5_POSTGRES_USER/PASSWORD/DATABASE` | `edge_platform` / `edge-platform-db-dev` / `edge_platform` | 容器内 schema 查询凭据 |
| `M5_CLOUD_RESTART_SCRIPT` | 必填 | 可执行的 Cloud 重启 hook；Node 脚本由当前 Node 执行 |
| `M5_INFRA_ROOT` / `M5_COLLECTOR_ROOT` | `../edge-dev-infra` / `../edge-collector` | sibling 仓库路径检查 |
| `M5_MQTT_HOST` / `M5_MQTT_PORT` | `127.0.0.1` / `18884` | 真实 Mosquitto TCP 可达性检查 |
| `M5_COLLECTOR_SOURCE_EXTERNAL_ID` | `m3-source-device` | Collector 真实采集 Device |
| `M5_NUMBER_ADDRESS` / `M5_BOOLEAN_ADDRESS` | `0` / `1` | 已在 M3 simulator register block 中采集的地址 |
| `M5_SESSION_CLOSE_TIMEOUT_MS` | `40000` | session revoke 等待窗口；默认覆盖 Cloud 30s session check |
| `M5_USERNAME` / `M5_PASSWORD` | `admin` / `admin123` | Cloud 本地验收账号 |
| `M5_COLLECTOR_USERNAME` / `M5_COLLECTOR_PASSWORD` | `admin` / `admin123` | Collector 本地验收账号 |

## 验收断言

脚本通过真实链路验证：

1. PostgreSQL `current_value`、`data_point`、`source_mapping` 真实表存在；Collector runtime 对真实 simulator 设备为 `ONLINE`，有 register blocks。
2. 真实采集产生 NUMBER、BOOLEAN 首次 snapshot，随后产生新的 live update；浏览器一次 WebSocket 批量订阅两个点。
3. mapping reset、disable/enable 先通过 Cloud transaction 推送 `NO_DATA`，旧 raw 不回填，重新采集后才恢复 GOOD。
4. 浏览器 ticket 只通过 `POST /api/realtime/ticket` 换取；重用 ticket、等待真实 `expiresAt` 后使用 expired ticket 都被拒绝。
5. Cloud 重启 hook 执行后，浏览器重新取 ticket、订阅并收到 snapshot；logout/session revoke 最终以 1008 关闭连接。
6. WebSocket payload 只含 CurrentValue 语义字段，不含 SourceMapping、MQTT、Modbus、Edge 或 source-device 内部字段；浏览器不连接 MQTT。
7. 脚本使用一个独立 token 的第二浏览器作为连接隔离观察者；慢客户端队列饱和、`1013` 隔离、不阻塞其他连接和 CurrentValue commit 由 `server/internal/realtime` 的 race/transport tests 覆盖。

snapshot/live 最高 revision 合并和 lower revision 忽略有显式辅助断言；它们不伪造正常采集值，真实 Collector 链路仍是主成功证据。M5 不引入 Redis、Kafka、Gateway 或 M6 能力。

## 最终门禁

```bash
task check
task backend:integration
task backend:mqtt-integration
go -C server test -race ./...
npm --prefix web run test:realtime
git diff --check
```

若机器没有 Docker/Compose、`psql`、Playwright Chromium、sibling 仓库、真实 Mosquitto/Collector/Modbus simulator，M5 真实验收不能宣称通过；先补齐对应前置条件后再重跑。不要用手写 raw MQTT、SQLite 或合成 WebSocket 消息替代主链路。
