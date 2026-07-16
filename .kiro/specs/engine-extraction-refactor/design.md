# Design Document — engine-extraction-refactor

## 概述

本设计将 `cmd/demo` 中的通用功能提取回引擎层，分为后端（yijian18-server `internal/`）和前端（yijian18-engine 引擎 `src/`）两部分。重构后 demo 只负责提供参数和调用引擎 API。

### 核心原则

1. **yijian18-server 负责后端引擎**：网络通信、多人同步、服务端权威逻辑
2. **yijian18-engine 负责前端引擎**：渲染、ECS、场景基类、联网场景通用能力
3. **demo 只做胶水**：提供配置参数、注册回调、组装引擎模块
4. **不破坏单机游戏**：yijian18-engine 引擎的联网扩展通过 `_arenaMode` 标志和可选 mixin 实现，单机场景不受影响

## 第一部分：后端重构（yijian18-server `internal/`）

### 1.1 消除重复的范围判定/伤害计算函数（对应需求 1.1, 1.2）

**现状**：`arena.go` 中 `distance()`、`isInEllipseRange()`、`isInFanRange()`、`calcDamage()` 已经是对 `combat.Distance()` 等的薄包装。

**设计**：直接删除 `arena.go` 中的包装函数，所有调用点改为直接调用 `combat.Distance()`、`combat.IsInEllipseRange()`、`combat.IsInFanRange()`、`combat.CalcDamage()`。`isInSafeZone()` 保留为 demo 层的便捷函数（因为它绑定了 demo 特有的 `CampfireX/Y/Radius` 常量）。

**变更文件**：`cmd/demo/arena.go`

### 1.2 新增 NPC AI 框架（对应需求 1.6）

**现状**：`arena.go` 的 `npcAITick()` 约 350 行，包含追踪、攻击、脱战回归、恐惧逃跑等通用 AI 行为。

**设计**：在 `internal/combat/` 新增 `npcai.go`，提供通用 NPC AI 框架：

```go
// internal/combat/npcai.go

// NPCAIConfig NPC AI 配置
type NPCAIConfig struct {
    AttackRange  float64
    AggroRange   float64
    LeashRange   float64
    AttackCD     time.Duration
    CritRate     float64
    CritDmg      float64
    Speed        float64
}

// NPCState NPC 运行时状态（由调用方维护）
type NPCState struct {
    ID           int64
    X, Y         float64
    HP, MaxHP    float64
    Attack       float64
    Defense      float64
    Dead         bool
    TargetID     int64
    SpawnX, SpawnY float64
    LastAttackAt int64
    // 状态效果
    FearUntil    int64
    FearDirX     float64
    FearDirY     float64
    StunUntil    int64
    EnragedUntil int64
}

// NPCAITarget 潜在攻击目标
type NPCAITarget struct {
    ID      int64
    X, Y    float64
    HP      float64
    Defense float64
    Dead    bool
}

// NPCAIAction AI 决策结果
type NPCAIAction struct {
    Type     string  // "idle", "chase", "attack", "return", "fear_move"
    TargetID int64
    MoveX    float64
    MoveY    float64
    Damage   float64
    IsCrit   bool
}

// ComputeNPCAI 纯函数：根据 NPC 状态和目标列表计算 AI 行为
// 不修改任何状态，只返回决策结果
func ComputeNPCAI(npc *NPCState, config *NPCAIConfig, targets []NPCAITarget, nowMs int64, isInSafeZone func(x, y float64) bool) NPCAIAction
```

**关键设计决策**：
- `ComputeNPCAI` 是纯函数，不持有状态，不修改输入，只返回决策
- demo 层负责：收集目标列表、调用 `ComputeNPCAI`、根据返回的 `NPCAIAction` 执行副作用（修改 HP、广播消息）
- 这样 demo 保留了对锁和广播的完全控制权，引擎层不需要知道并发模型

**变更文件**：新增 `internal/combat/npcai.go`，修改 `cmd/demo/arena.go`

### 1.3 新增安全区判定工具（对应需求 1.7）

**设计**：在 `internal/combat/range.go` 新增通用区域判定函数：

