# Realtime CurrentValue delivery 与 WebSocket 语义

- Status: Accepted
- Date: 2026-09-22

M5 在 M4 的 DataPoint / CurrentValue 之上增加浏览器实时分发能力。正式业务浏览器不直接连接 MQTT Broker；Cloud Server 负责把已经提交的 CurrentValue 以 latest-state 语义通过 WebSocket 推送给浏览器。

## 1. Scope and architecture

M5 只负责 CurrentValue realtime delivery：

```text
Edge Collector raw
  -> M4 CurrentValue commit
  -> post-commit notification
  -> process-local Realtime Hub
  -> WebSocket
  -> Browser realtime store
```

不在 M5 引入：
- CurrentValue history
- reliable WebSocket event log
- Redis/Kafka/NATS
- multi-instance fanout
- Command
- HMI editor

PostgreSQL CurrentValue 始终是 source of truth；Realtime Hub 是 best-effort process-local latest-state delivery。

## 2. WebSocket endpoint and authentication

浏览器先使用现有 Bearer session 申请一次性票据：

```text
POST /api/realtime/ticket
Authorization: Bearer ...
```

返回随机、不透明、一次性的短时 ticket。默认 TTL 30 秒。

ticket 绑定：
- userId
- auth session JTI
- auth session expiry

建立连接：

```text
GET /api/realtime/ws?ticket=...
```

不把长期 JWT 放在 query string，也不引入独立 WebSocket 登录体系。

upgrade 时必须再次确认绑定 session 当前有效。ticket 成功使用后立即失效。Cloud restart 后未使用 ticket 全部失效是允许的。

连接建立后至少每 30 秒重新检查 session 是否仍有效；session revoke、logout、用户禁用导致 session 失效时关闭连接。JWT/session 到期同样关闭。

正常 server shutdown 使用 WebSocket close code 1001；认证/授权失效使用 1008。

## 3. Subscription identity

公开 subscription identity 使用：

```text
deviceId + pointKey
```

而不是：
- MQTT topic
- sourceDeviceId
- register address
- DataPoint internal source mapping

服务器内部可以解析到 dataPointId。

一个 WebSocket connection 复用多个 point subscription。第一版单连接最多 1000 个 active points，单条协议消息最大 256 KiB。

## 4. Protocol

客户端命令：
- subscribe
- unsubscribe

服务器消息：
- subscribed
- unsubscribed
- snapshot
- update
- error

subscribe 支持批量 point：

```json
{
  "type": "subscribe",
  "requestId": "...",
  "points": [
    {"deviceId": "...", "pointKey": "current_a"}
  ]
}
```

重复 subscribe/unsubscribe 必须幂等。

一批订阅中某些 point 不存在时采用 partial result：有效 point 正常订阅，无效 point 在 subscribed/error 结果中单独返回；一个坏绑定不能使整批失败。

## 5. Realtime payload boundary

M5 realtime payload 只携带稳定语义与 CurrentValue：

- dataPointId
- deviceId
- pointKey
- valueType
- value
- quality
- sourceTimestamp
- observedAt
- revision

不携带 SourceMapping，也不携带 MQTT/Modbus 字段。

`name`、`unit`、`precision` 等可变 metadata 继续通过 REST 获取，不作为 realtime metadata 协议。

因此 metadata-only 修改不产生 realtime event；会真实改变 CurrentValue 的操作，例如 raw GOOD/BAD、mapping reset、disable/enable 导致 NO_DATA，必须产生 realtime update。

## 6. Snapshot/live race

订阅处理顺序：

```text
1. 注册 Hub subscription
2. 读取 PostgreSQL CurrentValue snapshot
3. 发送 snapshot
4. 继续 live updates
```

注册与 snapshot 查询之间可能产生 live update；客户端不依赖消息到达顺序，而是只按 M4 persisted `revision` 合并：

- incoming revision > local revision：接受
- incoming revision == local revision：幂等
- incoming revision < local revision：丢弃

因此 M5 不建立事件 replay，不需要数据库锁，也不暂停 live stream 等待 snapshot。

