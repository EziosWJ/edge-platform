# HMI Page lifecycle、binding 与 Runtime 语义

- Status: Accepted
- Date: 2026-09-22

M8 在已经稳定的 DataPoint / CurrentValue、Realtime WebSocket 与 Command 能力之上实现第一版 HMI 编辑与运行。M8 不依赖 M7 History & Event；M6 完成后，M7 与 M8 可以独立推进。

## 1. Scope and dependency

M8 正式依赖：

```text
M4 DataPoint / CurrentValue
  -> M5 Realtime WebSocket
  -> M6 Command Loop
  -> M8 HMI MVP
```

M7 History & Event 不属于 M8 的硬依赖。M8 MVP 不实现历史趋势、事件列表、告警历史或历史回放。

M8 只负责：
- HMI Page lifecycle
- AntV X6 editor
- custom versioned HMI document
- component registry
- DataPoint binding
- Command binding
- draft/published separation
- runtime rendering
- realtime subscription integration
- command interaction integration

不引入：
- history/event UI
- alarm engine
- SCADA scripting
- expression engine
- automatic command/workflow
- multi-user collaborative editing
- per-page ACL
- custom React/HTML execution

## 2. HMI Page lifecycle

Cloud HMI Page 使用全局稳定、不透明 UUID `pageId`。

第一版生命周期支持：
- create
- metadata update
- save draft
- publish
- read published runtime

不提供 hard delete、rollback、version restore、archive workflow 或 public anonymous runtime。

逻辑模型至少包含：

```text
HmiPage
  pageId
  name
  description
  draftDocument
  draftRevision
  publishedVersionId nullable
  createdAt
  updatedAt

HmiPageVersion
  versionId
  pageId
  versionNo
  sourceDraftRevision
  document
  publishedBy
  publishedAt
```

HmiPageVersion 是 immutable snapshot。

## 3. Draft / Published isolation

Editor 只修改 Draft；Runtime 只加载 Published version。

```text
Editor
  -> edit local state
  -> Save Draft
  -> server draftDocument + draftRevision
  -> Publish
  -> immutable HmiPageVersion

Runtime
  -> publishedVersionId
  -> immutable document
```

编辑 draft 不能影响当前运行页。没有 published version 时 runtime 必须返回明确的 `HMI_PAGE_NOT_PUBLISHED`，不得 fallback 到 draft。

Save Draft 与 Publish 是两个独立动作。浏览器存在未保存 local changes 时 Publish 应禁用；Publish 操作只针对服务器已经保存的 draftRevision。

## 4. Canonical HMI document

HMI 持久化使用 Cloud 自定义、显式版本化的 canonical schema，而不是 AntV X6 `graph.toJSON()` 输出。

M8 schema：

```json
{
  "schema": "hmi-page/v1",
  "canvas": {
    "width": 1920,
    "height": 1080
  },
  "nodes": []
}
```

Document 可保存为 PostgreSQL JSONB，但 JSONB 只是 canonical domain document 的持久化载体，不等于 X6 serialization contract。

未知 schema version 必须拒绝或经过显式 migration；不得静默改变 v1 语义。

## 5. Canvas and coordinates

M8 使用固定 logical canvas，不做 responsive reflow engine。

支持预设 logical size：
- 1920x1080
- 1366x768
- 1280x720

运行态等比缩放并居中：

```text
scale = min(viewportWidth / canvasWidth,
            viewportHeight / canvasHeight)
```

Node 持久化 logical pixel：
- nodeId UUID
- x
- y
- width
- height
- rotation
- zIndex
- type
- props
- bindings

rotation 第一版只允许 0/90/180/270。

页面使用 flat `nodes[] + zIndex`。M8 不建立 nested scene graph、深层 group hierarchy、master symbol、component instance inheritance。

复制节点必须生成新 nodeId。

## 6. Editor/X6 boundary

AntV X6 只负责编辑器交互，不是领域模型。

```text
Canonical HMI Document
       <-> adapter
AntV X6 Graph
```

禁止直接持久化：
- X6 internal shape identifiers
- ports/internal metadata
- viewport
- zoom/pan
- selection
- plugin state
- undo/redo history
- clipboard

这些 editor ephemeral state 不进入 canonical document。

Undo/redo 只存在当前 browser editing session。

Editor preview 与 Runtime 使用同一个 React Component Registry renderer；X6 node 只是 editor interaction shell，避免 editor/runtime 两套组件行为漂移。

