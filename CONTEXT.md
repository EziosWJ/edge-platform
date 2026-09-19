# Edge Platform 项目上下文

## 项目定位

Edge Platform 是与 Edge Collector 配套的云端工业 IoT / HMI 平台。

- Edge Collector 负责现场协议、Modbus/串口/TCP、Starlark 业务脚本、轮询、现场采集和最终控制执行。
- Edge Platform 负责 Edge/Device 管理、设备语义、DataPoint、实时状态、历史数据、事件、Command、告警与 HMI。
- Edge 与 Cloud 之间的业务通信边界是 MQTT v1 contract。
- 浏览器不是正式 MQTT 数据消费端。Cloud Server 负责 MQTT 订阅/发布，Web 通过 REST + WebSocket 获取业务数据。

## 仓库结构

```text
/
├── server/        # Go API 与 Cloud runtime
├── web/           # React 管理后台与未来 HMI
├── docs/
│   ├── adr/
│   ├── architecture/
│   └── agents/
├── CONTEXT.md
└── Taskfile.yml
```

## 技术基线

### Server

- Go 1.26
- Gin
- GORM
- Goose migration
- Koanf
- JWT session
- Prometheus
- Swagger
- PostgreSQL 是 Cloud 正式生产数据库
- SQLite 代码暂保留用于本地/测试兼容，不属于 Cloud 生产部署目标

### Web

- React 19
- TypeScript
- Vite 6
- Tailwind CSS
- Zustand
- react-hook-form + zod
- HMI 编辑器确定采用 AntV X6，但在 HMI 模块真正实施时再引入依赖
- 实时业务数据由 Cloud Server 通过 WebSocket 推送；不要让正式业务页面直接连接 MQTT Broker

## 架构形态

第一阶段采用 Modular Monolith，不拆微服务。

```text
Edge Collector
     │
     │ MQTT v1
     ▼
┌──────────────────────── Edge Platform Server ────────────────────────┐
│ MQTT runtime                                                       │
│      │                                                             │
│      ├─ Edge / Device ─ DataPoint ─ CurrentValue ─ WebSocket ──┐   │
│      │                                                         │   │
│      ├─ Event / History                                       Web  │
│      │                                                         │   │
│      └─ Command ─ MQTT publish ─ Edge Collector                │   │
│                                                                 │   │
│                        PostgreSQL                               │   │
└─────────────────────────────────────────────────────────────────┴───┘
```

业务模块按领域组织，不建立全局 controller/service/repository 技术分层目录。推荐模块边界：

```text
server/internal/
├── app/
├── platform/
├── auth/
├── rbac/
├── edge/          # 待实现
├── device/        # 待实现
├── datapoint/     # 待实现
├── realtime/      # 待实现
├── event/         # 待实现
├── command/       # 待实现
├── mqtt/          # 待实现
└── hmi/           # 待实现
```

不要仅为了体现目标结构创建空包；在对应业务开始实现时创建。

## 已确认领域术语

**Edge**：运行 Edge Collector 的边缘节点。Cloud 通过 MQTT 标识和维护其连接/状态。

**Device**：由某个 Edge 管理的现场设备。Cloud 不直接理解 Modbus 地址、串口参数、Unit ID 等 Edge 内部采集细节。

**DataPoint**：Cloud 对设备数据的语义化数据点，例如 `current_a`、`temperature`、`breaker_status`。HMI、历史、告警等上层能力绑定 DataPoint，而不是直接绑定 MQTT Topic 或原始寄存器地址。

**CurrentValue**：某个 DataPoint 的当前值及其时间、质量等状态。

**Command**：Cloud 发起的设备控制意图。Cloud 经 MQTT 下发，Edge Collector 在既有安全边界内执行并通过 command-result 返回状态。

**HMI**：基于 DataPoint 和 Command 的组态展示/控制页面。编辑器计划使用 AntV X6；持久化使用自定义 HMI schema，不把 X6 JSON 直接当作不可替换的领域模型。

## Edge / Cloud 边界

Cloud 可以依赖 Edge Collector 已公开的 MQTT contract，但不得把这些 Edge 内部概念扩散到 HMI 和 Cloud 业务层：

- Modbus register address
- serial port
- Unit ID
- poll block
- Starlark runtime internals

Cloud 领域层应面向 Edge、Device、DataPoint、Event、Command 等语义。

## 数据库约束

Cloud 正式生产只以 PostgreSQL 为目标。新 Cloud 业务 migration 默认写 PostgreSQL migration 树。

SQLite 现有实现保留的目的仅是脚手架兼容、本地开发或快速测试；新增 Cloud 业务不得为了 SQLite 生产兼容而牺牲 PostgreSQL 所需的数据建模能力。若未来决定彻底删除 SQLite，应单独记录 ADR 并清理相应代码与 migration。

## 继承自脚手架的稳定基础能力

以下能力由 base-project-golang 迁入并继续复用：

- JWT 登录与 session 撤销
- 用户、角色、菜单与部门
- 系统字典和配置
- 文件管理
- 登录日志和操作审计
- 站内通知
- 统一 HTTP response/error
- request_id 与基础可观测性
- Swagger
- PostgreSQL migration 与集成测试基线

这些是平台基础设施，不应为了 IoT 业务重写。

## 开发规则

1. 先确认领域边界，再增加抽象；第一阶段保持模块化单体。
2. Handler 只负责 HTTP；Service 负责业务规则；Repository 负责持久化。
3. MQTT 是 Cloud runtime 基础设施，但 MQTT Topic 不应成为 HMI/DataPoint 的领域主键。
4. Schema 变更必须提交 Goose migration。
5. REST API 变更同步 Swagger/契约文档。
6. Server 改动至少执行 `go test ./...` 与 `go vet ./...`；Web 改动执行 lint 和 build。
7. 不提前引入 Redis、Kafka、微服务、Gateway、Kubernetes 或分布式事务。
8. 不恢复脚手架 Demo 页面或伪造 Dashboard KPI。

## 当前实施顺序

当前只完成 Cloud 工程基线迁移和架构重定向。后续优先顺序：

```text
MQTT ingest
→ Edge
→ Device
→ DataPoint
→ CurrentValue
→ WebSocket
→ Command 闭环
→ History/Event
→ HMI editor/runtime
```

架构决策见 `docs/adr/0001-cloud-platform-scope-and-edge-cloud-boundary.md`。
