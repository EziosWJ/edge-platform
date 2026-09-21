# Device identity、发现与通信状态语义

- Status: Accepted
- Date: 2026-09-21

M3 为 Device 分配 Cloud 全局唯一且稳定的随机 UUID `deviceId`，并另存来源映射 `(edgeId, sourceDeviceId)`；数据库以 `device_id` 为主键并为来源二元组建立唯一约束。API 把 UUID 表示为不透明字符串，调用方不得依赖其生成顺序或内部结构。REST、未来 DataPoint 与 HMI 使用 Cloud `deviceId`，MQTT Topic/envelope 中 Collector 范围的 `deviceId` 对外称为 `sourceDeviceId`。不同 Edge 可以上报相同 `sourceDeviceId`，它们自动发现为不同 Device；现有协议不能证明两个来源代表同一物理设备，因此从 Edge A 改到 Edge B 在 M3 是新发现，不自动迁移或合并，未来迁移必须由显式生命周期契约完成。

第一次为已登记 Edge 收到领域有效的 `device-status/v1` 时自动登记 Device。DeviceStatus 永远不能创建 Edge。若 DeviceStatus 先于父 EdgeStatus 到达，Cloud ACK 该消息且不创建 Edge 或 Device；父 Edge 完成 Registration 后触发一次合并、限频的 DeviceStatus 重新订阅，让 Broker 重放 retained current status。接收侧本次 delivery 的 retained flag 不用于判断该消息是否领域有效或上游是否按契约发布：对已经存在的订阅者，原始发布即使设置 retain=true，实时转发的 delivery 也可以表现为 retained=false；只有 Broker 作为 retained replay 交付时该标志才可靠表示“这是保留消息重放”。因此未知父 DeviceStatus 不因 retained=false 被视为毒消息，也不通过不 ACK/reconnect 循环驱动重投。该协调只维护一个有界的重放信号，不保存 payload 或通用消息 staging；如果上游实际上没有保留 current DeviceStatus，replay 后仍没有对应状态，M3 不通过缓存、消息历史或无限重试弥补该上游契约缺失。结构/语义无效的 DeviceStatus 仍永久拒绝并 ACK，基础设施或数据库失败仍可重试。

父约束只要求 Edge 已登记，不要求它当前 `ONLINE`；已登记但 `OFFLINE` 的 Edge 仍可发现或更新 Device。DeviceStatus 不得改变 Edge 的状态或 `lastSeenAt`。每次真正插入新 Edge 时，协调器请求 MQTT Runtime 重新订阅既有 DeviceStatus wildcard；它最多维护一个执行中操作和一个 dirty 标志，并发 Registration 合并，执行期间的新 Registration 最多触发一次后续重放。失败由 MQTT Runtime 使用既有有界退避恢复，不通过 DeviceStatus 的不 ACK/reconnect 循环驱动；进程崩溃后，正常启动订阅也会重新取得 retained 状态。

Device 当前投影包含 Cloud `deviceId`、`edgeId`、`sourceDeviceId`、`communicationStatus`、`registeredAt`、`lastSeenAt`、`lastAttemptAt`、`lastSuccessAt` 和 `communicationError`。`registeredAt` 与 `lastSeenAt` 使用 Cloud `ReceivedAt`；后两个采集时间保留 Collector 来源语义。Envelope `SourceTimestamp`、`messageId`、Topic、QoS 和 retained 元数据不进入 Device 领域投影。

DeviceStatus 的 `data` 必须完整包含 `status`、`lastAttemptAt`、`lastSuccessAt` 和 `error`：状态只能是四个已知枚举，两个来源时间只能是合法 RFC3339/RFC3339Nano 字符串或 `null`，错误只能是字符串或 `null`，空字符串规范化为 `null`；未知扩展字段允许存在。来源时间解析后规范化为 UTC，不因与 Cloud 时钟偏差而拒绝，也不额外要求字段之间满足 Collector 当前实现通常成立的先后或非空关系。结构或语义无效是永久拒绝并 ACK；context 取消、数据库暂时失败等基础设施错误可重试且不 ACK。QoS/retain 偏离仍按 ADR-0010 记录 contract violation，不单独使已有合法父 Edge 的有效 payload 失效。

