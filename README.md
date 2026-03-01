# gfgame-MMRPG游戏后端引擎

#### 介绍
gfgame 是基于 GoFrame 框架构建的 MMRPG 游戏后端引擎。引擎采用 ECS（Entity-Component-System）架构，支持帧同步（Lockstep）与状态同步（State Sync）双模式，提供层次化地图系统、装备系统、战斗系统等核心模块。
引擎是纯后端服务。前端（如 html5-mmrpg-game）通过 WebSocket/TCP 协议与引擎通信，完全解耦。

#### 软件架构

```
gfgame/
├── internal/
│   ├── config/       # 配置管理（ConfigManager）
│   ├── monitor/      # 性能监控 & WorkerPool 并行计算
│   ├── persistence/  # 持久化管理（PersistenceManager）
│   ├── engine/       # 引擎核心：GameLoop、ObjectPool、ECS 类型定义
│   ├── ecs/          # ECS 实体管理（EntityManager）
│   ├── network/      # 网络层：WebSocket/TCP 连接管理
│   ├── codec/        # 消息编解码：JSON + Compact 二进制协议
│   ├── sync/         # 同步系统：帧同步（LockstepSyncer）& 状态同步（StateSyncer）
│   ├── aoi/          # AOI 兴趣区域：空间索引（SpatialIndex）& 跨边界管理
│   ├── scene/        # 场景管理：多场景切换 & 无缝过渡
│   ├── map/          # 地图系统：MapManager、A* 寻路、高度图遮挡
│   ├── room/         # 房间管理：RoomManager & 实体迁移
│   ├── equipment/    # 装备系统：装备槽、属性加成
│   ├── combat/       # 战斗系统：伤害计算、装备锁定、数据缓存
│   └── plugin/       # 插件系统 & EventBus 事件总线
├── proto/
│   └── protocol.md   # 通信协议文档
├── sdk/
│   └── client.ts     # TypeScript 客户端 SDK
├── go.mod
└── go.sum
```

主要依赖：
- [GoFrame v2](https://github.com/gogf/gf) — 基础框架
- [gorilla/websocket](https://github.com/gorilla/websocket) — WebSocket 支持
- [google/uuid](https://github.com/google/uuid) — 实体 ID 生成

#### 安装教程

1. 确保已安装 Go 1.21+（本项目使用 go 1.25.0）
2. 克隆仓库后，在项目根目录执行依赖下载：
   ```bash
   go mod download
   ```
3. 运行所有单元测试，验证环境正常：
   ```bash
   go test ./internal/... -count=1
   ```

#### 使用说明

1. 创建并启动引擎实例：
   ```go
   import "gfgame/internal/engine"

   eng := engine.NewEngine(engine.Config{
       TickRate: 20, // 每秒 tick 次数
   })
   eng.Start()
   defer eng.Stop()
   ```

2. 注册消息处理器（通过 NetworkLayer）：
   ```go
   import "gfgame/internal/network"

   nl := network.NewNetworkLayer(network.Config{
       Address: ":8080",
       Mode:    network.ModeWebSocket,
   })
   nl.RegisterHandler(codec.MsgTypeMove, handleMove)
   nl.Start()
   ```

3. 使用插件系统扩展功能：
   ```go
   import "gfgame/internal/plugin"

   pm := plugin.NewPluginManager()
   pm.Register(myPlugin)
   pm.Start()
   ```

4. 前端通过 WebSocket 接入，协议格式参见 `proto/protocol.md`，TypeScript SDK 位于 `sdk/client.ts`。

#### 修罗斗场 Demo

多人实时对战演示，包含注册登录、职业选择（战士/弓箭手）、装备整备、实时 PVP 战斗。数据存储使用 SQLite（可替换为 MySQL）。

```bash
cd cmd/demo
go run .
```

浏览器访问 `http://localhost:9100`，可多开窗口模拟多人对战。

#### 参与贡献

1. Fork 本仓库
2. 新建 Feat_xxx 分支
3. 提交代码并确保 `go test ./internal/... -count=1` 全部通过
4. 新建 Pull Request
