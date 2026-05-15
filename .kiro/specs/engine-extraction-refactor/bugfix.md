# Bugfix Requirements Document

## Introduction

Demo（修罗斗场）中包含了大量本应属于引擎层的通用功能代码，导致功能与 demo 强耦合，无法被其他场景/项目复用。这是一个架构级缺陷：引擎层（后端 `internal/`、前端 `src/core/`+`src/systems/`+`src/prologue/`）缺少必要的通用抽象，迫使 demo 层自行实现这些功能。目标是将 demo 中的通用功能提取到引擎的父类或公用方法中，让 demo 只负责提供参数和调用功能。

**影响范围：**
- 后端：`cmd/demo/arena.go`、`cmd/demo/handlers.go`、`cmd/demo/main.go` 中的通用战斗计算、状态同步、NPC AI、消息路由等逻辑
- 前端：`ArenaScene.js` 中的联网状态管理、远程玩家同步、安全区判定、篝火系统、地面掉落物、灵魂状态等逻辑

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN 后端 demo 需要距离计算、椭圆范围判定、扇形范围判定时 THEN `cmd/demo/arena.go` 中重复实现了 `distance()`、`isInEllipseRange()`、`isInFanRange()` 函数，与 `internal/combat/range.go` 中已有的 `Distance()`、`IsInEllipseRange()`、`IsInFanRange()` 功能完全重复

1.2 WHEN 后端 demo 需要伤害计算时 THEN `cmd/demo/arena.go` 中重复实现了 `calcDamage()` 函数，与 `internal/combat/range.go` 中已有的 `CalcDamage()` 功能完全重复

1.3 WHEN 后端 demo 需要 WebSocket 消息路由和会话管理时 THEN `cmd/demo/main.go` 中自行实现了完整的 WebSocket 升级、消息解析、会话管理（`PlayerSession`）、消息路由（`handleMessage` switch）逻辑，未复用 `internal/network/` 的 `NetworkLayer` 和 `Session` 接口

1.4 WHEN 后端 demo 需要竞技场房间管理（玩家加入/离开/广播）时 THEN `cmd/demo/main.go` 中自行实现了 `Arena` 结构体（含 `players` map、`Broadcast`、`BroadcastAll`），未复用 `internal/room/` 的 `RoomManager`

1.5 WHEN 后端 demo 需要增量状态同步时 THEN `cmd/demo/arena.go` 中自行实现了 `doStateSync()` 增量快照对比逻辑（`playerSnapshot`、逐字段 diff），未复用 `internal/sync/` 的 `StateSyncer`

1.6 WHEN 后端 demo 需要 NPC AI（追踪、攻击、脱战回归、恐惧逃跑）时 THEN `cmd/demo/arena.go` 中自行实现了完整的 `npcAITick()` 逻辑（约 350 行），这些是通用 MMRPG NPC AI 行为，未抽象到引擎层

1.7 WHEN 后端 demo 需要安全区判定时 THEN `cmd/demo/arena.go` 中自行实现了 `isInSafeZone()` 函数，这是通用的区域判定功能，未抽象到引擎层

1.8 WHEN 后端 demo 需要定时 tick 循环（状态同步、NPC 刷新、NPC AI）时 THEN `cmd/demo/arena.go` 中自行实现了 `arenaTick()` 多 ticker 循环，未复用 `internal/engine/` 的 `GameLoop` 机制

1.9 WHEN 前端 ArenaScene 需要管理远程玩家实体（创建、更新、插值、删除）时 THEN `ArenaScene.js` 中自行实现了远程玩家管理逻辑（`remotePlayers` Map、`addRemotePlayer`、`removeRemotePlayer`、`updateSelfFromServer`），未抽象到 `MultiplayerManager` 或 `BaseGameScene` 中作为通用联网场景功能