```go
// ZoneConfig 安全区/兴趣区域配置
type ZoneConfig struct {
    CenterX, CenterY float64
    Radius           float64  // 椭圆水平半径
    Shape            string   // "ellipse", "circle", "rect"
}

// IsInZone 通用区域判定
func IsInZone(zone *ZoneConfig, x, y float64) bool
```

demo 的 `isInSafeZone()` 改为调用 `combat.IsInZone()`。

**变更文件**：`internal/combat/range.go`，`cmd/demo/arena.go`

### 1.4 适配 GameLoop（对应需求 1.8）

**现状**：`arena.go` 的 `arenaTick()` 用多个 `time.NewTicker` + `select` 管理定时任务。引擎层 `internal/engine/GameLoop` 提供了 4 阶段 tick 机制（Input → Update → Sync → Cleanup）。

**设计**：demo 创建 `GameLoop` 实例，将各定时任务注册为不同阶段的 handler：

```go
gl := engine.NewGameLoop(engine.GameLoopConfig{TickRate: NPCAITickHz})
gl.RegisterPhase(engine.PhaseUpdate, func(tick uint64, dt time.Duration) {
    s.npcAITick()  // NPC AI
})
gl.RegisterPhase(engine.PhaseSync, func(tick uint64, dt time.Duration) {
    if tick % (NPCAITickHz/StateSyncHz) == 0 {
        s.doStateSync()  // 状态同步（降频）
    }
})
```

**注意**：NPC 刷新（60s 周期）和篝火倒计时（1s 周期）仍用计数器派生，不需要独立 ticker。

**变更文件**：`cmd/demo/arena.go`

### 1.5 消息路由框架（对应需求 1.3）

**现状**：`main.go` 自行实现 WebSocket 升级和消息路由。引擎层 `internal/network/NetworkLayer` 提供了完整的连接管理但缺少 JSON 消息路由。

**设计**：在 `internal/network/` 新增 `router.go`，提供 JSON 消息路由器：

```go
// internal/network/router.go

// MessageRouter JSON 消息路由器
type MessageRouter struct {
    handlers map[string]MessageHandler
}

type MessageHandler func(session Session, data json.RawMessage)

func NewMessageRouter() *MessageRouter
func (r *MessageRouter) Register(msgType string, handler MessageHandler)
func (r *MessageRouter) Dispatch(session Session, rawMsg []byte) error
```

demo 的 `handleMessage` switch 改为注册模式：

```go
router := network.NewMessageRouter()
router.Register("move", s.handleMove)
router.Register("attack", s.handleAttack)
// ...
```

**变更文件**：新增 `internal/network/router.go`，修改 `cmd/demo/main.go`

### 1.6 房间广播适配（对应需求 1.4）

**现状**：demo 的 `Arena` 有自己的 `Broadcast`/`BroadcastAll`。引擎层 `RoomManager` 管理房间生命周期但没有广播方法。

**设计**：保留 demo 的 `Arena` 结构体（因为它持有 `PlayerSession` 而非通用 `Session`），但将通用的广播模式提取为引擎层工具：

```go
// internal/room/broadcast.go

// Broadcaster 通用广播接口
type Broadcaster interface {
    Broadcast(msg []byte, excludeSessionID string)
    BroadcastAll(msg []byte)
}
```

demo 的 `Arena` 实现 `Broadcaster` 接口。这样其他 demo/游戏也能复用同一模式。

**变更文件**：新增 `internal/room/broadcast.go`，修改 `cmd/demo/main.go`


## 第二部分：前端重构（yijian18-engine 引擎 `src/`）

### 设计原则

前端引擎的联网扩展必须满足：
1. **单机游戏不受影响**：所有联网功能通过可选 mixin 或条件分支（`_arenaMode`）实现
2. **BaseGameScene 是扩展点**：联网通用能力加到 `BaseGameScene`，ArenaScene 只做 demo 特有配置
3. **已有系统优先扩展**：`MultiplayerManager`、`NetworkCombatSystem`、`GroundDropPickupSystem` 等已有类优先扩展，不新建平行实现

### 2.1 MultiplayerManager 已完成提取（对应需求 1.9, 1.11）

**现状**：`MultiplayerManager` 已经从 ArenaScene 中提取出来，管理远程玩家和 NPC 的生命周期、位置插值、增量状态同步。ArenaScene 通过委托调用。

