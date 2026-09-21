# M2 Edge 管理真实链路验收

本文定义 GitHub #19 的运行入口和验收契约。它只覆盖 PostgreSQL、Mosquitto、Edge Platform Cloud Server 和真实 Edge Collector 的联调；不引入微服务、不创建伪造的 Cloud 数据，也不把 Edge Collector 的 Modbus/寄存器内部概念扩散到 Cloud Edge 管理领域。

## 当前状态

根目录入口是：

```bash
task m2:edge-acceptance
```

它调用 `node scripts/m2-edge-acceptance.mjs`，脚本会创建唯一的临时 Compose project，并在结束时清理自己启动的 Cloud/Collector 进程和临时容器。该入口不属于默认 `task check`，必须显式运行。

## 前置条件

需要在同一台机器上准备：

- Docker daemon、Docker Compose plugin，以及可执行的 `task`；
- Node.js 20 或更高版本；脚本使用 ESM 和内置 `fetch`；
- Go 1.26 或更高版本，分别用于启动两个 Go API 和执行 migration；
- sibling 仓库 `edge-collector`，默认路径为 `../edge-collector`，其中后端目录是 `edge-collector-api/`；
- sibling 仓库 `edge-dev-infra`，默认路径为 `../edge-dev-infra`；
- 可用的临时端口：PostgreSQL `15432`、MQTT TCP `18884`，以及 Cloud/Collector HTTP 端口（下文示例使用 `18099/18098`）。

先准备隔离基础设施目录的 `.env`。脚本会使用 integration Compose 和 tmpfs，不复用长期开发环境的数据；不需要手动执行 `integration:up`：

```bash
cd /home/wangjian/project/golang/edge-dev-infra
cp .env.example .env                 # 仅首次执行；填写本机的 POSTGRES_ADMIN_PASSWORD
task integration:config
```

`edge-dev-infra` 的集成 Compose 会提供：

| 资源 | 默认地址 | 凭据 |
| --- | --- | --- |
| Cloud PostgreSQL | `127.0.0.1:15432/edge_platform` | `edge_platform / edge-platform-db-dev` |
| Collector PostgreSQL | `127.0.0.1:15432/edge_collector` | `edge_collector / edge-collector-db-dev` |
| Mosquitto TCP | `mqtt://127.0.0.1:18884` | `edge_platform / edge-platform-mqtt-dev`；Collector 使用 `edge_collector / edge-collector-mqtt-dev` |

这些是本地集成环境的固定测试凭据，不得用于生产。Cloud REST 的默认种子账号是 `admin / admin123`，也只用于本地验收。

## 一条可复制命令

下面的命令启动隔离基础设施、Cloud、真实 Collector 并在成功或失败后清理临时 Compose project：

```bash
cd /home/wangjian/project/golang/edge-platform
M2_EDGE_CLOUD_PORT=18099 M2_EDGE_COLLECTOR_PORT=18098 M2_EDGE_EDGE_ID=m2-edge-01 task m2:edge-acceptance
```

也可以直接在 `edge-platform` 根目录执行默认端口验收：

```bash
task m2:edge-acceptance
```

不要把长期开发 Compose（默认 `5432/1883`）当作本验收的数据库或 Broker；脚本默认使用独立的 integration project。

## 脚本接口与环境变量覆盖

Taskfile 不隐藏参数，所有 `M2_EDGE_*` 环境变量原样传给 Node 脚本。脚本的最终接口应支持以下覆盖；未设置时使用与 `edge-dev-infra` 集成 Compose 和 sibling 仓库相符的默认值。