1.10 WHEN 前端 ArenaScene 需要处理玩家死亡/复活的灵魂状态时 THEN `ArenaScene.js` 中自行实现了完整的灵魂状态管理（`_showSoulOverlay`、`_hideSoulOverlay`、双标志同步、死亡清理清单），这些是通用联网场景的死亡/复活处理，未抽象到引擎层

1.11 WHEN 前端 ArenaScene 需要处理增量状态同步时 THEN `ArenaScene.js` 中自行实现了 `onStateSync()` 方法（约 50 行），包含玩家和 NPC 的属性更新、双标志同步等逻辑，未抽象为通用的联网状态同步处理器

1.12 WHEN 前端 ArenaScene 需要篝火/安全区系统时 THEN `ArenaScene.js` 中自行实现了篝火渲染（`renderCampfire` 约 120 行）、篝火粒子（`initCampfireParticles` 约 90 行）、安全区判定（`isInSafeZone`），这些是通用的场景兴趣点/安全区功能，未抽象到引擎层

1.13 WHEN 前端 ArenaScene 需要地面掉落物系统时 THEN `ArenaScene.js` 的 `update()` 中内联了掉落物拾取逻辑（约 40 行），虽然已有 `GroundDropPickupSystem` 和 `GroundDropRenderer`，但拾取触发和批量处理逻辑仍在 demo 场景中

1.14 WHEN 前端 ArenaScene 需要恐惧/昏迷等状态效果的移动控制时 THEN `ArenaScene.js` 的 `update()` 中内联了恐惧自动移动逻辑（约 15 行），这是通用的状态效果驱动移动，未抽象到 `StatusEffectSystem` 或 `MovementSystem` 中

1.15 WHEN 前端 demo 需要 WebSocket 通信时 THEN `cmd/demo/static/js/ws.js` 自行实现了 WebSocket 封装类，`cmd/demo/static/js/app.js` 中自行实现了消息注册和路由，未复用 `src/core/MultiplayerManager.js` 的通信能力

### Expected Behavior (Correct)

2.1 WHEN 后端 demo 需要距离计算、椭圆范围判定、扇形范围判定时 THEN demo SHALL 直接调用 `internal/combat/range.go` 中已有的 `combat.Distance()`、`combat.IsInEllipseRange()`、`combat.IsInFanRange()` 函数，不再重复实现

2.2 WHEN 后端 demo 需要伤害计算时 THEN demo SHALL 直接调用 `internal/combat/range.go` 中已有的 `combat.CalcDamage()` 函数，不再重复实现

2.3 WHEN 后端 demo 需要 WebSocket 消息路由和会话管理时 THEN demo SHALL 复用 `internal/network/` 的 `NetworkLayer` 接口进行连接管理，或将通用的消息解析/路由模式抽象到引擎层，demo 只注册具体的消息处理函数

2.4 WHEN 后端 demo 需要竞技场房间管理时 THEN demo SHALL 复用 `internal/room/` 的 `RoomManager` 进行玩家加入/离开/广播管理，或将 `Arena` 的通用广播/玩家管理能力提取到引擎层的房间基类中

2.5 WHEN 后端 demo 需要增量状态同步时 THEN demo SHALL 复用 `internal/sync/` 的 `StateSyncer` 进行增量状态同步，或将 `doStateSync()` 的增量快照对比模式抽象为引擎层的通用同步工具

2.6 WHEN 后端 demo 需要 NPC AI 时 THEN 引擎层 SHALL 提供通用的 NPC AI 框架（追踪、攻击、脱战回归、状态效果响应），demo 只需提供 NPC 模板参数和自定义行为回调

2.7 WHEN 后端 demo 需要安全区判定时 THEN 引擎层 SHALL 提供通用的区域判定工具（支持椭圆/圆形/矩形区域），demo 只需配置区域参数

2.8 WHEN 后端 demo 需要定时 tick 循环时 THEN demo SHALL 复用 `internal/engine/` 的 `GameLoop` 机制注册各阶段处理器，不再自行管理多个 ticker