首次 subscribe、WebSocket reconnect、Cloud restart 后都通过新的 CurrentValue snapshot 恢复到最新状态。

## 7. Post-commit notification seam

M5 只允许在 CurrentValue 的数据库事务成功提交后发出 realtime notification：

```text
CurrentValue COMMIT
  -> best-effort non-blocking CurrentValueChange
  -> Realtime Hub
```

rollback 不得通知。

通知 seam 必须覆盖：
- raw projection GOOD/BAD
- mapping semantic reset -> NO_DATA
- enable/disable transition -> NO_DATA

metadata-only DataPoint update 不通知。

Realtime notification 不参与 M4 transaction；Hub 慢或不可用不能阻塞 DB commit、MQTT ingest 或 raw projection。

如果进程在 DB commit 成功后、notification 发出前崩溃，允许丢失该次 live event；客户端 reconnect 后通过 CurrentValue snapshot 恢复。M5 不为此引入 transactional outbox。

## 8. Backpressure and connection bounds

每个 WebSocket connection 使用 bounded outbound queue。

同一个 point 的多个尚未发送 update 可以 coalesce 到最高 revision。

如果大量不同 point 使 bounded queue 满：
- 不阻塞 M4
- 不阻塞 Realtime Hub 全局分发
- 关闭该慢客户端连接
- 浏览器重连并通过 snapshot 恢复

严禁 slow browser 向后传播 backpressure 到 CurrentValue DB commit 或 MQTT consumer。

服务器定期 ping，建议约 25 秒；浏览器断线后使用有上限的指数退避 + jitter 重连，例如 0.5 秒逐步增加到 15 秒。

## 9. Frontend store

前端 realtime store 使用：

```text
deviceId + pointKey
```

作为稳定 key，并保存最近 revision。

REST metadata store 与 realtime value store 分离；realtime update 只更新 CurrentValue 相关字段，不偷偷修改 name/unit/precision。

正式业务页面不得直接连接 MQTT Broker。

## 10. Authorization

ticket 创建需要有效 Bearer session。

M5 第一版 subscription 只订阅平台已有 DataPoint。若当前平台没有按 Device/DataPoint 的细粒度数据权限模型，M5 不自行发明资源级 ACL；它继承现有登录可见业务边界，并为未来授权 seam 保留接口。

WebSocket authentication 不能弱于 REST authentication。

## 11. Process model

Realtime Hub、ticket store、connection registry 都是当前 Cloud 进程内组件。

M5 明确只支持单 Cloud instance 下的 realtime fanout。多实例部署需要新的跨实例分发决策；不得在 M5 提前加入 Redis/NATS/Kafka。

## 12. Acceptance baseline

M5 真实验收链路必须使用真实 M4 数据：

```text
real Modbus simulator
-> real Edge Collector raw
-> M4 CurrentValue commit
-> realtime post-commit notification
-> WebSocket
-> browser
```

至少证明：
- 首次 subscribe 收到 PostgreSQL CurrentValue snapshot
- 后续真实 raw 产生 live update
- NUMBER / BOOLEAN 都可实时更新
- mapping reset / disable / enable 的 NO_DATA 实时到达
- snapshot/live race 通过 revision 正确合并
- multiple points 同连接订阅
- partial invalid subscription 不破坏有效订阅
- disconnect/reconnect 后重新 snapshot 恢复
- Cloud restart 后浏览器可重新 ticket/reconnect/resubscribe
- expired/used ticket 被拒绝
- logout/session revoke 后连接被关闭
- slow client 不阻塞 M4 ingest
- lower revision 被客户端忽略
- 正式浏览器没有 MQTT Broker 连接

## Consequences

- 浏览器获得稳定的 CurrentValue realtime 能力，但 PostgreSQL 仍是唯一恢复事实源。
- `revision` 成为 point-level freshness token。
- WebSocket 不承担可靠历史传输；丢 event 通过 snapshot 自愈。
- 当前设计刻意保持单实例，避免过早引入分布式 realtime infrastructure。
