# Edge Registration and Status Semantics

- Status: Accepted
- Date: 2026-09-21

Cloud 在第一次收到领域有效的 `edge-status/v1` 时自动登记 Edge；retained status、online status 和 LWT offline status 都可以触发登记，且登记以稳定、不可变并在 Cloud 部署内全局唯一的 `edgeId` 为身份。Cloud 不提供 M2 预配置前置条件，也不允许通过改名或自动合并改变既有 Edge 身份。

Edge 的 online/offline 是最近一个有效 EdgeStatus 的 `data.online` 断言，不是 Device 采集状态、Cloud MQTT Runtime 状态或基于沉默的 TTL 推断。Cloud 不从其他 MQTT 消息、连接断开或 `lastSeenAt` 超时合成 offline；因此 Collector 正常断开而未发布 offline 时，Cloud 保留最后已知状态。

`lastSeenAt` 使用 Cloud Ingest 的 `ReceivedAt`，包括 retained status 和 LWT status。EdgeStatus envelope 的 `SourceTimestamp` 不能替代它；尤其 LWT 的 timestamp 仅表示 Will 注册/生成时间，不表示实际离线时间。

## Delivery and adapter boundary

Cloud 按 EdgeStatus 到达 Cloud 的顺序更新 current state，不使用 `SourceTimestamp` 进行重排或抑制。相同状态的重复投递不会改变 online/offline 值，但会刷新 `lastSeenAt`；不同 `messageId` 的延迟或乱序消息无法仅凭当前 MQTT contract 识别为过时消息，因此按实际到达顺序作为最近观察到的断言处理。M2 不新增 raw payload 或 delivery history 保存机制。

EdgeStatus 只有在 topic/envelope identity 合法且 `data.online` 存在并为 boolean 时才是领域有效消息；`reason` 可缺省、可扩展，不决定状态。无效 EdgeStatus 不注册、不更新 Edge，并作为永久拒绝处理。

M2 仅消费 EdgeStatus。DeviceStatus、raw 和 DeviceEvent 即使接入层解析成功，也不创建 Edge、不更新 Edge 状态或 `lastSeenAt`，不持久化其 payload；它们在 M2 由接入 runtime 确认后结束。

## M2 domain boundary

M2 Edge 的领域字段严格限定为：

- `edgeId`：稳定且不可变的业务身份；
- `status`：`ONLINE` 或 `OFFLINE`；
- `registeredAt`：第一次领域有效 EdgeStatus 的 Cloud 接收时间；
- `lastSeenAt`：最近一次接受 EdgeStatus 的 Cloud 接收时间。

M2 不把 name、描述、位置、标签、Collector 版本/能力/配置、Broker/Topic/client ID、SourceTimestamp、messageId、retained、reason、Device 数量、`statusChangedAt` 或 `lastOnlineAt` 纳入 Edge 领域。Edge 发现后持续保留；M2 不提供删除、归档或禁用生命周期。

## REST baseline

M2 对外只提供经过认证的只读 Edge 查询：

```text
GET /api/edge/page
GET /api/edge/:edgeId
```

列表支持分页、可选 `status=ONLINE|OFFLINE` 精确状态过滤和可选 `edgeId` 精确过滤；默认稳定排序为 `registeredAt DESC, edgeId ASC`。列表/详情只返回 `edgeId`、`status`、`registeredAt`、`lastSeenAt`，时间为 RFC3339 UTC；未知 Edge 返回 404。M2 不提供写入接口，不返回 Device、MQTT 或 raw 字段，页面也不依赖 WebSocket。

## Acceptance baseline

M2 必须使用 PostgreSQL、Mosquitto、Cloud Server 和真实 Edge Collector 完成以下闭环：Collector 使用固定 edgeId 发布 retained online，Cloud 自动登记并在 REST/页面显示真实 Edge；Collector 被强制终止后通过 LWT 变为 offline，`lastSeenAt` 接近 Cloud 收到 LWT 的时间而不是 Will timestamp；Collector 使用相同 edgeId 重连后恢复 online、保持单条记录且 `registeredAt` 不变。重复、延迟、乱序 EdgeStatus 按 Cloud 接收顺序处理；DeviceStatus、raw、DeviceEvent 不创建或更新 Edge，也不被 M2 持久化。

## Consequences

- M2 不需要单独的 Edge provisioning 流程即可展示真实 Edge。
- retained offline 可以登记一个当前离线的 Edge；它仍然是 Broker 当前保留的状态断言。
- Cloud 不能仅凭消息沉默识别正常关闭或网络黑洞；若未来需要该语义，必须扩展上游 contract 或另行定义心跳/租约机制。
- Cloud 不承诺在没有上游序列号的情况下修复跨连接的延迟/乱序状态；若未来需要严格事件顺序，应扩展 MQTT contract，而不是让 Edge 领域猜测 SourceTimestamp。
- M2 的管理页面是 REST 查询页面，不提供 Edge 配置、生命周期操作或 WebSocket 实时推送。
