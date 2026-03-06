# 需求文档

## 简介

gogame 是轻量级 MMRPG 纯游戏后端引擎，不包含任何前端展示和交互逻辑。引擎支持帧同步（Lockstep）与状态同步（State Sync）双模式，核心卖点为千人同屏战斗。引擎需提供主流游戏引擎的核心功能模块，包括网络通信、场景管理、实体管理、战斗系统、AOI（Area of Interest）系统等，为大规模多人在线角色扮演游戏提供高性能、可扩展的服务端解决方案。

作为纯后端引擎，gogame 的设计重点在于扩展性和易用性：通过清晰的通信协议定义、标准化的客户端接入 API 和完善的接口文档，方便 H5 网页端、Unity 客户端、原生 App 等不同类型的前端系统快速接入。当前已有独立的 H5 前端项目 html5-mmrpg-game 通过 WebSocket 协议与引擎通信，该前端项目与 gogame 完全解耦，仅通过网络协议交互。

## 术语表

- **Engine**: 基于 GoFrame 框架构建的 MMRPG 游戏引擎服务端核心，不包含任何前端逻辑
- **Client**: 连接到 Engine 的游戏客户端实例，可以是 H5 网页端、Unity 客户端或原生 App 等
- **H5_Client**: 基于 html5-mmrpg-game 项目的 H5 网页前端客户端，通过 WebSocket 协议与 Engine 通信
- **Lockstep_Sync**: 帧同步模块，所有 Client 以相同的逻辑帧推进，Engine 收集并广播操作指令
- **State_Sync**: 状态同步模块，Engine 运行权威游戏逻辑并将状态快照下发给 Client
- **Sync_Manager**: 同步管理器，负责在 Lockstep_Sync 和 State_Sync 之间进行模式选择与切换
- **Scene_Manager**: 场景管理器，负责游戏世界的分区、加载与卸载
- **Entity_Manager**: 实体管理器，负责游戏实体（角色、NPC、怪物、道具等）的生命周期管理
- **AOI_Manager**: 兴趣区域管理器，负责计算并维护每个实体的可见范围内的其他实体集合
- **Combat_System**: 战斗系统，处理技能释放、伤害计算、Buff/Debuff 等战斗逻辑
- **Network_Layer**: 网络层，基于 GoFrame 提供 TCP/WebSocket 连接管理与消息收发
- **Message_Codec**: 消息编解码器，负责网络消息的序列化与反序列化
- **Protocol_Buffer**: 使用 Protocol Buffers 定义的网络消息协议格式
- **Protocol_Doc**: 协议文档，描述所有客户端与服务端之间通信消息的结构、字段和交互流程的文档
- **Client_SDK**: 客户端接入 SDK，封装了与 Engine 通信的底层细节，提供高层 API 供前端调用
- **Tick**: 服务端逻辑帧，Engine 以固定频率执行的一次完整逻辑更新周期
- **Game_Loop**: 游戏主循环，按固定 Tick 频率驱动所有游戏逻辑的更新
- **ECS**: Entity-Component-System 架构模式，将实体数据（Component）与行为逻辑（System）分离
- **Spatial_Index**: 空间索引结构（如九宫格、四叉树），用于加速空间查询
- **Room**: 游戏房间，一个独立的游戏逻辑运行实例，承载一组 Client 的交互
- **Map**: 游戏地图，包含地形数据、可行走区域、障碍物信息和传送点等空间信息的逻辑单元，由一个或多个 Map_Layer 组成
- **Map_Layer**: 地图层，Map 中的一个独立空间层级，代表不同的高度或空间区域（如地面层、地下层、桥梁层、山洞层等），每层拥有独立的地形数据和可行走区域
- **Layer_Connection**: 层连接点，连接同一 Map 内不同 Map_Layer 的通道（如山洞入口、楼梯、传送门等），实体可通过 Layer_Connection 在层之间切换
- **Height_Map**: 高度图，记录 Map_Layer 中每个网格单元的高度值，用于模拟高山、悬崖等地形的高度差
- **Occlusion**: 遮挡关系，基于 Height_Map 和 Map_Layer 层级计算的视野遮挡效果，高处地形可遮挡低处实体的视野
- **Map_Manager**: 地图管理器，负责地图的加载、卸载、切换和寻路计算，支持多层地图的管理
- **Pathfinding**: 寻路模块，基于 A* 或类似算法在 Map 的可行走区域上计算路径，支持跨 Map_Layer 的多层寻路
- **Seamless_Transition**: 无缝转场，玩家在相邻地图、不同 Map_Layer 或不同 Room 之间移动时无需加载画面，体验连续不中断
- **Boundary_Zone**: 边界区域，两个相邻 Map 或 Room 之间的重叠区域，该区域内的实体对两侧均可见，用于实现无缝过渡
- **Cross_Boundary_AOI**: 跨边界 AOI，在 Boundary_Zone 内运行的特殊 AOI 计算逻辑，使玩家在边界区域时可以看到相邻 Map 或 Room 中的实体
- **Boundary_Preload**: 边界预加载，当玩家接近地图或 Room 边界时，Engine 提前将相邻区域的数据推送给 Client，确保过渡时无需等待加载
- **Entity_Migration**: 实体迁移，实体从一个 Room 平滑转移到另一个 Room 的过程，迁移期间实体状态不丢失、操作不中断
- **Equipment_System**: 装备系统，负责角色装备的穿戴、卸下和属性计算
- **Equipment_Slot**: 装备槽位，角色身上可穿戴装备的部位（如武器、头盔、铠甲、鞋子等）
- **Equipment_Lock**: 装备锁定状态，战斗期间参战角色的装备被锁定，禁止切换操作
- **Combat_Context**: 战斗上下文，战斗开始时从持久化存储提取到内存中的参战实体战斗相关数据集合，包含属性、技能、Buff 等
- **Compact_Codec**: 紧凑编解码器，将玩家操作编码为极短的字符串或字节序列用于网络传输，服务端和客户端共用编解码逻辑
- **Operation_Code**: 操作码，单字符编码表示特定操作类型（如 u=上移、d=下移、l=左移、r=右移、a=攻击、s=技能等）
- **Short_ID**: 短标识符，将长实体 ID 映射为短字符标识用于紧凑协议传输，减少网络数据量
- **Compact_Message**: 紧凑消息，由 Short_ID、Operation_Code 和参数组成的极短编码字符串，如 "Au10aB" 表示用户A向上移动10像素并攻击用户B
- **Operation_Dictionary**: 操作码字典，定义所有 Operation_Code 与操作类型之间映射关系的配置表