## 7. Optimistic draft locking

HMI 不做多人实时协作，但必须防 lost update。

读取 draft 时返回 `draftRevision`。保存 draft 必须带 `expectedDraftRevision`。

```text
expected != current
  -> HTTP 409

expected == current
  -> save canonical draft
  -> draftRevision + 1
```

冲突由前端提示用户重新加载/处理，不静默覆盖另一浏览器已经保存的 draft。

## 8. Publish semantics and idempotency

Publish 请求携带 `expectedDraftRevision`。

流程：
1. 验证 current draftRevision
2. 对该 draft 进行 strict publish validation
3. 在一个 PostgreSQL transaction 中插入 immutable HmiPageVersion
4. 更新 HmiPage.publishedVersionId

HmiPageVersion 记录 `sourceDraftRevision`。

同一个 page + sourceDraftRevision 已成功发布后，重复 publish retry 返回已有 version，不创建新的 versionNo。这保证 HTTP response 丢失后的 publish retry 幂等。

## 9. Validation levels

### Save Draft

Draft 保存只要求结构安全，允许半成品：
- schema supported
- canonical JSON shape valid
- document size within limit
- canvas valid
- unique nodeId
- known component type
- geometry/zIndex/rotation types valid
- props/bindings basic JSON shape valid

Draft 可以暂时缺少 required binding。

### Publish

Publish 必须完整严格校验：
- every node type known
- required props valid
- required binding present
- DataPoint exists
- DataPoint valueType compatible with binding slot
- Command target Device exists
- Command name non-empty/bounded
- args JSON object
- ttlSeconds within M6 limits
- generated command request within M6 size contract
- switch state/on/off command use same Cloud Device
- no script/expression/custom HTML/CSS execution

任一 node 不合法，整页禁止发布；M8 不支持带 warning 发布。

## 10. Component Registry

M8 Component Registry 是 allowlisted/versioned typed registry。

第一版组件仅：
- `text`
- `shape`
- `value-display`
- `indicator`
- `gauge`
- `button`
- `switch`

不实现：
- image
- trend
- event panel
- table
- iframe
- arbitrary SVG/script
- custom component code

Registry 为每个 component 定义：
- allowed props
- defaults
- typed binding slots
- publish validation
- shared editor/runtime renderer

Schema 不允许任意 React source、HTML、JavaScript、Starlark、CSS/className/Tailwind string。

## 11. Static components

`text` 只允许 plain text，不支持 HTML、Markdown、template/interpolation。

`shape` 第一版只支持：
- rectangle
- ellipse
- line

shape 可以使用受控静态颜色值/semantic style，但不能携带任意 CSS。

M8 不用 text component 插值 DataPoint；动态值必须使用 value-display 等 typed components。

## 12. DataPoint binding

Canonical DataPoint binding identity 固定为：

```json
{
  "kind": "datapoint",
  "deviceId": "cloud-device-uuid",
  "pointKey": "current_a"
}
```

HMI 不绑定 MQTT topic、sourceDeviceId、register address 或 SourceMapping。

Registry slot compatibility baseline：
- value-display.value -> NUMBER | BOOLEAN
- indicator.value -> BOOLEAN
- gauge.value -> NUMBER
- switch.state -> BOOLEAN

Publish 时 Server 解析当前 DataPoint，并严格验证 existence/valueType。

DataPoint metadata（name/unit/precision）不复制进 published binding identity；Runtime bootstrap 从当前 M4 DataPoint 读取 metadata。因此 metadata-only change 不要求重新 publish HMI。

DataPoint 被 disable 后 published HMI version 仍合法；Runtime 按 M4/M5 当前值显示 NO_DATA/unavailable，而不是让整个 page 失效。

## 13. Realtime integration

Runtime 只通过 M5 WebSocket 获取 CurrentValue。

页面加载后：
1. 获取 runtime bootstrap
2. 从 published document 收集、去重所有 `deviceId + pointKey`
3. 获取 M5 one-time ticket
4. 建立一个 WebSocket connection
5. batch subscribe
6. shared realtime store 按 revision 合并
7. all components 从 shared store render

同一个 point 被多个组件绑定只订阅一次。

M8 单页不建立多 WebSocket connection 来绕过 M5 active-point limit。

Runtime 不维护第二套 freshness 规则；CurrentValue 仍按 M5 `revision` 合并。

## 14. Point quality rendering

