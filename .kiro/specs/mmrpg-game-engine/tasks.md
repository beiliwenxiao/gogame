# 实现计划：MMRPG 游戏引擎 (gogame)

## 概述

构建 MMRPG 纯游戏后端引擎，采用 ECS 架构，支持帧同步与状态同步双模式，核心目标为千人同屏战斗。实现顺序从基础设施层开始，逐步构建核心引擎层、游戏逻辑层和扩展层，最后完成集成与联调。

## 任务

- [x] 1. 项目结构与基础设施搭建
  - [x] 1.1 初始化项目结构与 Go 模块
    - 创建项目目录结构：`internal/engine/`, `internal/network/`, `internal/ecs/`, `internal/sync/`, `internal/scene/`, `internal/combat/`, `internal/equipment/`, `internal/map/`, `internal/aoi/`, `internal/room/`, `internal/plugin/`, `internal/persistence/`, `internal/config/`, `internal/monitor/`, `internal/codec/`, `proto/`, `sdk/`
    - 初始化 Go module，添加 GoFrame v2、gorilla/websocket、protobuf 等依赖
    - 定义核心类型文件 `internal/engine/types.go`，包含 `EntityID`、`ComponentType`、`Vector3`、`TransportProtocol` 等基础类型
    - _需求: 全局_

  - [x] 1.2 实现配置管理模块（ConfigManager）
    - 在 `internal/config/` 下实现 `ConfigManager` 接口
    - 基于 GoFrame gcfg 模块支持 YAML/JSON/TOML 格式配置加载
    - 实现 `GameTable` 接口，支持游戏数值表（技能表、怪物表、装备表等）的加载
    - 实现配置热更新：监听文件变更，不停服重新加载配置并通知回调
    - 实现启动时配置验证，格式或数据不合法时拒绝启动并输出错误位置
    - _需求: 14.1, 14.2, 14.3, 14.4, 14.5_

  - [x] 1.3 实现日志与监控模块（Monitor）
    - 在 `internal/monitor/` 下实现 `Monitor` 接口
    - 基于 GoFrame glog 模块提供分级日志（DEBUG/INFO/WARN/ERROR），支持按模块名称过滤
    - 每条日志附加时间戳、模块名称、协程 ID 等上下文信息
    - 实现 `Metrics` 结构体，记录在线人数、Room 数量、平均 Tick 耗时、内存使用量、网络吞吐量
    - 提供 HTTP 接口暴露运行时指标
    - 实现 ERROR 级别日志的 Webhook 告警通知
    - _需求: 15.1, 15.2, 15.3, 15.4, 15.5_

  - [x] 1.4 实现数据持久化模块（PersistenceManager）
    - 在 `internal/persistence/` 下实现 `PersistenceManager` 接口
    - 基于 GoFrame ORM 模块实现异步批量写入，避免阻塞 Game_Loop
    - 实现可配置的自动存档间隔（默认 5 分钟）
    - 实现关闭信号处理：刷写所有脏数据后再完成关闭
    - 实现写入失败重试（最多 3 次），失败后保存到本地恢复文件
    - 定义数据库模型：`DBCharacter`、`DBEquipment`、`DBCombatLog`
    - _需求: 13.1, 13.2, 13.3, 13.4, 13.5_

  - [x] 1.5 实现对象池
    - 在 `internal/engine/` 下基于 `sync.Pool` 实现高频对象（消息对象、事件对象等）的对象池
    - _需求: 12.4_

- [x] 2. 检查点 - 基础设施模块验证
  - 确保所有测试通过，如有疑问请向用户确认。