## 需求

### 需求 1：网络通信层

**用户故事：** 作为游戏开发者，我希望引擎提供高性能的网络通信层，以便支持大量客户端（包括 H5 网页端和原生客户端）的并发连接与低延迟消息传输。

#### 验收标准

1. THE Network_Layer SHALL 基于 GoFrame 的 gnet 或 gtcp 模块提供 TCP 长连接服务
2. THE Network_Layer SHALL 提供独立的 WebSocket 连接端点，作为 H5_Client 的主要接入方式
3. THE Network_Layer SHALL 对 TCP 连接和 WebSocket 连接提供统一的会话抽象接口，使上层业务逻辑无需感知底层传输协议差异
4. WHEN Client 发起连接请求时，THE Network_Layer SHALL 在 100ms 内完成连接握手并分配唯一会话标识
5. THE Network_Layer SHALL 支持不少于 5000 个 Client 的并发连接（TCP 与 WebSocket 连接合计）
6. WHEN Client 断开连接时，THE Network_Layer SHALL 在 3 秒内检测到断连并触发断连事件通知
7. IF Network_Layer 检测到连接异常，THEN THE Network_Layer SHALL 记录异常日志并通知 Session_Manager 清理对应会话资源
8. THE Network_Layer SHALL 支持 WebSocket 连接的心跳检测机制，心跳间隔和超时时间可配置
9. THE Network_Layer SHALL 支持 WebSocket 的 wss（TLS 加密）连接模式，保障 H5_Client 的通信安全

### 需求 2：消息协议与编解码

**用户故事：** 作为游戏开发者，我希望引擎使用高效且文档化的消息协议，以便减少网络带宽占用、提高消息处理速度，并方便外部前端系统对接。

#### 验收标准

