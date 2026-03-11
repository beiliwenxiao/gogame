---
inclusion: fileMatch
fileMatchPattern: "cmd/demo/**"
---

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
| CampfireRadius | 200 | 篝火安全区半径（像素） |

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
4. **`arena.js` 的 `ArenaRenderer` 里加代理方法**（这步容易漏！）：`onCampfireTick(data) { if (this.arenaScene) this.arenaScene.onCampfireTick(data); }`
5. `ArenaScene.js` 实现对应的 `onXxx(data)` 方法
- 注意：`handlers.go` 只处理客户端→服务端的消息，服务端推送不需要在这里注册
- **踩坑**：`app.js` 里的 `arena` 是 `ArenaRenderer`（`arena.js`），不是 `ArenaScene`，所有方法必须在 `ArenaRenderer` 里代理一层

## arenaTick 定时器模式

- 多个 `time.NewTicker` 并行跑在同一个 `select` 里，新增定时任务只需加一个 ticker + case
- **禁止用两个独立 ticker 管理同一状态变量**（如一个 60s ticker 重置倒计时、一个 1s ticker 递减广播），Go `select` 多 case 同时就绪时随机选择，会导致：重置后不广播、客户端收不到正确起始值、跳数等竞态 bug
- 正确模式：**单一最小粒度 ticker 统一管理**，倒计时每秒递减并广播，到 0 时触发动作并重置。其他周期性事件由计数派生，不要用独立 ticker
- 短周期倒计时用本地变量维护（如 `campfireCountdown`），不需要持久化

## 2.5D 地面圆圈画法

- 等距视角下地面圆圈用椭圆：`ctx.ellipse(x, y, rx, ry, 0, 0, Math.PI*2)`
- 水平半径是垂直半径的 2 倍，视觉上贴地（如 rx=100, ry=50）
- 配合 `createRadialGradient` 做内部光晕，中心透明度高、边缘渐变到 0

## Canvas 倒计时闪烁技巧

- `Math.floor(Date.now() / 400) % 2 === 0` 实现 400ms 周期闪烁
- 不需要额外状态变量，每帧渲染时直接计算，适合倒计时最后 N 秒的警示效果


## 前端实体死亡双标志体系

ArenaScene（多人联网）和 CombatSystem（单机引擎）各自有独立的死亡标志：
- `entity.dead` — ArenaScene 的灵魂状态标志，由后端 `player_died`/`state_sync` 消息驱动
- `entity.isDead` / `entity.isDying` — CombatSystem 的 ECS 生命周期标志，由 `checkDeath()` 每帧检测 `stats.hp <= 0` 触发

**冲突机制**：如果只设了 `entity.dead` 而没设 `isDead`/`isDying`，CombatSystem 会认为玩家"刚死"，触发 `handlePlayerDeath` → `setTimeout(5s, revivePlayer)` → `stats.fullRestore()` 把血回满。

**必须遵守的规则**：在 ArenaScene 所有设置 `entity.dead` 的地方（`onPlayerDied`、`onPlayerRespawn`、`onStateSync`），必须同步设置 `isDead`/`isDying`，阻止 CombatSystem 的单机复活逻辑介入。

### NPC 同样适用双标志规则

NPC 实体和玩家实体共享同一套 ECS 组件，CombatSystem 不区分玩家和 NPC。NPC 的 `isDead`/`isDying` 需要在 4 个位置同步：
1. `onNPCSpawn` — 创建时如果 `dead=true`，同步设置
2. `onNPCDied` — 收到死亡消息时立即设置
3. `onStateSync` NPC 部分 — 增量同步发现 `npc.dead` 时设置
4. `onDamage` — NPC 血量归零时**提前设置**

**踩坑：onDamage 时间窗口**：`damage_dealt` 消息先于 `npc_died` 到达前端，中间有时间窗口让 CombatSystem 检测到 `stats.hp <= 0` 并触发单机复活逻辑（"诈尸"现象）。必须在 `onDamage` 中检测 `target_is_npc && stats.hp <= 0` 时提前设置双标志。

### ParticleSystem 有限时长发射器

`particleSystem.createEmitter()` 的 `duration` 参数（毫秒）控制发射器生命周期，设为有限值即可自动停止，无需手动清理。适合复活光环等一次性粒子效果。多方向环形粒子用角度循环 + `Math.cos/sin` 计算速度分量，Y 轴速度乘 0.5 适配等距视角。