每条 DeviceStatus 的状态、两个来源时间和错误组成不可拆分的 current-state 快照。`registeredAt` 原子保留最早 Cloud `ReceivedAt`，`lastSeenAt` 原子保留最大 Cloud `ReceivedAt`，当前快照只由最大 `ReceivedAt` 的观察更新；相同时间按数据库语句顺序处理。重复、QoS1 重投和 retained 重放仍刷新 `lastSeenAt`，不按 `messageId` 去重；`SourceTimestamp` 不用于重排或抑制，M3 不保存 DeviceStatus 历史。

`INITIAL`、`ONLINE`、`DEGRADED`、`OFFLINE` 原样投影 Collector 的采集通信断言，Cloud 不折叠为二态，也不重新计算失败阈值。Device 状态与 Edge MQTT 会话状态是不同事实，因此 `Edge OFFLINE + Device ONLINE` 是合法的最后已知投影，Cloud 不因 Edge 离线、消息沉默、MQTT Runtime 状态或其他消息类型合成 Device OFFLINE。

Collector 当前删除设备或修改来源身份时不发布 retained tombstone 或 device-deleted 事件，因此 M3 的自动发现记录持续保留；删除不会合成 `OFFLINE`、`DELETED` 或 `STALE`，来源改名会登记新 Device 并保留旧 Device。同一 Edge 删除后复用相同 `sourceDeviceId` 时，Cloud 无法证明物理设备已经更换，因此继续更新原 Cloud Device，并保持其 `deviceId` 和 `registeredAt`。M3 不提供删除、归档、禁用、迁移、合并或手工修复，也不基于 `lastSeenAt` 自动标记失联或过期。

M3 只有 DeviceStatus 可以创建或更新 Device。EdgeStatus 只更新 Edge；合法 raw-register-snapshot/v1 与 device-event/v1 由 MQTT Runtime 正常接受并 ACK，但不创建或更新 Device，也不刷新 Device 或 Edge 的 `lastSeenAt`。M3 不保存 raw、DeviceEvent 或其 payload；非法消息仍由 MQTT Ingest 按 ADR-0010 拒绝。

## REST baseline

M3 提供全局只读资源 `GET /api/device/page` 与 `GET /api/device/:deviceId`。分页查询支持 `edgeId`、Cloud `deviceId`、`sourceDeviceId` 和四态 `status` 精确过滤，默认稳定排序为 `registeredAt DESC, deviceId ASC`；返回且只返回 M3 接受的身份、状态、诊断与时间字段。独立 Device 管理页面可从 Edge 列表携带 `edgeId` 进入过滤视图，列表展示身份、归属、四态及 Cloud 时间，详情展示来源采集时间和当前通信错误；不提供创建、编辑、删除或 WebSocket 实时更新。

## Acceptance baseline

M3 增加独立的 `task m3:device-acceptance`，复用 M2 的 PostgreSQL、Mosquitto、Cloud Server 和真实 Edge Collector 基座，并接入真实 Modbus simulator；它不属于默认 `task check`。验收必须证明真实 Collector 自动发现 Device 并产生 `INITIAL`、`ONLINE`、`DEGRADED`、`OFFLINE` 四态，重复、重连和 retained 重放不产生重复记录，`registeredAt` 稳定且 `lastSeenAt` 按 Cloud 观察前进。第二个真实 Collector 使用不同 `edgeId` 和相同 `sourceDeviceId` 时必须形成独立 Cloud Device；Edge 离线不得覆盖 Device 状态，DeviceStatus 不得更新 Edge，真实 raw 与 DeviceEvent 不得改变 Device 投影。

无法靠 Broker 稳定强制的 DeviceStatus 先于 EdgeStatus、重复、延迟以及数据库并发完成顺序，由确定性的后端集成测试覆盖；真实链路不假设 retained 投递顺序。验收还必须查询 PostgreSQL schema，确认只增加 Device current projection 及必要主键、唯一约束、索引和 Edge 外键，不出现 DataPoint、CurrentValue、raw/history、DeviceEvent、HMI、通用 pending-message 或消息历史持久化。验收使用独立临时环境，失败保留诊断日志，正常结束清理自身资源。
