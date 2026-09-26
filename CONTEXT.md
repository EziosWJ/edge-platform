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

**Edge Registration**：Cloud 第一次收到领域有效的 EdgeStatus 时认识并登记一个 Edge。Registration 不等同于人工预配置，也不会因为 DeviceStatus、raw 或 DeviceEvent 自动创建 Edge。
_Avoid_：provisioning、Device registration

**RegisteredAt**：Edge 第一次完成 Registration 时的 Cloud 接收时间。它在 Edge 的生命周期内保持不变。
_Avoid_：provisionedAt、createdAt（当它被用来表示 Collector 时间时）

**EdgeStatus**：Edge 对自身 MQTT 会话状态的上行断言。它只表达 Edge 的 online/offline，不表达其下属 Device 的采集状态。
_Avoid_：Device status、MQTT Runtime status

**Domain-valid EdgeStatus**：Topic/envelope identity 合法且 `data.online` 存在并为 boolean 的 EdgeStatus。`reason` 可以缺省或扩展，不改变 online/offline 的含义。
_Avoid_：raw status、collector internal status

**Edge Online / Edge Offline**：Cloud 按收到顺序对最近一个领域有效 EdgeStatus 的 `data.online` 值进行投影。Cloud 不因 SourceTimestamp、沉默、`lastSeenAt` 超时、自己的 MQTT 连接状态或其他消息类型推断状态。
_Avoid_：reachable、healthy、Device online

**LastSeenAt**：Cloud 最近收到并接受 EdgeStatus 的时间，使用 Cloud 的 ReceivedAt；retained、LWT 和重复投递也会更新它。它不是 SourceTimestamp，也不是实际的 offline 时间。
_Avoid_：offlineAt、lastOnlineAt

**Discovered Edge**：通过 EdgeStatus 自动登记且持续保留的 Edge。M2 不为 Edge 引入删除、归档或禁用状态；offline 只表示最近的 EdgeStatus 断言，不表示记录失效。
_Avoid_：provisioned Edge、temporary Edge

**Device**：Cloud 中可被 DataPoint、Command 和 HMI 稳定引用的现场设备。Device 拥有独立于来源 Edge 的 Cloud 全局身份；Cloud 不直接理解 Modbus 地址、串口参数、Unit ID 等 Edge 内部采集细节。
_Avoid_：Collector device、`(edgeId, deviceId)` 复合身份

**Cloud Device ID**：Cloud 为 Device 分配的全局唯一且稳定的不透明 UUID，对外字段名为 `deviceId`。Device 后续即使发生显式迁移，上层绑定也继续引用这个身份，调用方不得解释其生成顺序或内部结构。
_Avoid_：sourceDeviceId、MQTT deviceId、数据库自增 ID

**Source Device ID**：Edge Collector 在自身范围内分配并作为 MQTT Topic/envelope `deviceId` 上报的设备身份，对外字段名为 `sourceDeviceId`。它只在所属 Edge 内唯一，不能单独作为 Cloud Device 或 HMI 的身份。
_Avoid_：Cloud Device ID、全局 deviceId

**Device Registration**：Cloud 第一次为已登记 Edge 接受领域有效的 DeviceStatus 时认识并登记一个 Device。DeviceStatus 不会创建 Edge，自动发现也不会把不同来源身份猜测为同一 Device。
_Avoid_：Device provisioning、Edge Registration

**Discovered Device**：通过 DeviceStatus 自动登记的 Device。在出现明确的设备生命周期契约前，来源变更、删除或改名不会自动删除、迁移或合并既有 Device。
_Avoid_：temporary Device、provisioned Device

**DeviceStatus**：Edge Collector 对某个 Device 当前采集通信状态及诊断事实的上行断言。它不表达所属 Edge 的 MQTT 会话状态。
_Avoid_：EdgeStatus、DeviceEvent、raw snapshot

**Domain-valid DeviceStatus**：Topic/envelope 来源身份合法，`data` 完整包含四态 `status`、可空的 `lastAttemptAt` 与 `lastSuccessAt` 来源时间以及可空的 `error` 字符串，且各字段类型和格式符合 DeviceStatus 契约的断言。未知扩展字段不改变有效性，领域有效性也不由 raw 或 DeviceEvent 补足。
_Avoid_：raw communication status、inferred Device status

**Device Initial / Online / Degraded / Offline**：Collector 断言的四态采集通信状态：尚未形成可确认通信结果、最近完整采集成功、最近采集部分成功或尚未达到离线阈值、连续全失败达到离线阈值。Cloud 投影该断言，不自行计算阈值，也不因 Edge Offline 覆盖它。
_Avoid_：Edge online/offline、reachable、Cloud-computed status

**Device RegisteredAt**：Cloud 第一次完成 Device Registration 时的 ReceivedAt，在 Device 生命周期内保持不变。
_Avoid_：SourceTimestamp、lastAttemptAt、createdAt（当它被用来表示来源时间时）