1. THE Message_Codec SHALL 使用 Protocol_Buffer 作为消息序列化格式
2. THE Message_Codec SHALL 为每条消息定义唯一的消息 ID 和消息体结构
3. WHEN 收到原始字节流时，THE Message_Codec SHALL 按照「消息长度 + 消息ID + 消息体」的格式进行拆包解析
4. FOR ALL 有效的消息对象，经过序列化再反序列化后 SHALL 产生与原始对象等价的结果（往返一致性）
5. THE Message_Codec SHALL 提供消息的格式化输出功能，将消息对象转换为可读的文本格式用于调试
6. IF Message_Codec 收到格式不合法的消息数据，THEN THE Message_Codec SHALL 丢弃该消息并记录包含来源 Client 标识的错误日志
7. THE Message_Codec SHALL 同时支持 Protocol_Buffer 二进制格式和 JSON 文本格式的消息编解码，WebSocket 连接可选择使用 JSON 格式以方便 H5_Client 调试
8. THE Compact_Codec SHALL 将玩家操作编码为紧凑的字符串或字节序列，编码格式由 Short_ID + Operation_Code + 参数组成（如 "Au10aB" 表示用户A向上移动10像素并攻击用户B）
9. THE Compact_Codec SHALL 维护 Operation_Dictionary，定义移动（u/d/l/r）、攻击（a）、技能（s）等操作类型的单字符 Operation_Code 映射
10. THE Compact_Codec SHALL 维护实体 Short_ID 映射表，将长实体 ID 映射为单字符或短字符标识用于网络传输
11. WHEN 同一 Tick 内同一实体产生多个操作时，THE Compact_Codec SHALL 将多个操作合并为一条 Compact_Message 进行传输
12. THE Compact_Codec SHALL 提供 Encode 和 Decode 方法，服务端和客户端共用相同的编解码逻辑
13. FOR ALL 有效的操作序列，经过 Compact_Codec 编码再解码后 SHALL 产生与原始操作序列等价的结果（往返一致性）
14. THE Compact_Codec SHALL 提供格式化输出功能，将 Compact_Message 解码为可读的操作描述文本用于调试
15. IF Compact_Codec 收到格式不合法的 Compact_Message，THEN THE Compact_Codec SHALL 丢弃该消息并记录包含来源信息的错误日志

### 需求 3：游戏主循环

**用户故事：** 作为游戏开发者，我希望引擎提供稳定的游戏主循环，以便所有游戏逻辑按固定频率精确执行。

#### 验收标准

1. THE Game_Loop SHALL 以可配置的固定频率（默认 20 Tick/秒）驱动游戏逻辑更新
2. THE Game_Loop SHALL 维护单调递增的逻辑帧号计数器
3. WHEN 单个 Tick 的处理时间超过帧间隔时，THE Game_Loop SHALL 记录性能警告日志并跳过渲染帧以追赶逻辑帧
4. WHILE Game_Loop 运行期间，THE Game_Loop SHALL 按照「输入处理 → 逻辑更新 → 状态同步 → 清理」的固定顺序执行每个 Tick
5. WHEN 收到关闭信号时，THE Game_Loop SHALL 完成当前 Tick 的处理后安全停止，并持久化必要的游戏状态


### 需求 4：帧同步模块

**用户故事：** 作为游戏开发者，我希望引擎支持帧同步模式，以便在小规模高精度战斗场景中实现确定性一致的游戏体验。

#### 验收标准

1. THE Lockstep_Sync SHALL 收集同一逻辑帧内所有 Client 的操作指令并打包为帧数据包
2. WHEN 一个逻辑帧的所有 Client 操作指令收集完毕或等待超时（可配置，默认 100ms）到达时，THE Lockstep_Sync SHALL 将该帧数据包广播给 Room 内所有 Client
3. THE Lockstep_Sync SHALL 保证所有 Client 收到的帧数据包内容和顺序完全一致
4. WHEN Client 的操作指令在超时时间内未到达时，THE Lockstep_Sync SHALL 使用空操作指令填充该 Client 的帧数据
5. THE Lockstep_Sync SHALL 保存最近 N 帧（可配置，默认 300 帧）的历史帧数据用于断线重连追帧
6. WHEN Client 断线重连时，THE Lockstep_Sync SHALL 将该 Client 缺失的历史帧数据按顺序发送以实现状态追赶

### 需求 5：状态同步模块

**用户故事：** 作为游戏开发者，我希望引擎支持状态同步模式，以便在千人同屏等大规模场景中实现高效的状态分发。

#### 验收标准

1. THE State_Sync SHALL 在 Engine 端运行权威游戏逻辑，Client 仅作为输入发送方和状态展示方
2. WHEN 一个 Tick 的逻辑更新完成后，THE State_Sync SHALL 计算本帧与上一帧之间的状态差异（Delta）
3. THE State_Sync SHALL 仅将状态差异数据（而非完整状态快照）下发给相关 Client 以减少带宽占用
4. THE State_Sync SHALL 每隔 K 帧（可配置，默认 100 帧）生成一次完整状态快照用于校验和断线重连
5. WHEN Client 断线重连时，THE State_Sync SHALL 发送最近的完整状态快照及后续的差异数据以恢复 Client 状态
6. WHILE 千人同屏场景运行期间，THE State_Sync SHALL 结合 AOI_Manager 的结果仅向 Client 同步其兴趣区域内的实体状态