## 安全区（Safe Zone）系统

### 规则
- 安全区 = 篝火周围 `CampfireRadius`（200px）范围
- 安全区内的玩家：禁止攻击（PVP + PVE）、禁止使用技能、禁止轻功、禁止投掷武器
- NPC 不能进入安全区、不能刷新在安全区内、不追踪安全区内的玩家
- 安全区半径由后端 `CampfireRadius` 常量定义，通过 `arena_state.campfire.radius` 下发前端

### 需要覆盖的完整入口清单

后端 4 个 handler（arena.go）：
1. `handleAttack` — 普攻玩家
2. `handleAttackNPC` — 普攻 NPC
3. `handleCastSkill` — 技能打玩家
4. `handleCastSkillNPC` — 技能打 NPC

前端 6 个入口（ArenaScene.js）：
1. `handleEnemySelection` — 交互层（选中目标）
2. `attackTarget` — 执行层（发送攻击）
3. `castSkill` — 执行层（发送技能）
4. `handleTeleport` — 轻功（覆写 BaseGameScene）
5. `handleWeaponThrow` — 投掷武器（覆写 BaseGameScene）
6. `renderCampfire` — 视觉层（安全区圈渲染）

NPC AI 3 个位置（arena.go npcAITick）：
1. 目标筛选 — `alivePlayers` 过滤掉安全区内玩家
2. 移动边界 — 追踪/恐惧/回归移动前检查 `isInSafeZone(newX, newY)`
3. 刷新距离 — `spawnNPCWave` 最小距离 = `CampfireRadius + 50`

### 检查原则
- 只检查攻击者/施法者位置，不双向检查目标位置（安全区内无 NPC，区内玩家无法被选中）
- 后端是权威拦截，前端是预判提示（浮动文字 `'安全区内禁止xxx'`）

### `super.update()` 绕过安全区的陷阱

`ArenaScene.update` 调用 `super.update()` 会触发 BaseGameScene 里的单机战斗系统（MeleeAttackSystem.performSectorAttack、MeleeAttackSystem.updateSectorSlashEffects、CombatSystem.handleSkillInput），这些系统有独立的攻击/技能逻辑，完全绕过 ArenaScene 的安全区检查。

解决方案：ArenaScene.update 在 `super.update()` 前后设置/清除 `_safeZoneDisabled` 标志：
1. `super.update()` 前：检测玩家是否在安全区，若是则在 MeleeAttackSystem 和 CombatSystem 上设 `_safeZoneDisabled = true`
2. `super.update()` 后：清除标志
3. 各系统内部检测标志，跳过攻击/技能逻辑

与 `_arenaMode`（场景级持久开关）不同，`_safeZoneDisabled` 是每帧动态设/清，因为玩家会进出安全区。

### 继承链中的行为拦截模式
- `handleTeleport` 和 `handleWeaponThrow` 定义在 `BaseGameScene` 中
- ArenaScene 通过覆写 + `super.xxx()` 模式拦截，不修改 BaseGameScene 通用代码
- 轻功触发：`Ctrl+鼠标左键`，由 `update()` 每帧调用 `handleTeleport()` 检测
- 投掷触发：`Shift+鼠标左键`，由 `handleMouseClick()` 中 `shiftPressed` 分支调用
- 两者都是纯前端行为（FlightSystem / WeaponRenderer），无后端消息，只需前端拦截


## `_arenaMode` 标志：联网模式禁用单机逻辑

- CombatSystem 的 `handlePlayerDeath` 内置 `setTimeout(5s, revivePlayer)` 自动复活，双标志有时序竞态风险
- 在 CombatSystem 上设 `_arenaMode` 标志，`handlePlayerDeath` 检测到直接 return，从根源切断单机复活
- ArenaScene `enter` 时 `this.combatSystem._arenaMode = true`，`exit` 时清除
- 此模式可推广到其他需要联网/单机行为分离的系统（如 `_safeZoneDisabled` 同理）

## 死亡清理完整清单

玩家死亡（`onPlayerDied`）时需清理的所有状态：
1. 双标志：`entity.dead = true` + `entity.isDead = true` + `entity.isDying = true`
2. `stats.hp = 0`
3. `selectedTarget = null`
4. `meditationSystem.stop()`（打坐回血）
5. 精灵透明度 `sprite.alpha = 0.3` + `sprite.isWalking = false`
6. `_showSoulOverlay()`