**设计**：保持现状，无需额外修改。`MultiplayerManager` 已经是 yijian18-engine 引擎层的通用类。

### 2.2 联网死亡/复活系统提取到 BaseGameScene（对应需求 1.10）

**现状**：ArenaScene 中 `onPlayerDied`/`onPlayerRespawn` 包含灵魂状态管理（overlay、双标志同步、死亡清理）。

**设计**：在 `BaseGameScene` 中新增可选的联网死亡/复活处理方法：

```javascript
// BaseGameScene 新增方法

/**
 * 联网模式：处理实体死亡（通用逻辑）
 * 子类可覆写 onNetworkDeathExtra(entity, data) 添加额外行为
 */
handleNetworkDeath(entity, data) {
    if (!this._arenaMode) return;
    // 双标志同步
    entity.dead = true;
    entity.isDead = true;
    entity.isDying = true;
    // 清理交互状态
    if (this.selectedTarget === entity.charId || this.selectedTarget === entity.npcId) {
        this.selectedTarget = null;
    }
    // 视觉效果
    const stats = entity.getComponent('stats');
    if (stats) stats.hp = 0;
    const sprite = entity.getComponent('sprite');
    if (sprite) { sprite.alpha = 0.3; sprite.isWalking = false; }
    // 子类扩展点
    this.onNetworkDeathExtra?.(entity, data);
}

/**
 * 联网模式：处理实体复活（通用逻辑）
 */
handleNetworkRespawn(entity, data) {
    if (!this._arenaMode) return;
    entity.dead = false;
    entity.isDead = false;
    entity.isDying = false;
    const stats = entity.getComponent('stats');
    if (stats) {
        stats.hp = data.hp;
        stats.maxHp = data.max_hp;
        stats.mp = data.mp;
        stats.maxMp = data.max_mp;
    }
    const sprite = entity.getComponent('sprite');
    if (sprite) sprite.alpha = 1.0;
    const transform = entity.getComponent('transform');
    if (transform) {
        transform.position.x = data.x;
        transform.position.y = data.y;
    }
    this.onNetworkRespawnExtra?.(entity, data);
}
```

ArenaScene 的 `onPlayerDied`/`onPlayerRespawn` 改为调用 `super.handleNetworkDeath()` + 自己的额外逻辑（灵魂 overlay、击杀文字等）。

**变更文件**：`src/prologue/scenes/BaseGameScene.js`，`src/demo/scenes/ArenaScene.js`

### 2.3 安全区系统提取（对应需求 1.12）

**现状**：ArenaScene 中 `isInSafeZone()`、`renderCampfire()`、`initCampfireParticles()` 是 demo 特有的篝火实现。

**设计**：将安全区判定提取为通用系统，篝火渲染保留在 demo 层：

```javascript
// src/systems/SafeZoneSystem.js（新增）

export class SafeZoneSystem {
    constructor(scene) {
        this.scene = scene;
        this.zones = []; // [{x, y, radius, shape}]
    }

    addZone(config) {
        this.zones.push(config);
    }

    isInAnyZone(x, y) {
        return this.zones.some(z => this._checkZone(z, x, y));
    }

    _checkZone(zone, x, y) {
        if (zone.shape === 'ellipse') {
            const dx = x - zone.x;
            const dy = y - zone.y;
            const rx = zone.radius;
            const ry = zone.radius / 2;
            return (dx*dx)/(rx*rx) + (dy*dy)/(ry*ry) <= 1;
        }
        // circle, rect 等其他形状...
    }
}
```

ArenaScene 在 `enter()` 中创建 `SafeZoneSystem` 并添加篝火安全区。篝火渲染（`renderCampfire`、`initCampfireParticles`）保留在 ArenaScene 中，因为这是 demo 特有的视觉效果。

**变更文件**：新增 `src/systems/SafeZoneSystem.js`，修改 `src/demo/scenes/ArenaScene.js`

### 2.4 GroundDropPickupSystem 完整封装（对应需求 1.13）

**现状**：`GroundDropPickupSystem` 和 `GroundDropRenderer` 已存在，但拾取触发逻辑（按键检测、范围判定、批量拾取、浮动文字）仍内联在 ArenaScene 的 `update()` 中。