- [x] 3. 网络层与消息编解码
  - [x] 3.1 实现网络层（NetworkLayer）
    - 在 `internal/network/` 下实现 `Session` 接口和 `NetworkLayer` 接口
    - 基于 GoFrame gtcp 实现 TCP 长连接服务
    - 基于 gorilla/websocket 实现 WebSocket 连接端点（支持 ws 和 wss）
    - 实现统一会话抽象，上层业务无需感知底层传输协议差异
    - 实现连接握手与唯一会话标识分配
    - 实现心跳检测机制（间隔和超时可配置）
    - 实现断连检测（3 秒内检测）与断连事件通知
    - 实现连接异常日志记录与会话资源清理
    - 支持最大连接数配置（默认 5000）
    - _需求: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9_

  - [x] 3.2 实现消息编解码器（MessageCodec）
    - 在 `internal/codec/` 下实现 `MessageCodec` 接口
    - 定义 Protocol Buffer 消息文件（`proto/` 目录），包含所有核心消息类型
    - 实现 Protobuf 二进制格式编解码
    - 实现 JSON 文本格式编解码（供 H5_Client 调试使用）
    - 实现消息拆包：按「4字节消息长度 + 2字节消息ID + 消息体」格式解析
    - 实现消息格式化输出功能（调试用可读文本）
    - 实现非法消息丢弃与错误日志记录（包含来源 Client 标识）
    - _需求: 2.1, 2.2, 2.3, 2.5, 2.6, 2.7_

  - [ ]* 3.3 编写消息编解码往返一致性属性测试
    - **属性 P1：消息编解码往返一致性**
    - 随机生成各类型 Message 对象，验证 `Decode(Encode(M)) ≡ M`
    - 同时验证 Protobuf 和 JSON 两种编解码格式
    - **验证: 需求 2.4**

  - [x] 3.4 实现紧凑编解码器（CompactCodec）
    - 在 `internal/codec/` 下实现 `CompactCodec`、`ShortIDMapper`、`OperationDictionary` 接口
    - 实现 `OperationDictionary`：注册移动(u/d/l/r)、攻击(a)、技能(s)、交互(i)、聊天(c) 等操作码映射
    - 实现 `ShortIDMapper`：长实体 ID 到短字符标识的映射与释放
    - 实现 `Encode`/`Decode` 方法：将操作序列编码为紧凑字符串（如 "Au10aB"）并解码还原
    - 实现 `EncodeBatch`：同一 Tick 内同一实体多个操作合并编码
    - 实现 `Format` 格式化输出功能（调试用可读文本）
    - 实现非法 Compact_Message 丢弃与错误日志记录
    - _需求: 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 2.14, 2.15_

  - [ ]* 3.5 编写紧凑编解码往返一致性属性测试
    - **属性 P14：紧凑编解码往返一致性**
    - 随机生成各类型 CompactOperation 序列（移动、攻击、技能等组合），验证 `Decode(Encode(Ops)) ≡ Ops`
    - 验证批量操作合并编码的往返一致性
    - **验证: 需求 2.13**


- [x] 4. 检查点 - 网络层与编解码验证
  - 确保所有测试通过，如有疑问请向用户确认。