**注意**：`StatsComponent.heal()` 不检查死亡状态，任何回血来源都能让死人"诈尸"，必须从调用方层面阻断。

## 安全区 2.5D 椭圆判定

- 渲染用椭圆 `rx=radius, ry=radius/2`，碰撞判定必须同步用椭圆公式：`(dx/rx)² + (dy/ry)² <= 1`
- 前端 `ArenaScene.isInSafeZone` 和后端 `arena.go isInSafeZone` 必须同时改，否则判定不一致
- 不能用圆形 `distance <= radius` 代替椭圆判定

## 鼠标按键分流

### 左键攻击 / 右键移动的入口清单
改鼠标按键分配时，需排查所有响应鼠标点击的入口：
- `MovementSystem.handleClickMovement` — `button === 2`（右键移动）
- `ArenaScene.handleEnemySelection` — `button === 0`（左键选中）
- `CombatSystem.handleTargetSelection` — 已有 `button === 0`
- `MeleeAttackSystem` — 用 `isMouseDown()` 不受 `isMouseClicked()` 影响
- `MovementSystem.findEnemyAtPosition` — 只查 `type === 'enemy'`，ArenaScene 实体不匹配，右键点敌人位置仍正常移动

### InputManager 鼠标按键机制
- `handleMouseDown` 记录 `event.button`（0=左, 2=右）到 `this.mouse.button`
- `isMouseClicked()` 不区分按键，需配合 `getMouseButton()` 过滤
- Canvas 已有 `contextmenu` 的 `preventDefault()`，右键菜单不会弹出


## 攻击触发链路与快捷键机制

### 攻击两步分离模式
- ArenaScene 的攻击流程是"选中目标 → 触发攻击"两步分离
- `handleEnemySelection()`（左键点击选中）由父类 `BaseGameScene.update` 调用
- `attackTarget()` 需要额外触发入口（快捷键、UI 按钮等），不会自动执行
- `attackTarget()` 内置完整防护链（无目标 → 灵魂状态 → 目标死亡 → 安全区 → 距离预判），新增触发入口只需调用它，不需要重复检查

### arena.js 的废弃状态
- `ArenaRenderer.renderSkillBar()` 中有旧的键盘监听器（空格键攻击、数字键技能）
- 但 `start()` 已注释掉 `renderSkillBar()` 调用，改用引擎 `BottomControlBar`，旧监听器不会注册
- 新的键盘快捷键功能必须在 `ArenaScene` 中实现，不能依赖 `arena.js`

### InputManager 键盘按键机制
- `isKeyPressed(key)` 是单帧触发（`keysPressed` map 在 `update` 末尾清空），适合"按一次打一次"的操作
- 空格键 `' '` 已在 `handleKeyDown` 的 `preventDefault` 列表中，不会触发页面滚动
- 快捷键检测应放在 `ArenaScene.update()` 中 `super.update()` 之前的自定义逻辑区域


## InputManager keyMap 映射陷阱

### 核心规则
`InputManager.handleKeyDown` 用 `const mappedKey = this.keyMap[key] || key` 将原始键名转换后存入 `keysPressed`/`keys`。调用 `isKeyPressed()` / `isKeyDown()` 时**必须使用映射后的名称**，不能用原始键名。

### 完整映射表
- 移动：`WASD` / 方向键 → `up` / `down` / `left` / `right`
- 数字：`1-7` → `skill1-skill7`
- 功能：`' '` → `space`、`Escape` → `escape`、`Enter` → `enter`、`Shift` → `shift`、`Control` → `ctrl`、`Tab` → `tab`
- 未映射的字母键（`m`、`h`、`r` 等）保持原始键名

### 两套键盘监听机制
- `arena.js`（旧）：`window.addEventListener('keydown')` 直接监听 `event.key`，键名是原始的（如 `' '`）
- `ArenaScene`（新）：通过 `InputManager.isKeyPressed/isKeyDown` 查询，键名是映射后的（如 `'space'`）
- 两套机制的键名**不能混用**


## 键盘攻击与自动选中模式

### attackTarget 的前置条件
- `attackTarget()` 第一行 `if (!this.selectedTarget) return`，只负责"对已选中目标发送攻击消息"
- 鼠标攻击通过 `handleEnemySelection` 左键点击预先选中目标，键盘快捷键没有这个前置步骤