M8 必须统一展示 M4 quality，不允许组件关闭质量状态提示：

- GOOD：正常显示 value
- BAD：保留并显示最后 GOOD value，但必须明确标识 invalid/BAD
- NO_DATA：显示 unavailable/`--`，不显示旧值

HMI 不根据 DeviceStatus、EdgeStatus、WebSocket disconnect、observedAt age 或自定义 timeout 重算 point quality。

WebSocket transport status 与 point quality 是两种事实。断线时页面显示明确的 realtime disconnected/reconnecting 状态，同时保持当前 point quality/value，不篡改它们。

## 15. Presentation semantics

M8 不执行第二次业务转换。

禁止：
- `value > threshold ? styleA : styleB` expression
- value arithmetic
- conditional visibility expression
- dynamic rotation/position
- DataPoint-to-DataPoint formula

M4 SourceMapping 是业务数值转换边界。

允许有限的 component-specific semantic states，例如 indicator true/false style token、switch on/off style。style 使用 Registry semantic token，而不是任意 CSS。

`value-display` / `gauge` 可以有 presentation-only precision override，但不得改变 underlying CurrentValue。unit 默认来自当前 DataPoint metadata；组件只能决定是否展示 unit，不重新定义业务单位。

Gauge min/max 仅影响视觉范围。超范围值可以视觉 clamp，但文本仍显示真实 value；M8 不把超范围自动解释为 alarm。

## 16. Command binding

M8 复用 M6 Command API，不创建 CommandDefinition/schema discovery。

Canonical binding：

```json
{
  "kind": "command",
  "deviceId": "cloud-device-uuid",
  "name": "close",
  "args": {},
  "ttlSeconds": 30,
  "confirmation": {
    "required": true,
    "message": "确认执行？"
  }
}
```

M8 command args 仅允许 static JSON object literal。

禁止：
- args 引用 DataPoint
- expression-generated args
- JavaScript/Starlark args
- operator arbitrary JSON input
- automatic command from point changes
- timer/page-load command

新建 command binding 默认 `confirmation.required=true`；编辑者可以显式关闭，用于低风险操作。Confirmation 只是 operator UX，不替代 M6 server-side authorization/TTL/Edge safety。

每次明确 operator action 生成新的 commandId；同一次 action 的 HTTP retry 必须复用原 commandId，继承 M6 idempotency。

## 17. Button and Switch semantics

`button` 是纯 Command control，不绑定 DataPoint 来推导其业务状态。

`switch` 包含三个 binding：
- state: BOOLEAN DataPoint
- onCommand: Command
- offCommand: Command

state/on/off command 必须绑定同一个 Cloud Device。

Control component 禁止 optimistic process-state update。

```text
operator action
  -> Command lifecycle feedback

actual switch state
  -> only M5 CurrentValue update
```

即使 Command SUCCEEDED，也不能直接写入/伪造 switch state。只有对应 BOOLEAN CurrentValue 发生真实变化后 UI 才更新现场状态。

同一个 control component 在本地一次 action 尚未 terminal 或 create request 失败前临时禁用，防止双击产生多个 commandId。这是 UX，不替代 M6 idempotency。

## 18. Command authorization and feedback

HMI binding 不授予命令执行权限。

`hmi:run` 与 `command:execute` 完全独立。Runtime 发起控制仍必须通过 M6 server-side `command:execute` authorization。

没有 command:execute 的用户：
- 可以按 hmi:run 查看运行页和 realtime
- control 显示不可执行 UX
- server POST command 仍必须 403

HMI 只维护当前 operator action 的局部 Command feedback，并通过 M6 REST detail polling（约 2s）观察 PENDING/ACCEPTED/terminal。terminal 后停止 polling。

Command state 不持久化到 HMI document，也不能直接修改 DataPoint realtime store。

## 19. HMI authorization

M8 最低 server-side permission：
- `hmi:list`
- `hmi:detail`
- `hmi:edit`
- `hmi:publish`
- `hmi:run`

`hmi:edit` 覆盖 create、metadata update、save draft。

第一版不建立：
- per-page ACL
- owner/share model
- department-specific page ACL
- anonymous/public page

只有 `hmi:run` 权限的用户不能通过 Runtime API 取得 draft 内容。

## 20. REST surfaces