- [x] 5. ECS 核心与游戏主循环
  - [x] 5.1 实现 ECS 实体管理器（EntityManager）
    - 在 `internal/ecs/` 下实现 `Component`、`System`、`World`、`EntityManager` 接口
    - 实现全局唯一 EntityID 分配
    - 实现 Component 的动态添加和移除
    - 实现 System 注册与按优先级顺序执行
    - 实现基于 Component 类型组合的高效实体查询（`Query` 方法）
    - 实现实体销毁：移除所有 Component 并从所有 System 处理列表中清除
    - 定义核心 Component 类型：`PositionComponent`、`MovementComponent`、`CombatAttributeComponent`、`EquipmentComponent`、`SkillComponent`、`BuffComponent`、`NetworkComponent`、`AOIComponent`
    - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6_

  - [ ]* 5.2 编写实体 ID 全局唯一性属性测试
    - **属性 P2：实体 ID 全局唯一性**
    - 在单个 World 中批量创建大量实体，验证所有 EntityID 无重复
    - **验证: 需求 7.1**

  - [ ]* 5.3 编写 ECS 组件完整性属性测试
    - **属性 P3：ECS 组件完整性**
    - 创建实体并添加多种 Component，销毁后验证所有 Component 查询返回空，所有 System 查询不包含该实体
    - **验证: 需求 7.6**

  - [x] 5.4 实现游戏主循环（GameLoop）
    - 在 `internal/engine/` 下实现 `GameLoop` 接口
    - 实现可配置的固定频率 Tick（默认 20 Tick/秒）
    - 实现单调递增的逻辑帧号计数器
    - 实现四阶段 Tick 执行：输入处理 → 逻辑更新 → 状态同步 → 清理
    - 实现 `RegisterPhase` 方法，支持注册各阶段回调
    - 实现 Tick 超时检测：处理时间超过帧间隔时记录性能警告并跳帧追赶
    - 实现安全停止：收到关闭信号后完成当前 Tick 并持久化游戏状态
    - _需求: 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 6. 同步管理器
  - [x] 6.1 实现帧同步器（LockstepSyncer）
    - 在 `internal/sync/` 下实现 `Syncer` 和 `LockstepSyncer` 接口
    - 实现同一逻辑帧内所有 Client 操作指令的收集与打包
    - 实现帧数据包广播，保证所有 Client 收到内容和顺序一致
    - 实现等待超时机制（可配置，默认 100ms），超时后用空操作填充
    - 实现历史帧缓存（可配置，默认 300 帧）
    - 实现断线重连追帧：按顺序发送缺失的历史帧数据
    - _需求: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_

  - [ ]* 6.2 编写帧同步一致性属性测试
    - **属性 P4：帧同步一致性**
    - 模拟多个 Client 在同一 Room 中发送随机操作指令，验证所有 Client 收到的帧数据包序列一致
    - **验证: 需求 4.3**

  - [x] 6.3 实现状态同步器（StateSyncer）
    - 在 `internal/sync/` 下实现 `StateSyncer` 接口
    - 实现 Engine 端权威游戏逻辑运行
    - 实现 Delta 计算：每 Tick 计算与上一帧的状态差异
    - 实现仅下发 Delta 数据（非完整快照）以减少带宽
    - 实现定期完整状态快照（可配置，默认 100 帧间隔）
    - 实现断线重连：发送最近快照 + 后续 Delta 恢复状态
    - 结合 AOI 结果仅同步兴趣区域内的实体状态
    - _需求: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

  - [ ]* 6.4 编写状态同步 Delta 正确性属性测试
    - **属性 P5：状态同步 Delta 正确性**
    - 在随机实体状态变更场景下，验证 `ApplyDelta(State(N), Delta(N+1)) ≡ State(N+1)`
    - **验证: 需求 5.2, 5.3**

  - [x] 6.5 实现同步管理器（SyncManager）
    - 在 `internal/sync/` 下实现 `SyncManager`
    - 支持按 Room 粒度独立配置同步模式
    - 创建 Room 时根据配置初始化对应同步模块实例
    - 提供统一同步接口抽象，上层无需感知具体同步模式
    - Client 数量超过阈值（默认 50 人）时记录建议切换为 State_Sync 的警告日志
    - _需求: 6.1, 6.2, 6.3, 6.4_

- [x] 7. 检查点 - ECS 与同步模块验证
  - 确保所有测试通过，如有疑问请向用户确认。