### 键盘攻击必须配合 autoSelect
- 键盘触发攻击（如空格键）时，如果 `selectedTarget` 为空，需要先调用 `_autoSelectNearestEnemy()` 自动选中攻击范围内最近的敌人
- `_autoSelectNearestEnemy()` 遍历 `npcEntities` + `remotePlayers`，在武器攻击范围内找最近存活目标，遵守安全区限制
- 标准调用模式：`if (!this.selectedTarget) this._autoSelectNearestEnemy(); this.attackTarget();`

### 鼠标 vs 键盘攻击流程
- 鼠标：`handleEnemySelection`（左键选中）→ 空格/其他方式 → `attackTarget`
- 键盘：按键检测 → `_autoSelectNearestEnemy`（无目标时）→ `attackTarget`
- 两条路径最终汇入 `attackTarget`，共享同一套防护检查链（灵魂状态、安全区、距离预判）


## EquipmentComponent API 速查

### 正确方法名
- `getEquipment(slotType)` — 获取指定槽位装备（**不是** `getEquipped`）
- `getAllEquipment()` — 获取所有装备
- `equip(slotType, equipment)` — 装备物品，返回被替换的旧装备
- `unequip(slotType)` — 卸下装备
- `getBonusStats()` — 获取装备属性加成总和
- `getBonusStat(statType)` — 获取单项属性加成

### 槽位名
`mainhand`、`offhand`、`helmet`、`armor`、`boots`、`ring1`、`ring2`、`necklace`、`belt`、`accessory`、`instrument`、`mount`

### 踩坑：复制代码前先验证 API
`attackTarget` 中原本就写错了 `getEquipped`，因为之前没有键盘攻击入口，错误分支未被执行。新增功能入口会暴露已有代码中的潜伏 bug，复制代码时要先确认原代码的 API 调用是否正确。


## NetworkCombatSystem — 联网战斗系统

### 职责
`src/systems/NetworkCombatSystem.js` 封装了所有联网战斗逻辑，ArenaScene 中的战斗方法全部委托给它：
- `attackTarget()` — 联网普攻（含弯刀动画触发）
- `castSkill(skillId)` — 联网技能施放
- `autoSelectNearestEnemy()` — 自动选中最近敌人
- `handleEnemySelection()` — 鼠标左键点击选敌
- `onDamage(data)` — 处理 `damage_dealt` 消息（HP 更新 + 浮动文字 + NPC 双标志）
- `onSkillCasted(data)` — 处理 `skill_casted` 消息（MP 同步 + 粒子效果）
- `handleSpaceAttack()` — 空格键攻击组合入口（检测按键 + 自动选敌 + 攻击）

### 使用方式
- ArenaScene constructor 中 `this.networkCombat = new NetworkCombatSystem(this)`
- 场景方法改为一行委托：`attackTarget() { this.networkCombat.attackTarget(); }`
- `app.js` 的消息注册链路不变（`arena.onDamage(data)` → ArenaScene → NetworkCombatSystem）

### 联网攻击的弯刀动画
- 单机模式由 `CombatSystem` 内部调用 `weaponRenderer.startAttack()`
- 联网模式 `attackTarget()` 只发 WebSocket 消息，不经过 CombatSystem，必须在发送后手动调用 `weaponRenderer.startAttack('thrust')`
- 攻击方向通过 `Math.atan2(dy, dx)` 计算并设置 `weaponRenderer.currentMouseAngle`

### `entity.dead` 为 undefined 的特性
- 未死亡的实体 `dead` 属性为 `undefined`（非 `false`），依赖 JS falsy 判定
- `dead` 是动态挂载属性，不在组件初始化时设定


## 群攻模式（attackAllInRange）

### 机制
- `attackAllInRange()` 遍历 `npcEntities` + `remotePlayers` 所有存活目标，用 `combat.isInSkillRange` 做范围判定，对每个命中目标单独发送 `attack_npc`/`attack` 消息
- 后端每条消息独立处理，不需要新增群攻协议
- 弯刀动画朝向第一个命中目标（`Math.atan2` 计算角度）
- `selectedTarget` 自动设为第一个命中目标（用于 UI 高亮），但攻击本身不依赖它

### 触发入口
- 空格键：`handleSpaceAttack()` → `attackAllInRange()`
- 左键点击：`handleEnemySelection()` → `attackAllInRange()`
- 两者效果完全一致，左键不做"选中目标"，直接群攻

