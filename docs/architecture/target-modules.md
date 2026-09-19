# Edge Platform 目标模块

本文只描述已确认的目标边界，不代表这些包已经实现。

```text
server/internal/
├── app          # composition root / HTTP routes / runtime lifecycle
├── platform     # database, HTTP, logging 等通用基础设施
├── auth         # 已迁入
├── rbac         # 已迁入
├── edge         # Edge 节点
├── device       # 云端设备
├── datapoint    # 设备数据点语义
├── realtime     # CurrentValue 与实时分发
├── event        # 事件/历史入口
├── command      # 控制意图与状态
├── mqtt         # MQTT subscriber/publisher runtime
└── hmi          # HMI 页面/schema
```

模块在首次业务实现时创建，不为目标结构预建空 package。

核心依赖方向：

```text
MQTT -> Edge/Device -> DataPoint -> Realtime -> WebSocket
HMI  -> DataPoint
HMI  -> Command -> MQTT
Event/History <- MQTT/DataPoint
```

MQTT payload 解析应停留在接入层；HMI 不直接解析 MQTT payload。