- [ ] 8. AOI 系统
  - [x] 8.1 实现空间索引（SpatialIndex）
    - 在 `internal/aoi/` 下实现 `SpatialIndex` 接口
    - 实现九宫格或四叉树空间索引结构
    - 实现 `Insert`、`Remove`、`Update`、`QueryRange` 方法
    - _需求: 9.1_

  - [x] 8.2 实现 AOI 管理器（AOIManager）
    - 在 `internal/aoi/` 下实现 `AOIManager` 接口
    - 基于 SpatialIndex 维护每个实体的可见实体列表
    - 实现位置变化时的空间索引更新与可见列表重算
    - 实现「进入视野」和「离开视野」事件回调
    - 支持可配置的 AOI 半径，不同实体类型可设置不同范围
    - 千人同屏场景下单 Tick 内完成所有 AOI 更新计算
    - _需求: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7_

  - [ ]* 8.3 编写 AOI 对称性属性测试
    - **属性 P9：AOI 对称性**
    - 随机移动大量实体，验证可见列表的对称性和进入/离开事件的配对完整性
    - **验证: 需求 9.4, 9.5**

  - [x] 8.4 实现跨边界 AOI（Cross_Boundary_AOI）
    - 在 `AOIManager` 中实现 `RegisterNeighborAOI`、`GetCrossBoundaryVisibleEntities`、`GetBoundaryEntities` 方法
    - 实现 `AOIBoundaryZone` 和 `CrossBoundaryEntity` 数据结构
    - 实体在 Boundary_Zone 内时可看到相邻 AOI_Manager 中的实体
    - _需求: 8.9, 11.9, 19.20_


- [ ] 9. 场景管理
  - [x] 9.1 实现场景管理器（SceneManager）
    - 在 `internal/scene/` 下实现 `Scene`、`SceneBoundaryZone`、`SceneConfig`、`SceneManager` 接口
    - 实现 Scene 的动态创建、加载和卸载
    - 每个 Scene 维护独立的 EntityManager 和 AOIManager 实例
    - 实现准入条件验证后将实体添加到目标 Scene
    - 实现实体跨 Scene 切换：从源 Scene 移除并添加到目标 Scene，不丢失状态
    - 实现空闲 Scene 自动卸载（可配置，默认 5 分钟无活跃 Client）
    - _需求: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [ ]* 9.2 编写场景切换状态保持属性测试
    - **属性 P10：场景切换状态保持**
    - 为实体添加随机 Component 数据，执行场景切换，验证切换后所有 Component 数据与切换前完全一致
    - **验证: 需求 8.4**

  - [x] 9.3 实现无缝转场（Seamless_Transition for Scene）
    - 实现 `ConfigureBoundaryZone`、`GetBoundaryZone`、`IsInBoundaryZone` 方法
    - 实现 `TriggerBoundaryPreload`：实体接近边界时预加载相邻 Scene 数据
    - 实现 `SeamlessTransfer`：实体从 Boundary_Zone 完全进入目标 Scene 时无缝切换归属
    - 确保 Boundary_Zone 内实体同时对源和目标 Scene 的 AOI 可见
    - _需求: 8.7, 8.8, 8.9, 8.10, 8.11_