### 弯刀特效必须无条件触发
- `attackAllInRange()` 中 `weaponRenderer.startAttack('thrust')` 不能放在 `if (attacked)` 条件内，否则范围内无敌人时按空格/左键没有挥刀动画
- 正确模式：有目标时 `currentMouseAngle` 朝目标方向，无目标时保持当前鼠标方向（由 `BaseGameScene.update` → `updateMouseAngle` 每帧维护），然后无条件调用 `startAttack`
- `startAttack()` 内部有 `if (this.attackAnimation.active) return` 防重入，不会叠加动画

### 联网攻击的刀光/箭光特效
- 联网攻击绕过 `MeleeAttackSystem`，刀光/箭光特效不会自动触发，需在 `attackAllInRange()` 里手动调用
- 入口：`meleeAttackSystem.spawnSectorSlashEffect(playerCenter, dir, cx, cy, sectorRadius)`
- `playerCenter` = `transform.position` 减去 `spriteHeight/2`（脚底坐标转身体中心）
- `dir` 有目标时用 `Math.atan2(dy, dx)` 朝目标方向，并同步写入 `meleeAttackSystem.sectorDirection`；无目标时直接读 `sectorDirection`（每帧由 `MeleeAttackSystem.update` 跟随鼠标维护）
- `sectorRadius` 读 `meleeAttackSystem.sliceAttackRange`
- 安全区内不触发任何特效（弯刀和刀光都不触发）

### 联网攻击与单机系统共享冷却状态

武器冷却由 `MeleeAttackSystem` 管理，核心状态：
- `sliceLastAttackTime` — 上次攻击时间（秒，`performance.now() / 1000`）
- `sliceGlobalCooldown` — 默认冷却，实际值从装备读取（`mainhand.attackSpeed` 优先，其次 `offhand.attackSpeed`）
- `sliceCooldownShown` — 冷却提示防重复标志

`NetworkCombatSystem.attackAllInRange` 绕过了 `MeleeAttackSystem`，需手动复制同一套冷却逻辑：
1. 攻击前：读 `mas.sliceLastAttackTime` + 装备 `attackSpeed` 计算剩余冷却，未就绪则显示"冷却中 Xs"并 return
2. 攻击后：写回 `mas.sliceLastAttackTime = performance.now() / 1000` + 重置 `mas.sliceCooldownShown = false`

两者操作同一个 `meleeAttackSystem` 实例，天然共享，无需额外同步。

**通用原则**：联网攻击绕过单机系统时，凡是单机系统管理的"状态"（冷却、特效、动画），联网侧都需要手动读写。不只是"禁用单机逻辑"，有时还需要"复用单机状态"。

### 注意事项
- `inputManager.markMouseClickHandled()` 必须在 `attackAllInRange()` 之前调用，防止 MovementSystem 同帧响应左键导致角色移动
- 原有的 `attackTarget()` 仍保留用于单目标攻击场景（如技能指定目标）


## 技能 area_type 驱动范围判定

后端 `handleCastSkill` / `handleCastSkillNPC` 用 `skill.AreaType` switch 分支统一处理：
- `fan` — 扇形（猛击），以施法者为中心，朝目标方向，半角 45°（π/4），用 `isInFanRange`
- `ellipse` — 椭圆（旋风斩/战吼），以施法者为中心，用 `isInEllipseRange`
- `circle` — 圆形，以目标点为中心
- `single` — 单体，距离判定

前端 `NetworkCombatSystem.castSkill` 也必须按 `area_type` 分支做预判，两端保持一致。漏掉任何一端会导致判定不一致（前端放行但后端拦截，或前端拦截但后端命中）。

动态范围覆写规则（后端在 handler 里按技能名修改）：
- 猛击：`skill.Range = session.weaponAttackDist`
- 旋风斩：`skill.AreaSize = session.weaponAttackDist`
- 战吼：`skill.AreaSize = session.weaponAttackDist * 3`

## seed.go 技能数据版本检测

通过查询具体技能的字段值来判断是否需要重建数据，比只检查 `COUNT(*)` 或 `icon_id` 更精准：

```go
var skillVersion string
s.db.QueryRow("SELECT area_type FROM skill_defs WHERE name='猛击' LIMIT 1").Scan(&skillVersion)
if skillVersion != "fan" {
    // 旧数据，删除重建
}
```

新增技能字段或修改技能属性后，更新此处的检测条件，确保旧数据库能自动重建。


## 技能范围虚线的实现模式