### 需求 6：同步模式管理

**用户故事：** 作为游戏开发者，我希望引擎能灵活切换帧同步和状态同步模式，以便根据不同游戏场景选择最优的同步策略。

#### 验收标准

1. THE Sync_Manager SHALL 支持按 Room 粒度独立配置同步模式（Lockstep_Sync 或 State_Sync）
2. WHEN 创建 Room 时，THE Sync_Manager SHALL 根据 Room 配置参数初始化对应的同步模块实例
3. THE Sync_Manager SHALL 提供统一的同步接口抽象，使上层业务逻辑无需感知具体的同步模式
4. WHEN Room 内的 Client 数量超过可配置阈值（默认 50 人）时，THE Sync_Manager SHALL 记录建议切换为 State_Sync 模式的警告日志

### 需求 7：实体管理（ECS 架构）

**用户故事：** 作为游戏开发者，我希望引擎采用 ECS 架构管理游戏实体，以便实现高性能的数据驱动游戏逻辑和良好的可扩展性。

#### 验收标准

1. THE Entity_Manager SHALL 为每个实体分配全局唯一的实体 ID
2. THE Entity_Manager SHALL 支持动态地为实体添加和移除 Component
3. THE Entity_Manager SHALL 支持注册 System，每个 System 声明其关注的 Component 类型组合
4. WHEN 每个 Tick 执行时，THE Entity_Manager SHALL 按照 System 的注册优先级顺序依次执行各 System 的更新逻辑
5. THE Entity_Manager SHALL 支持通过 Component 类型组合高效查询符合条件的实体集合
6. WHEN 实体被销毁时，THE Entity_Manager SHALL 移除该实体的所有 Component 并从所有 System 的处理列表中清除

### 需求 8：场景管理

**用户故事：** 作为游戏开发者，我希望引擎提供场景管理功能，以便支持大规模游戏世界的分区管理和动态加载。

#### 验收标准

1. THE Scene_Manager SHALL 支持将游戏世界划分为多个独立的 Scene 实例
2. THE Scene_Manager SHALL 支持 Scene 的动态创建、加载和卸载
3. WHEN Client 请求进入某个 Scene 时，THE Scene_Manager SHALL 验证准入条件后将 Client 对应的实体添加到目标 Scene
4. WHEN Client 从一个 Scene 切换到另一个 Scene 时，THE Scene_Manager SHALL 确保实体从源 Scene 移除并添加到目标 Scene，过程中不丢失实体状态
5. THE Scene_Manager SHALL 为每个 Scene 维护独立的 Entity_Manager 和 AOI_Manager 实例
6. WHILE Scene 内无活跃 Client 超过可配置时间（默认 5 分钟）时，THE Scene_Manager SHALL 自动卸载该 Scene 并释放资源
7. WHEN 实体在相邻 Scene 之间切换时，THE Scene_Manager SHALL 实现 Seamless_Transition，Client 无需经历加载画面即可完成场景过渡
8. THE Scene_Manager SHALL 为相邻 Scene 之间维护 Boundary_Zone 配置，定义两个 Scene 的重叠区域范围
9. WHILE 实体位于 Boundary_Zone 内时，THE Scene_Manager SHALL 确保该实体同时对源 Scene 和目标 Scene 的 AOI_Manager 可见
10. WHEN 实体接近 Scene 边界（距离 Boundary_Zone 可配置阈值内）时，THE Scene_Manager SHALL 触发 Boundary_Preload，将相邻 Scene 的边界区域数据提前推送给对应 Client
11. WHEN 实体从 Boundary_Zone 完全进入目标 Scene 时，THE Scene_Manager SHALL 将该实体的归属从源 Scene 切换到目标 Scene，切换过程中实体状态和 Client 操作不中断

### 需求 9：AOI（兴趣区域）系统

**用户故事：** 作为游戏开发者，我希望引擎提供高效的 AOI 系统，以便在千人同屏场景中精确控制每个客户端的可见范围，降低网络和计算开销。

#### 验收标准