**Device LastSeenAt**：Cloud 最近收到并接受该 Device 的领域有效 DeviceStatus 的 ReceivedAt；retained 和重复投递同样属于新的 Cloud 观察。
_Avoid_：SourceTimestamp、lastAttemptAt、lastSuccessAt

**LastAttemptAt / LastSuccessAt**：Collector 分别记录最近一次采集通信尝试和最近一次成功的来源时间。它们保留来源侧语义，不代表 Cloud 接收时间。
_Avoid_：Device LastSeenAt、SourceTimestamp

**Communication Error**：DeviceStatus 携带的当前 Collector 诊断文本；它解释最近通信状态，但不是稳定错误码或历史事件。
_Avoid_：DeviceEvent、Alarm、error code

**DataPoint**：Cloud 手工定义的设备语义数据点。Cloud 为其分配稳定的全局 UUID `dataPointId`，并以 `(deviceId, pointKey)` 作为稳定业务唯一键；例如 `current_a`、`temperature`、`breaker_status`。pointKey 与 valueType 创建后不可修改。HMI、历史、告警等上层能力绑定 DataPoint，而不是直接绑定 MQTT Topic 或原始寄存器地址。
_Avoid_：MQTT topic、register address 作为上层数据点身份

**SourceMapping**：DataPoint 到 Edge raw register 的 Cloud 内部映射配置边界。M4 允许该边界理解 functionCode、address、encoding、word/byte order、bitIndex、scale/offset，但这些字段不得扩散到 CurrentValue、WebSocket、History 或 HMI 业务模型。
_Avoid_：把 Modbus mapping 当成 DataPoint identity

**CurrentValue**：某个 DataPoint 的当前语义投影，与 DataPoint configuration 独立持久化。它包含 typed value、`NO_DATA|GOOD|BAD` quality、来源值时间 `sourceTimestamp`、Cloud observation 时间 `observedAt` 与单调递增 `revision`。BAD 保留最近 GOOD value/sourceTimestamp；Device/Edge 状态不会自动覆盖 point quality。
_Avoid_：history record、Device status、SourceMapping

**Point Quality**：M4 CurrentValue 的可信度事实。`NO_DATA` 表示配置尚未由新 raw 形成可判定值，`GOOD` 表示本次 raw 可完整解码，`BAD` 表示已收到领域有效 raw 但该 mapping 当前无法形成新有效值。M4 不引入 UNCERTAIN。
_Avoid_：把 Device OFFLINE 直接解释为 BAD

**Realtime Subscription**：浏览器对 `deviceId + pointKey` 的当前值实时订阅。M5 通过 Cloud WebSocket 分发已提交的 CurrentValue；PostgreSQL CurrentValue 是恢复事实源，WebSocket 只承载 best-effort latest-state 更新，客户端按 `revision` 合并新旧状态。
_Avoid_：MQTT subscription、可靠事件历史、register subscription

**WebSocket Ticket**：由有效 Bearer session 换取的一次性、短时、不透明连接凭据，只用于 WebSocket upgrade。它绑定 user/session 身份，不替代 JWT session，也不在 URL 中暴露长期 token。
_Avoid_：把 JWT 直接放 WebSocket query string

**Command**：Cloud 持久化的设备控制意图与执行生命周期事实。它以 Cloud `deviceId` 为目标，创建时冻结 `edgeId + sourceDeviceId` 路由、name、args、issuedAt/expiresAt，并通过同一个 commandId 跨 HTTP retry、Cloud MQTT retry 与 Edge journal 保持幂等。
_Avoid_：MQTT publish record、直接使用 sourceDeviceId 作为业务控制身份

**Command Delivery**：Command 的短生命周期可靠下行传输记录，保存冻结 topic 与完全相同的 MQTT payload，在 expiresAt 前有界重发。MQTT PUBACK 只表示 Broker 收到，不能把 Command 解释为 ACCEPTED；只有 Edge command-result 可以改变业务执行状态。
_Avoid_：通用 Cloud outbox、把 PUBACK 当设备执行确认

**Command Result**：Edge Collector 对既有 Command 的执行事实，包括 ACCEPTED/REJECTED/EXPIRED/SUCCEEDED/FAILED 及来源时间、result/error。`edgeReceivedAt` 表示 Command 在 Edge 首次接收/准入的时间，并在同一 Command 的 ACCEPTED/FINAL 中保持不变；`startedAt`、`completedAt` 是 Edge 执行时间；`resultReceivedAt` 是 Cloud 收到该结果的时间。结果 envelope 的 `timestamp` 表示 Edge 发布/生成该结果的时间。Cloud 必须校验 commandId、冻结 route 和 name；FINAL 可以先于 ACCEPTED 到达，终态采用 first-terminal-wins。
_Avoid_：Cloud timeout 推导出的执行结果