### 实时跟随 vs 临时效果
- 范围指示器不能用"临时效果数组 + life 倒计时"模式，因为它需要每帧实时跟随玩家位置
- 正确做法：每帧直接从 `transform.position` 读取坐标，在 `renderWorldObjects` 末尾直接绘制，不入队列、不存状态
- 与 `boneCorpses` 的区别：范围指示器是"持续存在但条件显示"，不是"触发后倒计时消失"

### arena_state 技能数据的 area_size 问题
- `arena_state` 下发的 `skills` 是数据库原始数据，`area_size` 对旋风斩/战吼是 `0`
- 后端在 `handleCastSkill` 里动态覆写 `AreaSize`，但不写入 `arena_state`
- 前端范围预览必须用 `skill.area_size || weaponDist` 做 fallback（`0 || x` 在 JS 里会正确 fallback）

### BottomControlBar skillIndex 映射
- `skillSlots[i].skillIndex`：前2个槽是药水（`skillIndex = -1`），后5个是技能（`skillIndex = 0~4`）
- 获取后端技能原始数据的链路：`hoveredSlot` → `skillSlots[i].skillIndex` → `combat.skills[skillIndex].backendId` → `this.skills.find(s => s.id === backendId)`

### renderWorldObjects 的相机变换上下文
- `renderWorldObjects` 在 `ctx.translate(-viewBounds.left, -viewBounds.top)` 之后调用，处于相机变换内部
- 直接用世界坐标绘制即可，不需要手动换算屏幕坐标
- `inputManager.getMouseWorldPosition(this.camera)` 返回世界坐标，与 `transform.position` 坐标系一致，可直接用于方向计算


## 技能范围指示器的触发策略

### 近战 vs 远程的显示差异
- 战士近战技能（fan/ellipse）：**始终显示**范围圈跟随玩家，不需要任何触发条件（hover/按键），因为近战需要直观看到攻击覆盖区域
- 弓箭手远程技能（circle/single）：不始终显示，范围以目标点为中心，跟随玩家没有意义
- 过滤条件：`skill.class === 'warrior'` + `area_type !== 'single'` + `mp_cost > 0`

### 不要用 hover 触发战斗相关的持续显示效果
玩家操作时鼠标在世界区域（打怪/移动），不会悬停在底部 UI 上，`hoveredSlot` 始终为 `-1`。hover 只适合 tooltip 类的临时提示，不适合需要持续可见的战斗辅助信息。

### 后端技能数据的 class 字段
`arena_state` 下发的 `skills` 数组每个技能都有 `class` 字段（`"warrior"` / `"archer"`），前端可直接用于职业过滤，不需要额外映射表。


## 触发式技能范围指示器

### "临时效果 + 跟随实体"混合模式
- `skillRangeIndicators` 数组存储指示器对象（含 `life` 倒计时），但渲染时不用存储的 `x/y`，而是每帧从 `_getFootCenter()` 实时读取玩家脚下坐标
- 与纯临时效果（`boneCorpses`，坐标固定不动）不同，这是"有生命周期 + 实时跟随"的混合模式
- 触发入口：`attackAllInRange()`（普攻）和 `onSkillCasted()`（技能），调用 `scene._showSkillRange(opts)` push 到数组

### Canvas 虚线动画
- `ctx.setLineDash([6, 4])` + `ctx.lineDashOffset = ind.dashOffset` 实现虚线流动
- `update` 中每帧 `dashOffset += 60 * deltaTime` 递增偏移量
- 容易遗漏：只设了 `setLineDash` 但忘了 `lineDashOffset`，虚线就是静止的

### skill_casted 消息的范围数据
- 后端 `skill_casted` 广播已包含 `area_type`、`area_size`，前端 `onSkillCasted` 直接读取
- 比从本地 `this.skills` 反查更可靠，因为后端会动态覆写 `AreaSize`（旋风斩=武器距离，战吼=武器距离×3）

### 触发式 vs 始终显示
- 触发式（当前方案）：`_showSkillRange()` push 数组 → `update` 倒计时 → 渲染遍历，适合"反馈型"效果
- 始终显示：`renderWorldObjects` 直接绘制，不需要数组/life，适合"规划型"效果


## 指示器数组中的冗余坐标字段

- `_showSkillRange` push 的对象包含 `x/y`（触发时的玩家坐标），但 `_renderSkillRangeIndicator` 渲染时用 `_getFootCenter()` 实时坐标覆盖，存储的 `x/y` 实际未使用
- 当前只有自己的技能会触发指示器，所以直接读玩家坐标没问题
- 未来如果需要显示其他实体的技能范围，应改为存储 entity 引用而非坐标