1. THE AOI_Manager SHALL 使用 Spatial_Index（九宫格或四叉树）对 Scene 内的实体进行空间索引
2. THE AOI_Manager SHALL 为每个实体维护一个可见实体列表（观察者列表）
3. WHEN 实体位置发生变化时，THE AOI_Manager SHALL 更新该实体的空间索引位置并重新计算其可见实体列表
4. WHEN 实体进入另一个实体的 AOI 范围时，THE AOI_Manager SHALL 触发「进入视野」事件通知
5. WHEN 实体离开另一个实体的 AOI 范围时，THE AOI_Manager SHALL 触发「离开视野」事件通知
6. WHILE 千人同屏场景运行期间，THE AOI_Manager SHALL 在单个 Tick 内完成所有实体的 AOI 更新计算
7. THE AOI_Manager SHALL 支持可配置的 AOI 半径参数，不同实体类型可设置不同的 AOI 范围

### 需求 10：战斗系统

**用户故事：** 作为游戏开发者，我希望引擎提供完整的战斗系统框架，以便快速实现 MMRPG 的核心战斗玩法。

#### 验收标准

1. THE Combat_System SHALL 提供技能定义框架，支持通过数据配置定义技能的释放条件、作用范围、伤害公式和效果
2. THE Combat_System SHALL 支持基于 AOI 范围的目标选取，包括单体、扇形、圆形、矩形等选取模式
3. WHEN 角色释放技能时，THE Combat_System SHALL 按照「前摇 → 判定 → 结算 → 后摇」的阶段顺序处理技能流程
4. THE Combat_System SHALL 支持 Buff/Debuff 系统，包括效果叠加、持续时间管理和效果移除
5. WHEN 伤害结算完成后，THE Combat_System SHALL 生成包含攻击者、受击者、技能ID、伤害值、暴击标记等信息的战斗日志事件
6. IF 技能释放过程中释放者死亡或被控制，THEN THE Combat_System SHALL 根据技能配置的中断策略取消或继续该技能的后续阶段


### 需求 11：房间管理

**用户故事：** 作为游戏开发者，我希望引擎提供房间管理功能，以便将玩家组织到独立的游戏实例中进行交互。

#### 验收标准

1. THE Engine SHALL 支持动态创建和销毁 Room 实例
2. THE Engine SHALL 为每个 Room 分配独立的 Game_Loop 和同步模块实例
3. WHEN Client 请求加入 Room 时，THE Engine SHALL 验证 Room 容量限制和准入条件后将 Client 加入 Room
4. WHEN Room 内最后一个 Client 离开后超过可配置时间（默认 30 秒），THE Engine SHALL 自动销毁该 Room 并释放资源
5. THE Engine SHALL 支持 Room 的最大容量配置，State_Sync 模式下的 Room 容量上限不低于 1000 人
6. WHEN Room 达到容量上限时，THE Engine SHALL 拒绝新的加入请求并返回容量已满的错误信息
7. WHEN Client 从一个 Room 切换到另一个 Room 时，THE Engine SHALL 实现 Seamless_Transition，Client 无需断开连接即可完成 Room 切换
8. THE Engine SHALL 为相邻 Room 之间支持 Boundary_Zone 配置，定义两个 Room 的边界共享区域
9. WHILE 实体位于 Room 的 Boundary_Zone 内时，THE Engine SHALL 通过 Cross_Boundary_AOI 使该实体可以看到相邻 Room 中 Boundary_Zone 内的实体
10. WHEN 实体从一个 Room 迁移到另一个 Room 时，THE Engine SHALL 执行 Entity_Migration，确保迁移过程中实体状态完整保留且 Client 操作不中断
11. WHILE Entity_Migration 进行期间，THE Engine SHALL 保证实体在源 Room 和目标 Room 之间的状态一致性，不产生状态丢失或重复
12. WHEN Room 之间执行 Entity_Migration 时，THE Engine SHALL 在目标 Room 确认接收实体后再从源 Room 移除该实体，采用「先加后删」策略防止实体丢失

### 需求 12：千人同屏性能优化

**用户故事：** 作为游戏开发者，我希望引擎在千人同屏场景下保持稳定的性能表现，以便为玩家提供流畅的大规模战斗体验。

#### 验收标准

1. WHILE 单个 Scene 内存在 1000 个活跃实体时，THE Engine SHALL 保持 Game_Loop 的 Tick 处理时间不超过帧间隔的 80%
2. THE Engine SHALL 支持将计算密集型的 System（如 AOI 计算、碰撞检测）分配到独立的 goroutine 池中并行执行
3. THE State_Sync SHALL 在千人同屏场景中通过 AOI 过滤将单个 Client 的每 Tick 同步数据量控制在 10KB 以内
4. THE Engine SHALL 使用对象池（sync.Pool）管理高频创建销毁的对象（如消息对象、事件对象）以减少 GC 压力
5. THE Engine SHALL 提供性能监控接口，实时输出每个 Tick 的处理耗时、活跃实体数量、网络吞吐量等关键指标
6. WHEN 单个 Tick 的处理耗时连续 10 帧超过帧间隔的 90% 时，THE Engine SHALL 触发性能告警事件