2.9 WHEN 前端联网场景需要管理远程玩家实体时 THEN `BaseGameScene` 或 `MultiplayerManager` SHALL 提供通用的远程实体管理方法（创建、更新、插值、删除），demo 场景只需调用这些方法并传入服务端数据

2.10 WHEN 前端联网场景需要处理玩家死亡/复活时 THEN 引擎层 SHALL 提供通用的联网死亡/复活处理（灵魂状态 overlay、双标志同步、死亡清理），demo 场景只需调用并可选覆写特定行为

2.11 WHEN 前端联网场景需要处理增量状态同步时 THEN 引擎层 SHALL 提供通用的状态同步处理器（属性更新、双标志同步、实体创建/删除），demo 场景只需注册同步回调

2.12 WHEN 前端场景需要篝火/安全区系统时 THEN 引擎层 SHALL 提供通用的场景兴趣点渲染器和安全区判定系统，demo 场景只需配置位置、半径等参数

2.13 WHEN 前端场景需要地面掉落物拾取时 THEN `GroundDropPickupSystem` SHALL 封装完整的拾取触发逻辑（按键检测、范围判定、批量拾取、浮动文字），demo 场景只需调用 `system.update(deltaTime)` 即可

2.14 WHEN 前端联网场景需要状态效果驱动移动时 THEN `StatusEffectSystem` 或 `MovementSystem` SHALL 提供通用的状态效果移动控制（恐惧逃跑、昏迷禁止移动），demo 场景只需设置状态参数

2.15 WHEN 前端 demo 需要 WebSocket 通信时 THEN demo SHALL 复用 `src/core/MultiplayerManager.js` 的通信能力，或将 `ws.js` 的通用 WebSocket 封装提取到引擎核心层

### Unchanged Behavior (Regression Prevention)

3.1 WHEN 玩家在竞技场中进行 PVP/PVE 战斗时 THEN 系统 SHALL CONTINUE TO 正确计算伤害、判定范围、处理暴击，战斗体验不变

3.2 WHEN 玩家进入/离开竞技场时 THEN 系统 SHALL CONTINUE TO 正确广播玩家加入/离开消息，其他玩家能看到实时变化

3.3 WHEN 服务端进行增量状态同步时 THEN 系统 SHALL CONTINUE TO 以 5Hz 频率正确同步 HP/MP/位置/死亡状态，客户端显示与服务端一致

3.4 WHEN NPC 执行 AI 行为（追踪、攻击、脱战回归、恐惧逃跑）时 THEN 系统 SHALL CONTINUE TO 保持当前的 AI 行为逻辑和参数不变

3.5 WHEN 玩家在安全区内时 THEN 系统 SHALL CONTINUE TO 禁止攻击/技能/轻功/投掷，NPC 不进入安全区

3.6 WHEN 玩家死亡时 THEN 系统 SHALL CONTINUE TO 正确显示灵魂状态（半透明、overlay）、禁止交互、篝火复活

3.7 WHEN 篝火渲染和粒子效果运行时 THEN 系统 SHALL CONTINUE TO 保持当前的视觉效果不变

3.8 WHEN 地面掉落物出现时 THEN 系统 SHALL CONTINUE TO 支持按 E 键/左键批量拾取，显示浮动文字

3.9 WHEN 恐惧/昏迷状态效果生效时 THEN 系统 SHALL CONTINUE TO 正确控制玩家移动（恐惧自动逃跑、昏迷禁止移动）

3.10 WHEN 前端 WebSocket 消息收发时 THEN 系统 SHALL CONTINUE TO 正确解析和路由所有消息类型，保持当前的通信协议不变

3.11 WHEN 远程玩家实体在屏幕上显示时 THEN 系统 SHALL CONTINUE TO 正确插值移动、同步属性、显示名称和血条

3.12 WHEN 后端注册/登录/装备/背包等非战斗功能运行时 THEN 系统 SHALL CONTINUE TO 保持当前的功能逻辑不变（这些是 demo 特有的业务逻辑，不需要提取）