## handleCastSkill 与 handleCastSkillNPC 的对称性

- 两个 handler 的技能查找、MP 扣除、动态范围覆写（猛击/旋风斩/战吼）、`skill_casted` 广播字段完全对称
- 修改技能逻辑时必须同时改两个 handler，漏改一个会导致打玩家和打 NPC 的行为不一致
- 广播的 `skill_casted` 消息包含后端动态覆写后的 `area_size`，前端直接读取比本地反查更准确


## 2.5D 椭圆扇形的完整实现模式

普攻（`attackAllInRange`）和猛击（`handleCastSkill` fan 分支）共用椭圆扇形判定：

### 三处一致性
1. 前端渲染（`_renderSkillRangeIndicator`）：`rx = radius, ry = radius/2`，`ctx.lineTo(cx + cos(a)*rx, cy + sin(a)*ry)`
2. 前端碰撞（`attackAllInRange`）：`dy2d = dy * 2` 还原等距压缩，`dist2d = sqrt(dx² + dy2d²)`，角度用 `atan2(dy2d, dx)`
3. 后端碰撞（`isInFanRange`）：同样 `dy * 2` 还原，`math.Atan2` 计算方向

三处 Y 轴压缩比必须一致（均为 0.5），改任何一处都要同步其余两处。

### 普攻扇形参数的读取链路
- `sectorDir`：`meleeAttackSystem.sectorDirection`（每帧跟随鼠标维护）
- `sectorHalfAngle`：`mainhand.attackRange`（角度制 → 弧度 `* π/180 / 2`）
- `sectorRadius`：`mainhand.attackDistance`，fallback `meleeAttackSystem.sliceAttackRange`
- 与 `MeleeAttackSystem` 单机扇形攻击共享同一数据源，联网/单机攻击范围一致


## handleSkillInput 的联网模式盲区与修复

### 问题
`CombatSystem.handleSkillInput` 是键盘数字键（3-7）触发技能的唯一入口。内部有 `basic_attack`、`heal`、`meditation` 三个特殊分支，其余技能走 `tryUseSkillAtPosition`（单机本地执行）。

`injectBackendSkills` 注入的技能 id 格式为 `backend_18`，不匹配任何特殊分支，直接掉进单机 else 分支，导致：
- 不发 WebSocket → 后端无响应 → 无 `skill_casted` → 范围指示器不触发
- 单机执行产生 `伤害: NaN` 副作用（无正确攻击力数据）

### 修复模式：onNetworkSkillCast 回调
在 `handleSkillInput` 的 else 分支前插入条件：`_arenaMode && skill.backendId && onNetworkSkillCast`，命中时调用回调走联网路径。

- `ArenaScene.enter`：`combatSystem.onNetworkSkillCast = (skillId) => this.castSkill(skillId)`
- `ArenaScene.exit`：`combatSystem.onNetworkSkillCast = null`
- 键盘和 UI 点击两条路径最终汇入 `NetworkCombatSystem.castSkill`

### 通用规律
`CombatSystem` 每次联网场景新增功能入口时，都需检查 `handleSkillInput`、`handlePlayerDeath` 等方法是否有单机逻辑被意外触发。`_arenaMode` 标志 + 回调委托是标准分离模式。


## PlayerSession vs ArenaNPC 字段差异

- `ArenaNPC`（main.go）已有 `FearUntil/FearDirX/FearDirY` 等 AI 状态字段，`PlayerSession` 不一定有
- 两者虽然都在 `main.go` 中定义，字段集合不同（NPC 有 AI 相关字段，玩家有装备/同步相关字段）
- 改技能效果涉及玩家和 NPC 双目标时，必须分别确认两个 struct 的字段是否齐全，不能看到一个有就以为另一个也有


## 武器攻击属性的语义分离

### attackRange vs attackDistance
- `attackRange`（角度，度数）= **技能**扇形范围，不用于普攻判定
- `attackDistance`（像素）= **普攻**距离判定
- 后端 `handleAttack`/`handleAttackNPC` 必须用 `weaponAttackDist`，不能用 `weaponAttackRange`
- 战士武器：`attackRange=90°`（近战扇形），`attackDistance=100px`
- 弓箭手武器：`attackRange=30°`（技能扇形备用），`attackDistance=250px`