**设计**：将拾取触发逻辑移入 `GroundDropPickupSystem.update()`：

```javascript
// GroundDropPickupSystem 扩展

update(deltaTime) {
    if (!this.scene._arenaMode) return;
    // 按键检测（E 键或左键）
    // 范围判定
    // 批量拾取
    // 浮动文字
}
```

ArenaScene 的 `update()` 中删除内联的拾取逻辑，改为 `this.groundDropPickupSystem.update(deltaTime)`。

**变更文件**：`src/systems/GroundDropPickupSystem.js`，`src/demo/scenes/ArenaScene.js`

### 2.5 状态效果移动控制提取（对应需求 1.14）

**现状**：ArenaScene 的 `update()` 中内联了恐惧自动移动逻辑（约 15 行）。

**设计**：在 `MovementSystem` 或新增 `StatusEffectMovementSystem` 中处理状态效果驱动的移动：

```javascript
// src/systems/StatusEffectMovementSystem.js（新增）

export class StatusEffectMovementSystem {
    constructor(scene) {
        this.scene = scene;
    }

    update(deltaTime) {
        const entity = this.scene.playerEntity;
        if (!entity) return;
        const nowMs = Date.now();

        // 恐惧：自动远离施法者方向逃跑
        if (entity.fearUntil && nowMs < entity.fearUntil) {
            this._applyFearMovement(entity, deltaTime);
            return; // 恐惧期间不处理其他移动
        }

        // 昏迷：禁止移动（由调用方检查 isStunned 跳过移动输入）
    }

    _applyFearMovement(entity, deltaTime) {
        const transform = entity.getComponent('transform');
        const stats = entity.getComponent('stats');
        if (!transform) return;
        const speed = (stats?.speed || 120) * 1.5;
        const fdx = entity.fearDirX || 0;
        const fdy = entity.fearDirY || 0;
        transform.position.x += fdx * speed * deltaTime;
        transform.position.y += fdy * speed * deltaTime;
        // 边界限制由调用方配置
        const sprite = entity.getComponent('sprite');
        if (sprite) sprite.isWalking = true;
    }
}
```

**变更文件**：新增 `src/systems/StatusEffectMovementSystem.js`，修改 `src/demo/scenes/ArenaScene.js`

### 2.6 WebSocket 通信统一（对应需求 1.15）

**现状**：`js/ws.js` 是独立的 WebSocket 封装，`app.js` 中自行注册消息路由。`src/core/MultiplayerManager.js` 已有通信能力但未被 `app.js` 使用。

**设计**：保持现状。`ws.js` 是 demo 的入口层胶水代码（登录/注册 UI 逻辑），不属于引擎层。`MultiplayerManager` 已经在引擎层提供了联网实体管理。两者职责不同：
- `ws.js` + `app.js`：demo 的 UI 层 WebSocket 通信（登录、大厅、消息路由到 ArenaScene）
- `MultiplayerManager`：引擎层的联网实体生命周期管理

不需要合并，但可以在 `MultiplayerManager` 中增加通用的 WebSocket 消息分发能力，让 demo 的 `app.js` 更薄。这是可选优化，不在本次重构范围内。


## 第三部分：重构后的分层架构

### 后端分层

```
┌─────────────────────────────────────────────┐
│  cmd/demo/                                   │
│  ├── main.go     配置参数 + 注册消息处理器    │
│  ├── arena.go    调用引擎 API + demo 特有逻辑 │
│  └── handlers.go 具体消息处理函数             │
├─────────────────────────────────────────────┤
│  internal/       引擎层                      │
│  ├── combat/     范围判定 + 伤害计算 + NPC AI │
│  ├── network/    连接管理 + 消息路由          │
│  ├── room/       房间管理 + 广播接口          │
│  ├── sync/       帧同步 + 状态同步            │
│  └── engine/     GameLoop + ObjectPool        │
└─────────────────────────────────────────────┘
```

### 前端分层