### 需求 13：数据持久化

**用户故事：** 作为游戏开发者，我希望引擎提供数据持久化支持，以便安全地存储和恢复游戏数据。

#### 验收标准

1. THE Engine SHALL 基于 GoFrame 的 ORM 模块提供游戏数据的持久化存储接口
2. THE Engine SHALL 支持将实体状态异步批量写入数据库以避免阻塞 Game_Loop
3. WHEN Engine 收到关闭信号时，THE Engine SHALL 将所有脏数据（已修改但未持久化的数据）写入数据库后再完成关闭
4. THE Engine SHALL 支持可配置的自动存档间隔（默认 5 分钟），定期将脏数据持久化
5. IF 数据库写入操作失败，THEN THE Engine SHALL 重试写入操作（最多 3 次），若仍失败则记录错误日志并将数据保存到本地恢复文件

### 需求 14：配置管理

**用户故事：** 作为游戏开发者，我希望引擎提供灵活的配置管理，以便通过数据驱动的方式调整游戏参数而无需修改代码。

#### 验收标准

1. THE Engine SHALL 基于 GoFrame 的配置模块支持从 YAML、JSON、TOML 格式的文件加载配置
2. THE Engine SHALL 支持游戏数值表（如技能表、怪物表、道具表）的加载和热更新
3. WHEN 配置文件发生变更时，THE Engine SHALL 在不停服的情况下重新加载变更的配置并通知相关模块
4. THE Engine SHALL 在启动时验证所有配置文件的格式和数据完整性
5. IF 配置文件验证失败，THEN THE Engine SHALL 拒绝启动并输出包含具体错误位置的错误信息

### 需求 15：日志与监控

**用户故事：** 作为运维人员，我希望引擎提供完善的日志和监控功能，以便实时掌握服务运行状态并快速定位问题。

#### 验收标准

1. THE Engine SHALL 基于 GoFrame 的日志模块提供分级日志输出（DEBUG、INFO、WARN、ERROR）
2. THE Engine SHALL 支持按模块名称过滤日志输出
3. THE Engine SHALL 提供 HTTP 接口暴露运行时指标，包括在线人数、Room 数量、平均 Tick 耗时、内存使用量
4. WHEN 发生 ERROR 级别日志事件时，THE Engine SHALL 支持通过可配置的告警通道（如 Webhook）发送告警通知
5. THE Engine SHALL 为每条日志记录附加时间戳、模块名称、协程ID等上下文信息

### 需求 16：插件与扩展机制

**用户故事：** 作为游戏开发者，我希望引擎提供插件机制，以便在不修改引擎核心代码的情况下扩展功能。

#### 验收标准

1. THE Engine SHALL 提供 Plugin 接口定义，包含初始化、启动、停止等生命周期方法
2. THE Engine SHALL 支持在启动时按配置顺序加载和初始化 Plugin
3. THE Engine SHALL 提供事件总线（Event Bus），Plugin 可通过订阅事件与引擎核心及其他 Plugin 交互
4. WHEN Plugin 初始化失败时，THE Engine SHALL 记录错误日志并根据 Plugin 配置的「是否必需」标记决定是否终止启动
5. THE Engine SHALL 提供 Hook 机制，允许 Plugin 在 Game_Loop 的特定阶段（如 Tick 前、Tick 后）注入自定义逻辑

### 需求 17：客户端接入协议与文档

**用户故事：** 作为前端开发者（如 html5-mmrpg-game 项目的开发者），我希望引擎提供完整的通信协议文档和接入规范，以便快速理解并实现客户端与服务端的通信对接。

#### 验收标准

1. THE Engine SHALL 提供完整的 Protocol_Doc，描述所有客户端与服务端之间的消息类型、字段定义、交互时序和错误码
2. THE Protocol_Doc SHALL 随 Protocol_Buffer 定义文件的变更自动生成或同步更新
3. THE Engine SHALL 定义标准的客户端接入流程文档，包括连接建立、身份认证、进入游戏、断线重连的完整时序
4. THE Engine SHALL 为所有对外暴露的消息接口提供请求和响应的示例数据
5. WHEN Protocol_Buffer 定义文件发生变更时，THE Engine SHALL 通过版本号标识协议版本，Client 可通过版本号判断兼容性