- [ ] 10. 地图系统
  - [x] 10.1 实现地图管理器（MapManager）基础功能
    - 在 `internal/map/` 下实现 `Map`、`MapLayer`、`LayerConnection`、`TeleportPoint`、`MapManager` 接口
    - 实现 Map 的动态加载和卸载（不阻塞 Game_Loop）
    - 实现多层地图结构：每个 Map 包含多个 MapLayer，各层独立地形数据和可行走区域
    - 实现 HeightMap 数据支持
    - 实现传送点配置与跨 Map 传送
    - 实现 LayerConnection 定义与层间切换
    - 实现地图配置文件加载与热更新
    - _需求: 19.1, 19.2, 19.3, 19.4, 19.7, 19.8, 19.9, 19.10, 19.11, 19.12, 19.16_

  - [x] 10.2 实现寻路模块（Pathfinder）
    - 在 `internal/map/` 下实现 `Pathfinder` 接口
    - 基于 A* 算法在可行走区域上计算路径
    - 实现跨 MapLayer 的多层寻路：将 LayerConnection 作为可通行节点纳入路径搜索图
    - 起点或终点在不可行走区域时返回寻路失败错误
    - 跨层无可用 LayerConnection 路径时返回寻路失败错误
    - _需求: 19.5, 19.6, 19.14, 19.15_

  - [ ]* 10.3 编写寻路可达性一致性属性测试
    - **属性 P11：寻路可达性一致性**
    - 在随机生成的多层地图上执行寻路，验证路径每个节点可行走、步长合理、跨层经过有效连接点
    - **验证: 需求 19.5, 19.14**

  - [x] 10.4 实现基于 HeightMap 的遮挡计算
    - 在 AOIManager 中集成 HeightMap 和 MapLayer 层级信息
    - 实现 Occlusion 计算：高处地形遮挡低处实体视野时，被遮挡实体不出现在可见列表中
    - _需求: 19.13_

  - [x] 10.5 实现无缝地图转场与层切换
    - 实现 `ConfigureMapBoundary`、`GetMapBoundary`、`CheckBoundaryProximity` 方法
    - 实现 `TriggerMapBoundaryPreload`：实体接近地图边界时预加载相邻 Map 数据
    - 实现 `SeamlessMapTransfer`：实体从 Boundary_Zone 完全进入相邻 Map 时无缝切换
    - 实现 `SeamlessLayerSwitch`：实体通过 LayerConnection 切换层，无需加载画面
    - 实现 `TriggerLayerPreload`：实体接近 LayerConnection 时预加载目标层数据
    - _需求: 19.17, 19.18, 19.19, 19.20, 19.21, 19.22, 19.23_

  - [ ]* 10.6 编写无缝转场状态一致性属性测试
    - **属性 P13：无缝转场状态一致性**
    - 执行跨 Map、跨 MapLayer 的无缝转场，验证：(1) Component 数据一致；(2) Session 连接未中断；(3) Boundary_Zone 内实体同时出现在两侧 AOI 可见列表中
    - **验证: 需求 19.17, 19.21, 19.22**

- [x] 11. 检查点 - AOI、场景与地图模块验证
  - 确保所有测试通过，如有疑问请向用户确认。

- [ ] 12. 房间管理
  - [x] 12.1 实现房间管理器（RoomManager）基础功能
    - 在 `internal/room/` 下实现 `Room`、`RoomConfig`、`RoomManager` 接口
    - 实现 Room 的动态创建和销毁
    - 每个 Room 分配独立的 GameLoop 和 Syncer 实例
    - 实现 Client 加入/离开 Room：验证容量限制和准入条件
    - 实现 Room 容量上限配置（State_Sync 模式下不低于 1000 人）
    - Room 已满时拒绝加入并返回错误信息
    - 实现空闲 Room 自动销毁（可配置，默认 30 秒无 Client）
    - _需求: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6_

  - [ ]* 12.2 编写 Room 容量不变量属性测试
    - **属性 P12：Room 容量不变量**
    - 并发模拟大量 Client 加入同一 Room，验证 Client 数量不超过容量上限
    - **验证: 需求 11.5, 11.6**

  - [x] 12.3 实现无缝房间切换与实体迁移
    - 实现 `ConfigureRoomBoundary`、`GetRoomBoundary` 方法
    - 实现 `SeamlessTransfer`：Client 无需断开连接即可完成 Room 切换
    - 实现 `MigrateEntity`：实体从源 Room 平滑迁移到目标 Room
    - 实现 `EntityMigrationState` 状态机：准备 → 传输 → 确认 → 清理 → 完成
    - 采用「先加后删」策略：目标 Room 确认接收后再从源 Room 移除
    - 迁移期间保证实体状态完整保留且 Client 操作不中断
    - 实现 `GetBoundaryEntities` 供 Cross_Boundary_AOI 使用
    - _需求: 11.7, 11.8, 11.9, 11.10, 11.11, 11.12_