```
┌─────────────────────────────────────────────┐
│  src/demo/scenes/ArenaScene.js               │
│  ├── 配置参数（篝火坐标、竞技场尺寸）        │
│  ├── 调用引擎 API（MultiplayerManager 等）   │
│  └── demo 特有逻辑（篝火渲染、技能注入）     │
├─────────────────────────────────────────────┤
│  src/prologue/scenes/BaseGameScene.js        │
│  ├── 联网死亡/复活通用处理                    │
│  ├── _arenaMode 标志管理                     │
│  └── 渲染管线 + 战斗系统 + UI 面板           │
├─────────────────────────────────────────────┤
│  src/systems/                                │
│  ├── NetworkCombatSystem.js  联网战斗         │
│  ├── SafeZoneSystem.js       安全区判定       │
│  ├── StatusEffectMovementSystem.js 状态移动   │
│  ├── GroundDropPickupSystem.js 掉落物拾取     │
│  └── ...                                     │
├─────────────────────────────────────────────┤
│  src/core/                                   │
│  ├── MultiplayerManager.js   联网实体管理     │
│  ├── GameEngine.js           引擎核心         │
│  └── ...                                     │
└─────────────────────────────────────────────┘
```

## 第四部分：回归防护策略

### 后端回归防护

1. **范围判定**：`internal/combat/range.go` 已有完整单元测试，删除 demo 包装函数后运行 `go test ./internal/combat/... -count=1` 确认
2. **NPC AI**：新增 `internal/combat/npcai_test.go`，测试各 AI 行为分支（追踪、攻击、脱战、恐惧）
3. **消息路由**：新增 `internal/network/router_test.go`，测试消息注册和分发
4. **编译检查**：`go build ./cmd/demo/...` 确认无编译错误

### 前端回归防护

1. **单机游戏不受影响**：所有新增方法都有 `if (!this._arenaMode) return` 守卫
2. **ArenaScene 功能不变**：重构后运行 demo，验证 PVP/PVE 战斗、NPC AI、安全区、死亡/复活、掉落物拾取等功能正常
3. **继承链完整性**：`ArenaScene → BaseGameScene → PrologueScene → Scene` 不变

## 第五部分：实施优先级

按风险从低到高排序：

| 优先级 | 任务 | 风险 | 原因 |
|--------|------|------|------|
| P0 | 删除后端重复函数（1.1, 1.2） | 极低 | 已经是薄包装，只改调用点 |
| P1 | 安全区判定工具（1.7） | 低 | 纯函数提取，不改行为 |
| P2 | 消息路由框架（1.3） | 低 | 注册模式替换 switch，逻辑不变 |
| P3 | NPC AI 框架（1.6） | 中 | 逻辑复杂，需要仔细测试 |
| P4 | 前端死亡/复活提取（1.10） | 中 | 涉及双标志同步，容易遗漏 |
| P5 | 前端安全区系统（1.12） | 低 | 纯判定逻辑提取 |
| P6 | 掉落物拾取封装（1.13） | 低 | 移动代码位置，不改逻辑 |
| P7 | 状态效果移动（1.14） | 低 | 15 行代码提取 |
| P8 | GameLoop 适配（1.8） | 中 | 改变 tick 驱动模式 |
| P9 | 房间广播接口（1.4） | 低 | 接口抽象，不改实现 |

## 正确性属性

### CP1: 范围判定一致性
重构前后，对于任意坐标 (x1,y1) 和 (x2,y2)，`combat.IsInEllipseRange(x1,y1,x2,y2,r)` 的返回值必须与原 `arena.go` 中 `isInEllipseRange()` 完全一致。

### CP2: NPC AI 行为等价性
对于相同的 NPC 状态和目标列表输入，`ComputeNPCAI()` 返回的决策必须与原 `npcAITick()` 中的逻辑产生相同的行为（追踪/攻击/回归/恐惧）。

### CP3: 联网死亡双标志完整性
任何通过 `handleNetworkDeath()` 处理的实体，必须同时设置 `dead=true`、`isDead=true`、`isDying=true` 三个标志，防止 CombatSystem 单机复活逻辑介入。

### CP4: 安全区判定前后端一致性
前端 `SafeZoneSystem.isInAnyZone(x,y)` 和后端 `combat.IsInZone(zone,x,y)` 对于相同的坐标和区域配置，必须返回相同的结果。

### CP5: 单机游戏无回归
yijian18-engine 引擎中所有新增的联网方法，在 `_arenaMode === false`（默认值）时必须是无操作（no-op），不影响单机场景的任何行为。