Authenticated management baseline：
- `GET /api/hmi/page/page`
- `GET /api/hmi/page/:pageId`
- `POST /api/hmi/page`
- `PUT /api/hmi/page/:pageId`
- `PUT /api/hmi/page/:pageId/draft`
- `POST /api/hmi/page/:pageId/publish`

Authenticated runtime：
- `GET /api/hmi/page/:pageId/runtime`

不提供 DELETE、rollback/restore、anonymous runtime。

Runtime response 是 bootstrap，而不是裸 document：

```text
page metadata + published version/document
+ deduplicated current DataPoint metadata needed by bindings
```

CurrentValue realtime 数据不复制到 HMI persistence；页面运行后通过 M5 snapshot/live 获取。

## 21. Runtime persistence boundary

HMI database 只持久化：
- HmiPage metadata
- draft document
- draftRevision
- immutable published versions
- publishedVersionId

不持久化：
- CurrentValue
- WebSocket connection/session state
- runtime selected node
- Command pending feedback
- last clicked action
- realtime subscription state

Cloud restart 后 runtime 重新加载同一 published version，获取新 M5 ticket/WebSocket，并以 fresh CurrentValue snapshot 恢复。

## 22. Limits

M8 MVP 固定边界：
- canonical document <= 2 MiB
- nodes <= 500
- unique DataPoint bindings <= 1000

超过限制的 draft 可在保存时直接拒绝；至少 Publish 必须拒绝。不得为大页面自动创建多个 realtime connections 来绕过限制。

## 23. Failure isolation

Runtime 必须局部降级：
- one component render error -> node-level error fallback
- one DataPoint unavailable -> that component unavailable
- one Command failure -> that control shows failure
- WebSocket disconnected -> layout remains visible + reconnect indication

一个 node 的 React exception 不得导致整页 crash。shared component renderer 应有 node-level Error Boundary 或等价机制。

Published document 理论上经过 strict validation，但 runtime 仍需 defensive handling unknown/corrupt component data。

## 24. X6 round-trip contract

M8 必须测试 canonical document 与 X6 editor adapter 的 round trip。

保证 canonical fields 不丢失，同时以下 X6/editor internals 不进入持久化 document：
- selection
- viewport/zoom/pan
- undo history
- plugin state
- internal cell shape/type
- ports/internal metadata

这条测试是“X6 不是 domain schema”的架构 gate。

## 25. Real acceptance

增加独立：

```text
task m8:hmi-acceptance
```

环境：
- PostgreSQL
- Mosquitto
- Cloud Server
- real Edge Collector
- real Modbus simulator
- browser

必须证明页面生命周期：
- create page
- save draft
- publish
- runtime 只看到 published version
- draft 后续修改不影响 runtime
- concurrent draft save -> 409
- repeated same-draft publish retry 不创建第二个 version
- invalid binding/type mismatch 不能 publish

真实数据链：

```text
Modbus
-> Edge Collector
-> M4 CurrentValue
-> M5 WebSocket
-> HMI value-display/indicator/gauge/switch
```

至少证明：
- NUMBER / BOOLEAN 实时更新
- BAD 保留最后值但明确 invalid
- NO_DATA 显示 unavailable
- WebSocket disconnect 不改变 point quality
- reconnect snapshot 恢复
- duplicate binding 只产生一个 point subscription

真实控制链：

```text
HMI button/switch
-> M6 Command
-> MQTT
-> real Edge Collector Starlark
-> real Modbus
-> CommandResult
```

至少证明：
- button 执行真实 Command
- switch 不 optimistic update
- Command SUCCEEDED 不直接改变 switch state
- 真正 DataPoint 更新后 switch 才改变
- no command:execute -> server 403
- one in-flight local action prevents accidental double-trigger

还必须证明：
- Cloud restart 后 published runtime 恢复
- browser 不直接连接 MQTT Broker
- M7 History/Event 完全未实现时 M8 acceptance 仍全部通过

## 26. Consequences

- M8 与 M7 解耦，M6 后可以并行开发。
- HMI 页面版本冻结 layout/component/binding，而 DataPoint metadata 继续读取当前 Cloud semantic metadata。
- AntV X6 可替换，因为 canonical document 不依赖 X6 JSON。
- Draft 与 Published 强隔离，编辑不会直接影响生产运行页。
- HMI 只是 display/control layer，不成为第二个数值计算、quality、rule、workflow 或 command-definition engine。
- Realtime 与 Command 继续分别服从 M5/M6 已冻结的恢复、授权和幂等语义。