- [ ] 13. 装备系统
  - [x] 13.1 实现装备系统（EquipmentSystem）
    - 在 `internal/equipment/` 下实现 `EquipmentSystem`、`EquipmentItem`、`EquipmentSlotType`、`EquipmentQuality` 接口和类型
    - 实现装备穿戴和卸下操作
    - 实现多 Equipment_Slot 管理（武器、头盔、铠甲、鞋子、项链、戒指），每个槽位同一时间仅允许一件装备
    - 实现装备品质（普通/稀有/史诗/传说）和等级属性
    - 实现装备附加属性（攻击力、防御力、暴击率等加成），通过数据配置定义
    - 实现 `CalculateAttributes`：重新计算角色战斗属性 = 基础属性 + 所有已穿戴装备属性加成总和
    - 实现槽位类型不匹配时拒绝操作并返回错误
    - 实现已有装备时自动卸下旧装备并穿戴新装备，确保属性正确重算
    - 实现 `SetLock`/`IsLocked`：装备锁定状态管理
    - _需求: 20.1, 20.2, 20.3, 20.4, 20.5, 20.6, 20.7_

  - [ ]* 13.2 编写装备属性计算一致性属性测试
    - **属性 P8：装备属性计算一致性**
    - 随机穿戴/卸下装备序列后，验证最终属性与从零重算的结果一致
    - **验证: 需求 20.5**

- [ ] 14. 战斗系统
  - [x] 14.1 实现战斗系统（CombatSystem）
    - 在 `internal/combat/` 下实现 `CombatSystem`、`CombatContext`、`CombatEntity` 接口和类型
    - 实现 `StartCombat`：从持久化存储提取参战实体数据，构建 Combat_Context 内存实例，设置 Equipment_Lock 锁定
    - 实现 `EndCombat`：解除 Equipment_Lock，写回战斗结果到持久化存储，销毁 Combat_Context
    - 实现技能定义框架：通过数据配置定义技能的释放条件、作用范围、伤害公式和效果
    - 实现基于 AOI 的目标选取：单体、扇形、圆形、矩形模式
    - 实现技能流程：前摇 → 判定 → 结算 → 后摇
    - 实现 Buff/Debuff 系统：效果叠加、持续时间管理、效果移除
    - 实现战斗日志事件生成（攻击者、受击者、技能ID、伤害值、暴击标记）
    - 实现技能中断策略：释放者死亡或被控制时根据配置取消或继续后续阶段
    - _需求: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6_

  - [x] 14.2 实现战斗装备锁定逻辑
    - 战斗开始时设置所有参战角色 Equipment_Lock = 锁定
    - 锁定期间 EquipmentSystem 拒绝所有装备操作
    - 战斗结束时解除 Equipment_Lock
    - 锁定期间收到装备切换请求返回「战斗中禁止切换装备」错误
    - 异常退出战斗（断线）时在战斗结算完成后解除锁定
    - _需求: 21.1, 21.2, 21.3, 21.4, 21.5_

  - [ ]* 14.3 编写装备锁定不变量属性测试
    - **属性 P6：装备锁定不变量**
    - 在战斗进行中随机发送装备操作请求，验证全部被拒绝且装备状态未改变
    - **验证: 需求 21.2**

  - [x] 14.4 实现战斗数据内存化
    - 战斗开始时从持久化存储提取数据构建 Combat_Context
    - 战斗期间所有计算基于 Combat_Context 内存数据，不访问数据库
    - 战斗结束时写回结果到持久化存储
    - Combat_Context 包含完整战斗属性快照
    - 持久化存储访问失败时取消战斗初始化并返回错误
    - 写回失败时保存到本地恢复文件并记录错误日志
    - _需求: 22.1, 22.2, 22.3, 22.4, 22.5, 22.6_

  - [ ]* 14.5 编写战斗数据内存化隔离性属性测试
    - **属性 P7：战斗数据内存化隔离性**
    - 在战斗进行中修改 Combat_Context 数据，验证数据库数据未变；修改数据库数据，验证 Combat_Context 未受影响
    - **验证: 需求 22.2**

- [x] 15. 检查点 - 装备与战斗系统验证
  - 确保所有测试通过，如有疑问请向用户确认。