### 需求 18：客户端 SDK 与接入支持

**用户故事：** 作为前端开发者，我希望引擎提供客户端接入 SDK 或参考实现，以便降低前端系统对接引擎的开发成本。

#### 验收标准

1. THE Engine SHALL 提供 JavaScript/TypeScript 版本的 Client_SDK 参考实现，供 H5_Client（html5-mmrpg-game）直接使用或参考
2. THE Client_SDK SHALL 封装 WebSocket 连接管理、心跳维持、断线重连、消息编解码等底层通信细节
3. THE Client_SDK SHALL 提供高层 API 接口，包括登录、进入场景、移动、释放技能、聊天等常用操作
4. THE Client_SDK SHALL 提供事件回调机制，Client 可注册回调函数接收服务端推送的状态更新、战斗事件等通知
5. THE Engine SHALL 提供 Client_SDK 的接口文档和使用示例
6. WHEN Engine 的协议版本发生不兼容变更时，THE Client_SDK SHALL 在连接阶段检测版本不匹配并向 Client 报告明确的错误信息


### 需求 19：地图系统

**用户故事：** 作为游戏开发者，我希望引擎提供完整的层次化地图系统，以便构建由多个地图组成的游戏世界，支持多层地形管理、高度模拟、遮挡计算、动态加载和跨层自动寻路。

#### 验收标准

1. THE Map_Manager SHALL 支持管理多个 Map 实例，每个 Map 包含独立的地形数据、可行走区域和障碍物信息
2. THE Map_Manager SHALL 支持 Map 的动态加载和卸载，加载过程不阻塞 Game_Loop
3. WHEN Client 请求从当前 Map 传送到目标 Map 时，THE Map_Manager SHALL 验证传送条件后将 Client 对应的实体从源 Map 移除并添加到目标 Map，过程中不丢失实体状态
4. THE Map_Manager SHALL 为每个 Map 维护传送点配置，定义 Map 之间的连通关系
5. THE Pathfinding SHALL 基于 A* 或类似算法在 Map 的可行走区域上计算从起点到终点的路径
6. WHEN 请求寻路的起点或终点位于不可行走区域时，THE Pathfinding SHALL 返回寻路失败的错误信息
7. THE Map 的地形数据、可行走区域、障碍物信息和传送点 SHALL 通过配置文件定义
8. WHEN Map 配置文件发生变更时，THE Map_Manager SHALL 支持在不停服的情况下热更新 Map 数据
9. THE Map SHALL 支持多层结构，每个 Map 包含一个或多个 Map_Layer，每个 Map_Layer 代表不同的高度或空间层级（如地面层、地下层、桥梁层、山洞层等），各 Map_Layer 拥有独立的地形数据和可行走区域
10. THE Map SHALL 支持在同一 Map 的不同 Map_Layer 之间定义 Layer_Connection（如山洞入口、楼梯、传送门），实体可通过 Layer_Connection 在 Map_Layer 之间切换
11. WHEN 实体到达 Layer_Connection 位置并触发切换操作时，THE Map_Manager SHALL 将该实体从当前 Map_Layer 移除并添加到目标 Map_Layer，过程中不丢失实体状态
12. THE Map_Layer SHALL 支持 Height_Map 数据，记录每个网格单元的高度值，用于模拟高山、悬崖等地形的高度差
13. THE AOI_Manager SHALL 基于 Height_Map 和 Map_Layer 层级信息计算 Occlusion 关系，高处地形遮挡低处实体的视野时，被遮挡实体不出现在观察者的可见实体列表中
14. THE Pathfinding SHALL 支持跨 Map_Layer 的多层寻路，寻路算法在计算路径时将 Layer_Connection 作为可通行节点纳入路径搜索图
15. WHEN 跨 Map_Layer 寻路时起点和终点之间不存在可用的 Layer_Connection 路径，THE Pathfinding SHALL 返回寻路失败的错误信息
16. THE Map_Layer 的层级结构、Height_Map 数据、Layer_Connection 配置 SHALL 通过配置文件定义，与 Map 的基础地形配置统一管理
17. WHEN 实体在相邻 Map 之间移动时，THE Map_Manager SHALL 实现 Seamless_Transition，Client 无需经历加载画面即可完成地图过渡
18. THE Map_Manager SHALL 为相邻 Map 之间维护 Boundary_Zone 配置，定义两个 Map 的边界重叠区域
19. WHEN 实体接近 Map 边界（距离 Boundary_Zone 可配置阈值内）时，THE Map_Manager SHALL 触发 Boundary_Preload，将相邻 Map 的边界区域地形数据和实体数据提前推送给对应 Client
20. WHILE 实体位于 Map 的 Boundary_Zone 内时，THE AOI_Manager SHALL 执行 Cross_Boundary_AOI 计算，使该实体可以看到相邻 Map 中 Boundary_Zone 内的实体
21. WHEN 实体从 Boundary_Zone 完全进入相邻 Map 时，THE Map_Manager SHALL 将该实体的地图归属从源 Map 切换到目标 Map，切换过程中实体状态不丢失
22. WHEN 实体通过 Layer_Connection 在 Map_Layer 之间切换时，THE Map_Manager SHALL 实现 Seamless_Transition，Client 无需经历加载画面（如进入山洞不需要加载画面）
23. WHEN 实体接近 Layer_Connection 时，THE Map_Manager SHALL 触发目标 Map_Layer 的 Boundary_Preload，将目标层的地形数据和附近实体数据提前推送给对应 Client