**HMI**：基于 DataPoint、M5 Realtime 与 M6 Command 的组态展示/控制页面。M8 的硬依赖是 M4/M5/M6，不依赖 M7 History & Event；编辑器使用 AntV X6，但持久化使用自定义 versioned canonical schema，不把 X6 JSON 当领域模型。
_Avoid_：browser MQTT、register-bound HMI、X6 JSON 作为持久化 contract

**HMI Page**：Cloud 中可编辑和发布的 HMI 页面身份，以稳定 `pageId` 表示。Page 持有可变 Draft 与当前 `publishedVersionId`；Runtime 永远只加载 Published version，Draft 修改不会直接影响运行画面。
_Avoid_：把 Draft 直接作为生产 Runtime 页面

**HMI Page Version**：由一次 Publish 从已保存 Draft 生成的 immutable 页面快照，记录 versionNo 与 sourceDraftRevision。相同 sourceDraftRevision 的重复 Publish 必须返回同一版本而不是制造新版本。
_Avoid_：mutable published document、X6 history

**HMI Document**：显式版本化的 Cloud canonical 页面文档，例如 `hmi-page/v1`，只保存 canvas、node geometry、typed props 与 bindings。X6 viewport、selection、plugin state、undo/redo 等编辑器临时状态不得进入该文档。
_Avoid_：`graph.toJSON()` 直接存库

**HMI Component Registry**：M8 受控、强类型的组件注册表，定义允许的 component type、props、binding slot 与 shared editor/runtime renderer。M8 不允许用户注入 React、HTML、JavaScript、Starlark、任意 CSS 或通用 expression。
_Avoid_：custom code component、expression engine

**HMI DataPoint Binding**：HMI 对实时语义点的稳定绑定，canonical identity 为 `deviceId + pointKey`。Publish 校验 DataPoint 存在且 valueType 与组件 slot 兼容；Runtime 通过 M5 shared realtime store 消费 CurrentValue，不依赖 SourceMapping。
_Avoid_：dataPoint source mapping、MQTT topic、sourceDeviceId、register address

**HMI Command Binding**：HMI 对 M6 Command API 的声明式控制绑定，由 Cloud `deviceId`、command name、静态 JSON object args、TTL 与可选确认提示组成。一次明确 operator action 创建一个新 commandId；HMI binding 本身不授予 `command:execute` 权限。
_Avoid_：自动命令、动态表达式 args、CommandDefinition shadow model

**HMI Runtime**：只加载 Published HMI Page Version 的运行态。Runtime 从 bootstrap 获取页面与当前 DataPoint metadata，用一个 M5 WebSocket 对页面内去重后的 `deviceId + pointKey` 批量订阅；Command 继续走 M6 REST。Runtime 不持久化 CurrentValue、WebSocket state 或 Command pending state。
_Avoid_：运行 draft、每组件独立 WebSocket、HMI 自建 realtime/command store

**MQTT Ingest**：Cloud 接收并解析 Edge Collector MQTT v1 上行消息的接入能力。它产出带接收元数据的强类型接入消息，但不代表 Edge、Device、DataPoint 或 Event 已完成领域建模或持久化。
_Avoid_：MQTT 业务模型、MQTT 领域模型

**MQTT Runtime**：Edge Platform Server 进程内负责 Broker 连接、订阅恢复和 MQTT Ingest 生命周期的运行组件。
_Avoid_：MQTT 微服务、MQTT Gateway

**SourceTimestamp**：Edge Collector 写入消息 envelope 的来源时间。它用于表达来源侧观察或产生消息的时间，不等同于 Cloud 收到消息的时间。
_Avoid_：ReceivedAt、Cloud 时间

**ReceivedAt**：Cloud MQTT Ingest 收到消息时记录的本地时间。它不替代 SourceTimestamp，也不单独证明 Edge 或 Device 当前在线。
_Avoid_：SourceTimestamp、设备时间

**DeviceEvent**：Edge Collector 通过 `device-event/v1` 上报的通用设备事件。它不是 Alarm；只有后续领域规则明确赋予告警语义时才成为告警。
_Avoid_：Alarm、告警

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

实施依赖顺序：

```text
MQTT ingest
→ Edge
→ Device
→ DataPoint
→ CurrentValue
→ WebSocket
→ Command 闭环
     ├─→ History/Event
     └─→ HMI editor/runtime
```

History/Event 与 HMI 在 Command 闭环之后没有相互硬依赖，可以并行或按产品优先级独立推进。前瞻性 grill / ADR / spec 可以提前完成，但不得把未满足依赖的阶段提前进入 implementation。

架构决策见 `docs/adr/0001-cloud-platform-scope-and-edge-cloud-boundary.md`。