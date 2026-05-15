# Implementation Tasks — engine-extraction-refactor

## 后端重构

- [x] 1. 删除后端重复的范围判定/伤害计算包装函数
  - [x] 1.1 删除 `arena.go` 中的 `distance()`、`isInEllipseRange()`、`isInFanRange()`、`calcDamage()` 包装函数
  - [x] 1.2 将所有调用点改为直接调用 `combat.Distance()`、`combat.IsInEllipseRange()`、`combat.IsInFanRange()`、`combat.CalcDamage()`
  - [x] 1.3 保留 `isInSafeZone()` 作为 demo 层便捷函数（绑定 `CampfireX/Y/Radius` 常量）
  - [x] 1.4 运行 `go build ./cmd/demo/...` 确认编译通过

- [x] 2. 新增通用安全区判定工具
  - [x] 2.1 在 `internal/combat/range.go` 中新增 `ZoneConfig` 结构体和 `IsInZone()` 函数，支持 ellipse/circle/rect 三种形状
  - [x] 2.2 在 `internal/combat/combat_test.go` 或 `range_test.go` 中新增 `IsInZone` 的单元测试
  - [x] 2.3 将 `arena.go` 的 `isInSafeZone()` 改为调用 `combat.IsInZone()`
  - [x] 2.4 运行 `go test ./internal/combat/... -count=1` 确认测试通过

- [x] 3. 新增 JSON 消息路由器
  - [x] 3.1 在 `internal/network/` 新增 `router.go`，实现 `MessageRouter` 结构体（`Register`、`Dispatch` 方法）
  - [x] 3.2 新增 `internal/network/router_test.go`，测试消息注册、分发、未知消息处理
  - [x] 3.3 修改 `cmd/demo/main.go`，将 `handleMessage` 的 switch 改为 `MessageRouter` 注册模式
  - [x] 3.4 运行 `go test ./internal/network/... -count=1` 和 `go build ./cmd/demo/...` 确认通过

- [x] 4. 新增房间广播接口
  - [x] 4.1 在 `internal/room/` 新增 `broadcast.go`，定义 `Broadcaster` 接口（`Broadcast`、`BroadcastAll` 方法）
  - [x] 4.2 修改 `cmd/demo/main.go` 的 `Arena` 结构体，使其实现 `Broadcaster` 接口
  - [x] 4.3 运行 `go build ./cmd/demo/...` 确认编译通过

- [x] 5. 新增 NPC AI 框架
  - [x] 5.1 在 `internal/combat/` 新增 `npcai.go`，实现 `NPCAIConfig`、`NPCState`、`NPCAITarget`、`NPCAIAction` 类型和 `ComputeNPCAI()` 纯函数
  - [x] 5.2 新增 `internal/combat/npcai_test.go`，测试追踪、攻击、脱战回归、恐惧逃跑、昏迷、安全区回避等 AI 行为分支
  - [x] 5.3 修改 `cmd/demo/arena.go` 的 `npcAITick()`，将 AI 决策逻辑替换为调用 `combat.ComputeNPCAI()`，保留副作用执行（HP 修改、广播）在 demo 层
  - [x] 5.4 运行 `go test ./internal/combat/... -count=1` 和 `go build ./cmd/demo/...` 确认通过

- [ ] 6. 适配 GameLoop 替代手动 ticker
  - [ ] 6.1 修改 `cmd/demo/arena.go` 的 `arenaTick()`，创建 `engine.GameLoop` 实例替代手动 `time.NewTicker` + `select`
  - [ ] 6.2 将 NPC AI、状态同步、NPC 刷新、篝火倒计时注册为 GameLoop 的不同阶段 handler
  - [ ] 6.3 运行 `go build ./cmd/demo/...` 确认编译通过

## 前端重构

- [ ] 7. 联网死亡/复活系统提取到 BaseGameScene
  - [ ] 7.1 在 `src/prologue/scenes/BaseGameScene.js` 中新增 `handleNetworkDeath(entity, data)` 方法，包含双标志同步、交互状态清理、视觉效果，带 `_arenaMode` 守卫
  - [ ] 7.2 在 `BaseGameScene` 中新增 `handleNetworkRespawn(entity, data)` 方法，包含标志重置、属性恢复、位置更新，带 `_arenaMode` 守卫
  - [ ] 7.3 修改 `ArenaScene.js` 的 `onPlayerDied()` 和 `onPlayerRespawn()`，改为调用 `super.handleNetworkDeath()` / `super.handleNetworkRespawn()` + demo 特有逻辑（灵魂 overlay、击杀文字、粒子效果）

- [ ] 8. 新增前端安全区系统
  - [ ] 8.1 新增 `src/systems/SafeZoneSystem.js`，实现 `addZone(config)`、`isInAnyZone(x, y)` 方法，支持 ellipse 形状的 2.5D 椭圆判定
  - [ ] 8.2 修改 `ArenaScene.js`，在 `enter()` 中创建 `SafeZoneSystem` 并添加篝火安全区，将 `isInSafeZone()` 调用改为委托 `SafeZoneSystem`

- [ ] 9. GroundDropPickupSystem 完整封装拾取逻辑
  - [ ] 9.1 将 `ArenaScene.js` 的 `update()` 中内联的掉落物拾取逻辑（按键检测、范围判定、批量拾取、浮动文字）移入 `GroundDropPickupSystem.update()`
  - [ ] 9.2 修改 `ArenaScene.js` 的 `update()`，删除内联拾取逻辑，改为调用 `this.groundDropPickupSystem.update(deltaTime)`

- [ ] 10. 新增状态效果移动控制系统
  - [ ] 10.1 新增 `src/systems/StatusEffectMovementSystem.js`，实现恐惧自动移动（`_applyFearMovement`）和昏迷状态检查
  - [ ] 10.2 修改 `ArenaScene.js` 的 `update()`，将恐惧移动逻辑替换为 `this.statusEffectMovementSystem.update(deltaTime)` 调用
