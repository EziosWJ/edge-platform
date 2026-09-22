# Edge Platform Roadmap

本文件定义 Edge Platform 的正式开发阶段边界。GitHub Milestone 应与这里保持一致。

原则：

- 一个 Milestone 对应一个可独立验收的能力阶段。
- 当前 Milestone 未验收关闭前，不启动依赖它的下一阶段实现。
- 可以对后续阶段提前进行前瞻性 grill / ADR / spec，以发现上游模型缺口；这不代表实施授权，也不得提前实现未满足依赖的阶段。
- Matt skills 可以在当前实施 Milestone 内生成 implementation tickets，但不得扩大 Milestone Scope。
- 每个阶段生成的 Issue 必须关联到对应 GitHub Milestone。
- Milestone 100% Issue Closed 不代表自动验收通过；必须完成阶段 Acceptance 后再关闭 Milestone。

## M1 — MQTT Ingest Foundation

目标：Cloud Server 稳定连接 MQTT Broker，订阅并解析 Edge Collector MQTT v1 消息。

设计决策见 [ADR-0010: MQTT Ingest Runtime 与交付语义](adr/0010-mqtt-ingest-runtime-and-delivery-semantics.md)。

In Scope：
- MQTT configuration
- runtime lifecycle
- connect / reconnect / resubscribe
- MQTT v1 topic/envelope parsing
- runtime diagnostic status
- graceful shutdown
- basic metrics / integration tests

Out of Scope：
- Edge persistence
- Device
- DataPoint
- WebSocket
- Command
- HMI

Acceptance：
- Cloud 能稳定收到四类 MQTT v1 上行消息：Edge status、Device status、raw、event
- Broker 重启后自动恢复连接和订阅
- MQTT runtime 状态可诊断
- CI 全绿

## M2 — Edge Management

目标：建立 Cloud 侧 Edge 领域模型和在线状态。

In Scope：
- Edge model / migration
- edgeId identity
- online/offline/lastSeen
- Edge list/detail API
- Edge management page

Out of Scope：
- Device semantic model
- DataPoint
- Command

Acceptance：
- 页面能看到真实 Edge
- Edge 上下线状态变化正确
- lastSeen 正确更新

## M3 — Device Domain

目标：建立 Cloud 侧 Device 模型以及 Edge 与 Device 的关系。

In Scope：
- Device model / migration
- Edge-Device relationship
- deviceId identity
- device status
- Device list/detail API
- Device management page

Out of Scope：
- DataPoint semantic mapping
- history
- HMI

Acceptance：
- 能查看某个 Edge 下的设备
- 设备状态由 MQTT 数据正确更新

## M4 — DataPoint & CurrentValue

目标：把 Edge 原始上报转换为 Cloud 可复用的设备语义。

设计决策见 [ADR-0013: DataPoint identity、Raw mapping 与 CurrentValue 语义](adr/0013-datapoint-identity-raw-mapping-and-currentvalue-semantics.md)。

In Scope：
- DataPoint definition
- semantic point key
- raw -> semantic mapping
- CurrentValue
- quality / timestamp
- DataPoint API / UI

Out of Scope：
- history persistence
- HMI editor
- command

Acceptance：
- Cloud 能稳定形成如 current_a = 12.3 A 的语义数据
- 上层代码不依赖 Modbus register / MQTT topic

## M5 — Realtime WebSocket

目标：把 CurrentValue 实时推送给浏览器。

设计决策见 [ADR-0014: Realtime CurrentValue delivery 与 WebSocket 语义](adr/0014-realtime-currentvalue-delivery-and-websocket-semantics.md)。

In Scope：
- WebSocket lifecycle
- subscription model
- point-level realtime delivery
- reconnect behavior
- frontend realtime store

Out of Scope：
- Command
- history
- HMI editor

Acceptance：
- Edge 数据变化后浏览器实时更新
- 浏览器正式业务页面不直接连接 MQTT Broker

## M6 — Command Loop

目标：完成 Cloud -> Edge -> Cloud 的控制闭环。

设计决策见 [ADR-0015: Cloud Command identity、durable delivery 与 result 语义](adr/0015-cloud-command-identity-durable-delivery-and-result-semantics.md)。

In Scope：
- Command API
- authorization / audit
- MQTT command publish
- command-result consume
- command status lifecycle
- frontend command UX

Out of Scope：
- HMI editor
- advanced workflow / scheduling

Acceptance：
- 浏览器可发起真实控制
- Edge 执行后 Cloud 收到 command-result
- 成功/失败状态可追踪和审计

## M7 — History & Event

目标：形成基础历史数据和事件查询能力。

M7 在 M6 之后可以与 M8 独立推进；M8 不依赖 M7。

In Scope：
- point history persistence
- event persistence
- query API
- retention baseline
- basic trend/event UI

Out of Scope：
- advanced analytics
- rule engine
- long-term data warehouse

Acceptance：
- 可查询指定 DataPoint 的时间范围历史
- 可查询设备事件
- 前端能显示基础趋势图

## M8 — HMI MVP

目标：基于已稳定的 DataPoint / Realtime / Command 实现第一版组态编辑与运行。

设计决策见 [ADR-0016: HMI Page lifecycle、binding 与 Runtime 语义](adr/0016-hmi-page-lifecycle-bindings-and-runtime-semantics.md)。

M8 的硬依赖是 M4、M5、M6；不依赖 M7 History & Event。M6 验收关闭后，M7 与 M8 可以并行或按产品优先级独立实施。

In Scope：
- AntV X6 editor adapter
- custom versioned HMI page schema
- Draft / Published separation
- typed component registry
- DataPoint binding
- M5 realtime runtime
- Command binding
- M6 control integration
- edit/runtime separation
- basic industrial components

Out of Scope：
- history trend / event panel
- advanced SCADA scripting
- expression / rule engine
- complex animation engine
- automatic command / workflow
- custom HTML/React/JavaScript
- multi-user collaborative editing
- per-page ACL / anonymous runtime

Acceptance：
- 可创建、保存 Draft 并发布 immutable HMI 页面版本
- Draft 修改不会直接影响 Published Runtime
- 可拖入受控组件并绑定真实 DataPoint
- 运行态通过 M5 实时显示真实 CurrentValue，并正确展示 GOOD/BAD/NO_DATA
- button/switch 可通过 M6 发起真实 Command，且不 optimistic 修改现场状态
- HMI 不直接依赖 MQTT Topic / Modbus register / SourceMapping
- M7 未实现时 M8 完整 acceptance 仍可通过

## Recommended execution order

```text
M1 MQTT Ingest
→ M2 Edge
→ M3 Device
→ M4 DataPoint & CurrentValue
→ M5 WebSocket
→ M6 Command
     ├─→ M7 History & Event
     └─→ M8 HMI MVP
```

M7 / M8 在 M6 后无相互硬依赖；可并行，也可根据产品优先级选择 `M6 -> M8 -> M7` 或 `M6 -> M7 -> M8`。