| 变量 | 示例默认值 | 作用 |
| --- | --- | --- |
| `M2_EDGE_INFRA_ROOT` / `M2_EDGE_COLLECTOR_ROOT` | `../edge-dev-infra` / `../edge-collector` | 两个 sibling 仓库路径；也兼容脚本的 `_DIR` 别名 |
| `M2_EDGE_POSTGRES_PORT` / `M2_EDGE_MQTT_PORT` | `15432` / `18884` | integration Compose 暴露端口 |
| `M2_EDGE_CLOUD_PORT` / `M2_EDGE_COLLECTOR_PORT` | `18099` / `18098` | Cloud / Collector HTTP 端口 |
| `M2_EDGE_CLOUD_DB_USER/PASSWORD/NAME` | `edge_platform / edge-platform-db-dev / edge_platform` | Cloud PostgreSQL 凭据和数据库 |
| `M2_EDGE_COLLECTOR_DB_USER/PASSWORD/NAME` | `edge_collector / edge-collector-db-dev / edge_collector` | Collector PostgreSQL 凭据和数据库 |
| `M2_EDGE_CLOUD_MQTT_USER/PASSWORD` | `edge_platform / edge-platform-mqtt-dev` | Cloud MQTT 凭据 |
| `M2_EDGE_COLLECTOR_MQTT_USER/PASSWORD` | `edge_collector / edge-collector-mqtt-dev` | Collector MQTT 凭据 |
| `M2_EDGE_MASTER_SECRET` | 本次运行的临时值 | Collector 加密 MQTT 配置所需的部署级主密钥；也兼容 `M2_EDGE_COLLECTOR_MQTT_MASTER_SECRET` |
| `M2_EDGE_MQTT_TOPIC_PREFIX` | `edge` | MQTT topic prefix |
| `M2_EDGE_EDGE_ID` | `m2-edge-01` | 本次验收使用的固定 Edge identity；也兼容 `M2_EDGE_ID` |
| `M2_EDGE_COMPOSE_PROJECT` | 当前进程生成 | 临时 Compose project 名称 |
| `M2_EDGE_CLEANUP` | `1` | 设为 `0` 保留环境用于排障；正常验收保持默认清理 |

若改用非默认端口，只需同时覆盖对应的端口变量；脚本会为两个 API 和 MQTT runtime 生成一致的地址。Collector 的 `APP_MQTT__MASTER_SECRET` 是部署级密钥，脚本启动 Collector 时必须为它提供临时值，并在通过 Collector API 写入 MQTT 配置和后续重启时保持一致；它不是 PostgreSQL 或 Broker 密码。

## 测试实际启动的进程

验收不是仅调用几个 HTTP mock。完整运行应包含：

1. `edge-dev-infra` 集成 Compose 中真实的 PostgreSQL 17 容器和 Eclipse Mosquitto 2.0.22 容器；
2. Cloud Server 的真实 Go 进程（`server/cmd/migrate` 后启动 `server/cmd/api`），启用进程内 MQTT runtime；
3. Edge Collector 的真实 Go API 进程（`edge-collector/edge-collector-api/cmd/migrate` 后启动 `cmd/api`），通过其 MQTT 配置 API 指向同一个 Mosquitto；
4. 本 M2 Edge 生命周期不依赖手写正常状态 payload；如果后续要扩展到 Collector 的设备状态、raw 或 event 上报，可在同一 harness 中接入 sibling 仓库已有的 Modbus simulator。它只用于产生真实 Collector 上报，不替代 Collector MQTT client。

脚本可以启动和终止 Cloud/Collector 进程，也可以检查外部 Compose 的健康状态；基础设施的最终清理由外层命令的 `trap` 或人工 `task integration:down` 完成。不得以直接调用 Cloud `EdgeService`、注入内存消息或手写正常生命周期 payload 替代真实 MQTT 链路。

## 验收断言

### Edge 生命周期

脚本必须通过真实 Collector 完成以下顺序，并从 Cloud 的认证 REST API 和 PostgreSQL 事实交叉验证：

1. Collector MQTT 连接后发布 retained `edge-status/v1` online；Cloud 通过真实订阅收到它，`GET /api/edge/page` 和 `GET /api/edge/:edgeId` 都能查询到同一个 Edge，状态为 `ONLINE`。
2. 记录 `registeredAt`、`lastSeenAt`，并记录 Cloud 观察到断开和收到 LWT 的时间。
3. 强制终止真实 Collector 进程，让 Mosquitto 产生其 CONNECT 时注册的 LWT offline；不得通过 `mosquitto_pub` 或脚本直接发布正常 offline 状态。Cloud 最终状态必须是 `OFFLINE`。
4. `lastSeenAt` 必须接近 Cloud 收到 LWT 的时间，而不是 LWT envelope 自带的 timestamp/Will 生成时间；断言应使用明确容差并把两者都打印到失败信息中。
5. 使用同一 `edgeId` 重启真实 Collector 并重新发布 retained online。Cloud 中只能有一条记录，状态恢复为 `ONLINE`，且 `registeredAt` 与第一次登记完全不变；`lastSeenAt` 应是重连后新收到的时间。

### 消息隔离

- 未知 Edge 的 `device-status/v1`、`raw-register-snapshot/v1`、`device-event/v1` 负向探针不得创建 Edge 记录。
- 对已知 Edge，device status、raw、event 负向探针不得改变 Edge 的 `status` 或 `lastSeenAt`；Cloud 不保存这些 payload，也不因它们创建 Device、DataPoint、raw history 或 Event 记录。脚本明确把这些探针标记为负向安全测试，正常 online/offline/reconnect 仍全部由真实 Collector 产生。
- Edge 状态消息的 `online` 必须是存在且为 JSON boolean；缺失、`null`、字符串或数字都应被拒绝且不改变数据库。retained 标志和 LWT 来源不能改变正常 Edge 状态投影规则。