### 需求 20：装备系统

**用户故事：** 作为游戏开发者，我希望引擎提供装备系统，以便角色可以穿戴和管理装备，装备属性影响角色的战斗能力。

#### 验收标准

1. THE Equipment_System SHALL 支持角色穿戴和卸下装备操作
2. THE Equipment_System SHALL 为角色定义多个 Equipment_Slot（包括武器、头盔、铠甲、鞋子等部位），每个 Equipment_Slot 同一时间仅允许装备一件对应类型的装备
3. THE Equipment_System SHALL 支持装备的品质属性（如普通、稀有、史诗、传说等）和等级属性
4. THE Equipment_System SHALL 支持装备的附加属性（如攻击力加成、防御力加成、暴击率加成等），附加属性通过数据配置定义
5. WHEN 角色穿戴或卸下装备时，THE Equipment_System SHALL 重新计算角色的战斗属性，计算结果反映所有已穿戴装备的属性加成总和
6. WHEN 角色尝试穿戴与 Equipment_Slot 类型不匹配的装备时，THE Equipment_System SHALL 拒绝操作并返回槽位类型不匹配的错误信息
7. IF 目标 Equipment_Slot 已有装备且角色请求穿戴新装备，THEN THE Equipment_System SHALL 自动卸下旧装备并穿戴新装备，确保属性正确重算

### 需求 21：战斗装备锁定

**用户故事：** 作为游戏开发者，我希望战斗期间锁定参战角色的装备状态，以便保证战斗过程中角色属性的稳定性和公平性。

#### 验收标准

1. WHEN 战斗开始时，THE Combat_System SHALL 将所有参战角色的 Equipment_Lock 状态设置为锁定
2. WHILE Equipment_Lock 状态为锁定期间，THE Equipment_System SHALL 拒绝该角色的所有装备穿戴和卸下操作
3. WHEN 战斗结束时，THE Combat_System SHALL 解除所有参战角色的 Equipment_Lock 状态
4. IF Client 在 Equipment_Lock 状态为锁定期间发送装备切换请求，THEN THE Engine SHALL 拒绝该请求并返回「战斗中禁止切换装备」的错误信息
5. WHEN 角色因异常退出战斗（如断线）时，THE Combat_System SHALL 在该角色的战斗结算完成后解除其 Equipment_Lock 状态

### 需求 22：战斗数据内存化

**用户故事：** 作为游戏开发者，我希望战斗期间所有计算基于内存中的战斗上下文执行，以便支持千人同屏场景下的高性能战斗计算。

#### 验收标准

1. WHEN 战斗开始时，THE Combat_System SHALL 将所有参战实体的战斗相关数据（属性、技能、Buff 等）从持久化存储提取并构建 Combat_Context 实例存储在内存中
2. WHILE 战斗进行期间，THE Combat_System SHALL 基于 Combat_Context 中的内存数据执行所有战斗计算，不访问数据库或其他持久化存储
3. WHEN 战斗结束时，THE Combat_System SHALL 将 Combat_Context 中的战斗结果（经验值、掉落物、属性变更等）写回持久化存储
4. THE Combat_Context SHALL 包含参战实体的完整战斗属性快照，使战斗计算不依赖外部数据源
5. IF Combat_Context 构建过程中持久化存储访问失败，THEN THE Combat_System SHALL 取消战斗初始化并向相关 Client 返回战斗启动失败的错误信息
6. WHEN 战斗结果写回持久化存储失败时，THE Combat_System SHALL 将 Combat_Context 数据保存到本地恢复文件并记录错误日志，防止战斗数据丢失
