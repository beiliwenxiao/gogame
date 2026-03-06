# cmd/demo — 修罗斗场 Demo

## 概述

多人实时 PVP 对战演示，运行在 `http://localhost:9100`。
启动方式：`cd cmd/demo && go run .`

## 目录结构

```
cmd/demo/
├── main.go          # 服务器入口、PlayerSession、Arena、DemoServer 定义
├── arena.go         # 竞技场逻辑：移动、攻击、技能、NPC AI、状态同步、复活
├── handlers.go      # 消息路由、注册/登录/装备等处理器
├── store/           # 数据层（SQLite）
│   ├── models.go    # Account、Character、EquipmentDef、SkillDef 等模型
│   ├── store.go     # Store 接口
│   ├── account.go   # 账号 CRUD
│   ├── character.go # 角色 CRUD & 属性计算
│   ├── equipment.go # 装备 CRUD
│   ├── seed.go      # 初始装备/技能数据
│   └── migrate.go   # 数据库迁移
└── static/          # 前端静态资源（html5-mmrpg-game 前端引擎）
    ├── index.html
    ├── js/
    │   ├── app.js       # 登录/注册/大厅 UI 逻辑
    │   ├── arena.js     # 竞技场入口（已废弃，由 ArenaScene 替代）
    │   └── ws.js        # WebSocket 封装
    └── src/             # 前端引擎源码（html5-mmrpg-game）
        ├── core/        # 引擎核心：GameEngine、SceneManager、InputManager 等
        ├── ecs/         # ECS：Entity、Component、EntityFactory
        ├── rendering/   # 渲染：IsometricRenderer、ParticleSystem、CombatEffects 等
        ├── systems/     # 游戏系统：CombatSystem、MovementSystem、NPCSystem 等
        ├── data/        # 静态数据：EquipmentData、ItemData
        ├── prologue/    # 序章场景基类（PrologueScene）及数据
        └── demo/
            └── scenes/
                └── ArenaScene.js  # 竞技场主场景（核心前端逻辑）
```

## 后端关键常量（main.go）

| 常量 | 值 | 说明 |
|------|-----|------|
| WSAddr | :9100 | WebSocket 监听地址 |
| ArenaWidth/Height | 960 | 竞技场尺寸（像素） |
| CampfireX/Y | 0 / 464 | 篝火坐标（等距地图中心） |
| TickRate | 30 | 移动同步帧率 |
| StateSyncHz | 5 | 状态同步频率 |
| CampfireRadius | 50 | 篝火复活范围（像素） |

## 消息协议（WebSocket JSON）

### 客户端 → 服务端
- `register` / `login` / `select_class`
- `equip` / `unequip` / `get_char_info` / `get_equip_list`
- `enter_arena` / `leave_arena`
- `move` — `{x, y, direction}`
- `attack` — `{target_id}`
- `attack_npc` — `{target_id}`
- `cast_skill` — `{skill_id, target_id?}`
- `cast_skill_npc` — `{skill_id, target_id}`
- `chat` — `{text}`

### 服务端 → 客户端
- `arena_state` — 进场时全量状态（players、npcs、campfire、skills、equipments）
- `player_joined` / `player_left` / `player_moved`
- `damage_dealt` — `{attacker_id, target_id, damage, is_crit, target_hp, target_is_npc?}`
- `player_died` — `{char_id, name, killer_id, killer}`
- `player_respawn` — `{char_id, name, x, y, hp, max_hp, mp, max_mp}`
- `state_sync` — 增量状态同步（HP/MP/位置/死亡）
- `npc_spawn` / `npc_died` / `npc_update` / `npc_drop`
- `skill_casted` / `chat_msg` / `char_info` / `equip_list`
- `error`

## 后端核心逻辑

### PlayerSession（main.go）
玩家会话，持有 WebSocket 连接和竞技场内状态（位置、HP/MP、装备属性、dead 标志）。

### Arena（main.go）
- `players map[int64]*PlayerSession` — 在场玩家
- `npcs map[int64]*ArenaNPC` — 在场 NPC
- `Broadcast(msg, exclude)` / `BroadcastAll(msg)`

### arenaTick（arena.go）
主循环，三个定时器：
1. `StateSyncHz(5Hz)` → `doStateSync()` 增量状态同步
2. `60s` → `spawnNPCWave()` NPC 刷新
3. `NPCAITickHz(2Hz)` → `npcAITick()` NPC AI

### 死亡与复活（arena.go）
- 玩家 HP ≤ 0 → `dead=true` → 广播 `player_died` → `go respawnPlayer()`
- 当前 `respawnPlayer`：等待 5 秒后在篝火附近随机位置自动复活
- 篝火坐标：`CampfireX=0, CampfireY=464`，复活范围半径 50px

### NPC 系统（arena.go）
- `ArenaNPC`：含 AI 状态（TargetID、AttackRange、AggroRange、FearUntil 等）
- `spawnNPCWave()`：每 60 秒刷新，读取 `EnemyData.json` 模板
- `npcAITick()`：追踪目标、攻击、脱战回归、恐惧逃跑

## 前端核心逻辑

### ArenaScene.js（src/demo/scenes/ArenaScene.js）
继承自 `PrologueScene`，是竞技场的主场景类。

关键属性：
- `selfId` — 本玩家 charID
- `remotePlayers: Map<charId, entity>` — 远程玩家实体
- `npcEntities: Map<npcId, entity>` — NPC 实体
- `campfire: {x, y, lit, ...}` — 篝火状态
- `ws` — WebSocket 引用（外部注入）
- `floatingTextManager` — 浮动文字管理器