- [ ] 16. 插件与扩展机制
  - [x] 16.1 实现插件系统与事件总线
    - 在 `internal/plugin/` 下实现 `Plugin`、`EventBus`、`Event`、`Hook` 接口
    - 实现 Plugin 生命周期管理：初始化、启动、停止
    - 实现启动时按配置顺序加载和初始化 Plugin
    - 实现事件总线：Plugin 通过订阅事件与引擎核心及其他 Plugin 交互
    - Plugin 初始化失败时根据「是否必需」标记决定是否终止启动
    - 实现 Hook 机制：允许 Plugin 在 Game_Loop 特定阶段注入自定义逻辑
    - _需求: 16.1, 16.2, 16.3, 16.4, 16.5_

- [ ] 17. 千人同屏性能优化
  - [x] 17.1 实现并行计算与性能监控
    - 实现计算密集型 System（AOI 计算、碰撞检测）分配到独立 goroutine 池并行执行
    - 实现 State_Sync 千人同屏场景下通过 AOI 过滤将单 Client 每 Tick 同步数据量控制在 10KB 以内
    - 实现性能监控接口：实时输出每 Tick 处理耗时、活跃实体数量、网络吞吐量
    - 实现性能告警：连续 10 帧 Tick 耗时超过帧间隔 90% 时触发告警事件
    - 确保单 Scene 1000 活跃实体时 Tick 处理时间不超过帧间隔 80%
    - _需求: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6_

- [ ] 18. 协议文档与客户端 SDK
  - [x] 18.1 生成协议文档（Protocol_Doc）
    - 编写完整的 Protocol_Doc，描述所有消息类型、字段定义、交互时序和错误码
    - 定义标准客户端接入流程文档：连接建立、身份认证、进入游戏、断线重连的完整时序
    - 为所有对外消息接口提供请求和响应示例数据
    - 实现协议版本号标识，Client 可通过版本号判断兼容性
    - 配置 Protocol_Doc 随 .proto 文件变更自动生成或同步更新
    - _需求: 17.1, 17.2, 17.3, 17.4, 17.5_

  - [x] 18.2 实现 JavaScript/TypeScript 客户端 SDK
    - 在 `sdk/` 目录下创建 JS/TS 版本的 Client_SDK 参考实现
    - 封装 WebSocket 连接管理、心跳维持、断线重连、消息编解码
    - 提供高层 API：登录、进入场景、移动、释放技能、聊天等
    - 提供事件回调机制：注册回调接收服务端推送的状态更新、战斗事件等
    - 实现连接阶段协议版本检测，版本不匹配时报告明确错误
    - 编写 Client_SDK 接口文档和使用示例
    - _需求: 18.1, 18.2, 18.3, 18.4, 18.5, 18.6_

- [ ] 19. 引擎集成与整体联调
  - [x] 19.1 实现 Engine 主入口与模块组装
    - 创建 `internal/engine/engine.go`，定义 `Engine` 结构体
    - 按依赖顺序初始化所有模块：Config → Logger → Persistence → Network → Codec → ECS → AOI → Scene → Map → Room → Sync → Combat → Equipment → Plugin → Monitor
    - 实现 Engine 的 Start/Stop 生命周期
    - 将 NetworkLayer 的消息回调连接到 GameLoop 的输入处理阶段
    - 将 GameLoop 的各阶段回调连接到对应的 System 和 Manager
    - _需求: 全局_

  - [ ]* 19.2 编写集成测试
    - 测试完整的 Client 连接 → 登录 → 进入场景 → 移动 → 释放技能 → 断线重连流程
    - 测试帧同步和状态同步两种模式下的基本交互
    - 测试跨 Scene、跨 Map、跨 Room 的无缝转场
    - _需求: 全局_

- [x] 20. 最终检查点 - 全部模块集成验证
  - 确保所有测试通过，如有疑问请向用户确认。

## 备注

- 标记 `*` 的任务为可选任务，可跳过以加速 MVP 开发
- 每个任务引用了具体的需求编号以确保可追溯性
- 检查点任务确保增量验证
- 属性测试验证核心正确性不变量（P1-P14）
- 单元测试验证具体示例和边界情况
- 实现语言为 Go 1.21+，框架为 GoFrame v2