### 远程 vs 近战普攻判定分离
- 近战普攻：扇形判定（角度 + 距离），用 `sectorHalfAngle` + `sectorRadius`
- 远程普攻：全方向距离判定（只看 `attackDistance`，不限角度）
- `attackAllInRange` 里用 `mas.sectorIsRanged` 分支区分两种判定

### 前端装备字段映射的坑
- 后端 JSON 字段名：`attack_range`、`attack_distance`、`attack_interval`（snake_case）
- `loadBackendEquipments` 里 `item.subType` 默认被设成槽位名（`'mainhand'`），导致 `checkIsRangedWeapon` 识别失败（它检查 `subType === 'bow'`）
- 弓箭手武器必须显式设 `item.subType = 'bow'`，才能触发箭矢消耗和箭光特效

### `_getWeaponRange` 返回值语义
- 必须返回 `attackDistance`（像素距离），不能返回 `attackRange`（角度值）
- 原来返回角度值（30）被当距离用，导致弓箭手普攻几乎打不到任何人


## npcAITick 的时序竞态与幽灵攻击

### 问题根源
`npcAITick` 执行模式：持锁 → 收集攻击列表 → 解锁 → 广播。在解锁到广播之间存在时间窗口，玩家可能在此期间击杀了 NPC（`handleAttackNPC` 设 `npc.Dead=true` 并广播 `npc_died`）。前端可能先收到 `npc_died`（删除实体），再收到 `npcAITick` 广播的 `damage_dealt`，造成"幽灵攻击"。

### 修复模式：双层防护
- 后端（`arena.go`）：广播攻击前再次加读锁检查 NPC 是否仍存活，已死亡则 `continue` 丢弃
- 前端（`NetworkCombatSystem.onDamage`）：收到 `attacker_is_npc` 时，验证攻击者 NPC 是否还在 `npcEntities` 且 `dead/isDead` 未设置，不满足则 `return`

### 通用规律：锁外广播的竞态窗口
Go 的"锁内收集、锁外广播"模式在高并发下必然存在竞态窗口。广播前应再次验证数据有效性，不能假设锁内收集的数据在广播时仍然有效。

### 前端消息处理的防御性原则
WebSocket 消息到达顺序不保证与服务端发送顺序完全一致。处理任何消息时，都应检查相关实体是否仍然存在且状态有效，不能假设"先发的消息一定先到"。

### "锁内收集、锁外广播"模式中的副作用延迟原则

锁内阶段只做纯计算和数据收集，**不修改任何共享状态**（HP、dead 标志等）。副作用必须延迟到锁外二次检查通过后才执行。

反例（`npcAITick` 旧实现）：锁内直接 `closest.hp -= dmg`，解锁后二次检查发现 NPC 已死跳过广播，但 HP 已被扣除，`doStateSync` 把错误 HP 同步给前端，表现为"NPC 死了但玩家还在掉血"。

正确做法：锁内只记录 `{target, damage}` 到攻击列表，锁外加写锁确认 NPC 存活后才执行 `target.hp -= damage` 并广播。副作用和广播绑定在同一个原子操作中。

排查此类 bug 的思路：
1. 锁内阶段修改了哪些共享状态？
2. 解锁到广播之间其他 goroutine 可能做了什么？
3. 广播被过滤后，已生效的副作用是否导致数据不一致？

### NPC 死亡即时清理原则

NPC 死亡后应立即从 `s.arena.npcs` map 中 `delete`，不要只设 `npc.Dead = true` 等待 `spawnNPCWave`（60 秒周期）批量清理。

延迟清理的问题：
- `npcAITick`（2Hz）每次遍历都要跳过死亡 NPC，浪费 CPU
- 所有引用 `s.arena.npcs` 的地方都需要额外 `npc.Dead` 检查，增加遗漏风险
- `len(s.arena.npcs)` 会把死亡 NPC 算进 `MaxNPCCount` 上限

清理位置（3 处）：
1. `handleAttackNPC` — 普攻杀死时，锁内 `delete(s.arena.npcs, id)`
2. `handleCastSkillNPC` — 技能杀死时，锁内 `delete(s.arena.npcs, id)`
3. `npcAITick` 广播阶段 — 二次检查发现残留死亡 NPC 时兜底删除

前提：掉落（`tryNPCDrop`）和广播（`MsgNPCDied`）在删除前完成，删除后不再需要 NPC 数据。