关键方法：
- `onPlayerDied(data)` — 死亡处理：精灵半透明、显示击杀文字
- `onPlayerRespawn(data)` — 复活处理：恢复位置/HP/精灵透明度
- `onStateSync(data)` — 增量状态同步
- `renderCampfire(ctx)` — 渲染篝火（木材底座 + 发光 + 火焰帧动画）
- `initCampfireParticles()` — 初始化篝火粒子
- `setupBottomControlBar()` — 底部技能栏 UI
- `injectBackendSkills()` — 注入后端技能数据

### 前端 ECS 实体结构
玩家/NPC 实体包含以下 Component：
- `transform` — `{position: {x, y}}`
- `stats` — `{hp, maxHp, mp, maxMp, attack, defense, speed, level}`
- `sprite` — `{alpha, isWalking, direction}`
- `name` — `{name, class}`
- `combat` — 战斗相关

### 前端渲染层
- `IsometricRenderer` — 等距地图渲染
- `SpriteRenderer` — 精灵渲染
- `ParticleSystem` — 粒子效果
- `CombatEffects` — 战斗特效（伤害数字、技能特效）
- `WeaponRenderer` / `EnemyWeaponRenderer` — 武器渲染

## 职业系统

| 职业 | class 值 | 默认攻击范围 | 默认攻击距离 |
|------|----------|------------|------------|
| 战士 | warrior | 60px | 100px |
| 弓箭手 | archer | 200px | 250px |

## 数据文件（src/prologue/data/）

- `EnemyData.json` — NPC/敌人模板（template ID、属性、AI 参数）
- `EquipmentData.json` — 装备定义
- `ItemData.json` — 道具定义（红瓶/蓝瓶等）
- `AudioConfig.json` — 音效配置
- `Act1-6Data.json` — 序章各幕数据

## 开发注意事项

1. 前端新功能优先复用 `src/systems/`、`src/core/`、`src/rendering/` 中已有系统
2. 修改死亡/复活逻辑需同时修改后端 `arena.go` 和前端 `ArenaScene.js`
3. 新增消息类型需在 `main.go` 的 const 块中声明，并在 `handlers.go` 的 switch 中注册
4. 篝火位置由后端 `CampfireX/Y` 常量决定，前端通过 `arena_state` 消息接收
5. 状态同步采用增量模式（`doStateSync` 对比 `lastSync` 快照），避免全量广播


## 灵魂状态开发注意事项

### dead 标志的分布与检查
- 玩家灵魂状态用 `entity.dead` 布尔值标记，不是独立状态机
- `attackTarget()` 和 `castSkill()` 各自有独立的 `dead` 检查，但**交互入口（如 `handleEnemySelection`）也必须加检查**，否则死亡状态下仍可选中目标并显示高亮
- 修复模式：`onPlayerDied` 处理自己死亡时，除了视觉（透明度、overlay），还要清除所有交互状态（`selectedTarget = null` 等）

### ArenaScene 继承链
- `ArenaScene → BaseGameScene → PrologueScene → Scene`
- **ArenaScene 继承 BaseGameScene**，不是直接继承 PrologueScene
- `BaseGameScene` 里有完整的渲染管线、战斗系统、UI 面板等，ArenaScene 在此基础上扩展多人联网逻辑
- 搜索 ArenaScene 的渲染逻辑时，看 `ArenaScene.renderWorldObjects`（覆写）和 `BaseGameScene.renderEntity`（继承）

### 踩坑：PowerShell Select-String 多模式搜索
- `Select-String -Pattern "a|b"` 中的 `|` 有时被 shell 解析为管道，导致结果不完整
- 建议改用 `-Pattern "a" , "b"` 或用 `grepSearch` 工具替代


## 临时视觉效果的标准模式

ArenaScene 中临时效果（如技能范围指示器、NPC 白骨）统一用以下模式：

1. constructor 里初始化数组：`this.boneCorpses = []`
2. 触发时 push `{x, y, life, maxLife, ...}`
3. `update` 里用 `filter` 倒计时：`b.life -= deltaTime; return b.life > 0`
4. `renderWorldObjects` 里 push 带 `render` 回调的对象进 `renderQueue`，参与 Y 轴深度排序
5. `exit` 里清空数组

淡出效果：`life < fadeThreshold` 时 `alpha = life / fadeThreshold`，用 `ctx.globalAlpha` 应用。

新增临时效果时参考 `skillRangeIndicators`（范围指示器）或 `boneCorpses`（白骨）的实现。


## 新增消息类型的完整流程

1. `main.go` const 块声明常量（如 `MsgCampfireTick = "campfire_tick"`）
2. `arena.go` 里用 `BroadcastAll` 推送
3. 前端 `app.js` 注册 `ws.on('campfire_tick', (data) => arena.onCampfireTick(data))`
4. `ArenaScene.js` 实现对应的 `onXxx(data)` 方法
- 注意：`handlers.go` 只处理客户端→服务端的消息，服务端推送不需要在这里注册

## arenaTick 定时器模式

- 多个 `time.NewTicker` 并行跑在同一个 `select` 里，新增定时任务只需加一个 ticker + case
- 短周期倒计时用本地变量维护（如 `campfireCountdown`），复活触发时重置，不需要持久化

## 2.5D 地面圆圈画法

- 等距视角下地面圆圈用椭圆：`ctx.ellipse(x, y, rx, ry, 0, 0, Math.PI*2)`
- 水平半径是垂直半径的 2 倍，视觉上贴地（如 rx=100, ry=50）
- 配合 `createRadialGradient` 做内部光晕，中心透明度高、边缘渐变到 0

## Canvas 倒计时闪烁技巧

- `Math.floor(Date.now() / 400) % 2 === 0` 实现 400ms 周期闪烁
- 不需要额外状态变量，每帧渲染时直接计算，适合倒计时最后 N 秒的警示效果