浏览器页面或手写 MQTT client 只能用于上述“未知消息不产生副作用”的负向隔离探针。它们不能产生、补发或修正 online/offline、LWT 或重连生命周期；这些事实必须来自真实 Collector。

### Cloud 查询和时间

- 使用认证 REST 请求验证 `/api/edge/page` 的默认分页、状态/Edge ID 精确过滤、稳定排序和 `/api/edge/:edgeId` 详情；未知详情返回 404。
- 页面查询不作为生命周期事实源；浏览器测试最多验证真实 query contract 和负向隔离探针，不替代后端链路验收。
- API 时间字段必须是带 `Z` 或明确 UTC offset 的 RFC3339 instant；比较时间时先解析为 instant，不按字符串前缀或本地时间比较。

### PostgreSQL schema

连接 Cloud 的 `edge_platform` 数据库查询 `information_schema.tables` 和 `information_schema.columns`，断言 M2 只增加 Edge 当前投影所需的表/列（`edge_id`、`status`、`registered_at`、`last_seen_at` 及其必要索引），不存在 Device、DataPoint、raw history、DeviceEvent 或任意 payload/history 表和列。该检查必须针对真实 PostgreSQL，而不是 SQLite 或 migration 文件文本。

## 清理策略

- Node 脚本必须在成功、断言失败、收到 SIGINT/SIGTERM 时停止自己启动的 Cloud/Collector 进程，并保留最后一段 stdout/stderr 供排查。
- 脚本会停止自己启动的进程并执行唯一 Compose project 的 `down --volumes`；临时 Compose 使用 tmpfs，正常结束后不保留本次数据库和 Broker 数据。
- 如果运行过程中断电或清理未执行，先查看脚本输出中的 Compose project 名称，再在 `edge-dev-infra` 目录执行对应的 down：

  ```bash
  docker compose --project-name m2-edge-acceptance-<pid> --file compose.yaml --file compose.integration.yaml down --remove-orphans --volumes
  ```

- 不要对长期开发环境使用清理集成环境的 Compose project 名称，也不要删除 named volume；长期环境应使用 `task down`，需要清空数据时另行确认。

## 失败排查

1. **入口立即失败**：确认从 `edge-platform` 根目录执行，并检查 `scripts/m2-edge-acceptance.mjs`、两个 sibling 路径和 Docker Compose plugin。
2. **Docker/Compose 失败**：检查 `docker info`、`docker compose version`、`task integration:ps`；确认 `15432` 和 `18884` 没有被其他 Compose 占用。
3. **Cloud/Collector 启动失败**：先看两边的 migration 输出和 API stderr；确认 Go 版本、工作树路径、HTTP 端口，以及两个数据库 URL、用户名、密码分别对应 `edge_platform`/`edge_collector`。
4. **Cloud REST 401/连接失败**：检查 Cloud `/health`、登录账号和 `M2_EDGE_PLATFORM_HTTP`；迁移必须包含 schema 和 seed，不能只执行 schema。
5. **MQTT 无消息**：确认两进程使用同一个 `M2_EDGE_MQTT_URL`、topic prefix 为 `edge`，用户名与 ACL 对应；检查 Collector 的 `APP_MQTT__MASTER_SECRET` 在配置写入和进程重启间一致。
6. **没有 LWT offline**：确认是终止真实 Collector 进程而不是优雅关闭或手动发布 payload，并等待 Broker keepalive/LWT；同时检查脚本是否把 LWT envelope timestamp 错当成 Cloud `ReceivedAt`。
7. **出现重复 Edge 或时间倒退**：检查是否复用了旧数据库/Broker 数据、是否更换了 `M2_EDGE_ID`，并打印两次 online 的 `registeredAt`、`lastSeenAt` 和 Cloud receive time；不要用手工 SQL 修复后重跑。
8. **schema 断言失败**：直接查询 Cloud `edge_platform` 的 `information_schema`，确认连接的不是 Collector 数据库或长期开发数据库，并检查是否误把 Collector 自身的持久化表当成 Cloud schema。

验收完成后，应同时保存命令、环境覆盖、进程日志、REST 断言结果和 schema 查询结果；只有这些证据和项目规定的 Go race/vet、受影响集成测试、Web lint/build/browser 检查均通过，才能在 GitHub #19 上标记完成。
