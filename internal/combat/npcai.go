package combat

import (
	"math"
	"math/rand"
)

// ---------- NPC AI 框架 ----------
// 提供通用 NPC AI 决策纯函数，不持有状态，不修改输入，只返回决策结果。
// demo 层负责：收集目标列表、调用 ComputeNPCAI、根据返回的 NPCAIAction 执行副作用。

// NPCAIConfig NPC AI 配置（不可变参数）
type NPCAIConfig struct {
	AttackRange float64 // 攻击范围（2.5D 椭圆距离）
	AggroRange  float64 // 仇恨范围（主动攻击距离）
	LeashRange  float64 // 脱战回归距离（未使用，保留扩展）
	AttackCD    float64 // 攻击冷却（秒）
	CritRate    float64 // 暴击率
	CritDmg     float64 // 暴击倍率
	Speed       float64 // 移动速度
}

// NPCState NPC 运行时状态（由调用方维护，ComputeNPCAI 只读不写）
type NPCState struct {
	ID           int64
	X, Y         float64
	HP, MaxHP    float64
	Attack       float64
	Defense      float64
	Dead         bool
	TargetID     int64
	SpawnX       float64
	SpawnY       float64
	LastAttackAt int64 // 上次攻击时间（UnixMilli）

	// 状态效果
	FearUntil    int64   // 恐惧结束时间（UnixMilli），0 表示无恐惧
	FearDirX     float64 // 恐惧逃跑方向 X
	FearDirY     float64 // 恐惧逃跑方向 Y
	StunUntil    int64   // 昏迷结束时间（UnixMilli），0 表示无昏迷
	EnragedUntil int64   // 激怒结束时间（UnixMilli），0 表示无激怒
}

// NPCAITarget 潜在攻击目标（由调用方从玩家列表中构建）
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
	TargetID int64   // 攻击/追踪目标 ID
	MoveX    float64 // 移动后的新 X 坐标
	MoveY    float64 // 移动后的新 Y 坐标
	MoveDir  string  // 移动方向 "u"/"d"/"l"/"r"
	Damage   float64 // 攻击伤害值
	IsCrit   bool    // 是否暴击

	// 状态更新提示（调用方根据这些字段更新 NPC 状态）
	NewTargetID     int64 // 新的目标 ID（0 表示清除目标）
	NewLastAttackAt int64 // 新的上次攻击时间（0 表示不更新）

	// 恐惧结束时的状态重置
	ClearFear       bool    // 是否清除恐惧状态
	ClearStun       bool    // 是否清除昏迷状态
	ClearEnraged    bool    // 是否清除激怒状态
	SetEnragedUntil int64   // 设置激怒结束时间（0 表示不设置）
	ResetSpawn      bool    // 是否将出生点重置为当前位置

	// 脱战回归时的回血
	HealAmount float64 // 回血量（0 表示不回血）
}

// ComputeNPCAI 纯函数：根据 NPC 状态和目标列表计算 AI 行为。
// 不修改任何输入状态，只返回决策结果。
//
// 参数：
//   - npc: NPC 当前状态（只读）
//   - config: NPC AI 配置（只读）
//   - targets: 可攻击目标列表（已过滤死亡和安全区内的玩家）
//   - nowMs: 当前时间（UnixMilli）
//   - dt: 本次 tick 的时间间隔（秒）
//   - isInSafeZone: 安全区判定函数（demo 特有配置）
//   - arenaMinX, arenaMaxX, arenaMinY, arenaMaxY: 竞技场边界
func ComputeNPCAI(
	npc *NPCState,
	config *NPCAIConfig,
	targets []NPCAITarget,
	nowMs int64,
	dt float64,
	isInSafeZone func(x, y float64) bool,
	arenaMinX, arenaMaxX, arenaMinY, arenaMaxY float64,
) NPCAIAction {
	action := NPCAIAction{
		Type:        "idle",
		MoveX:       npc.X,
		MoveY:       npc.Y,
		NewTargetID: npc.TargetID,
	}

	if npc.Dead {
		return action
	}

	// 0. 检查昏迷状态
	if npc.StunUntil > 0 && nowMs < npc.StunUntil {
		return action // 昏迷中：无法行动
	}
	if npc.StunUntil > 0 && nowMs >= npc.StunUntil {
		action.ClearStun = true
	}

	// 1. 检查恐惧状态
	if npc.FearUntil > 0 && nowMs < npc.FearUntil {
		return computeFearMove(npc, config, dt, isInSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	}
	// 恐惧结束，清除状态
	if npc.FearUntil > 0 && nowMs >= npc.FearUntil {
		action.ClearFear = true
		action.ResetSpawn = true
		action.SetEnragedUntil = nowMs + 30000 // 恐惧结束后激怒 30 秒
	}

	// 2. 寻找最近的存活目标（2.5D 椭圆距离）
	closestIdx := -1
	closestDist := math.MaxFloat64
	for i, t := range targets {
		if t.Dead {
			continue
		}
		d2d := ellipseDistance(npc.X, npc.Y, t.X, t.Y)
		if d2d < closestDist {
			closestDist = d2d
			closestIdx = i
		}
	}
	if closestIdx < 0 {
		// 无目标，检查是否需要回归
		action.NewTargetID = 0
		return computeReturnToSpawn(npc, config, dt, isInSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY, action)
	}
	closest := targets[closestIdx]

	// 3. 检查是否在仇恨范围内（激怒状态下无视仇恨范围）
	isEnraged := npc.EnragedUntil > 0 && nowMs < npc.EnragedUntil
	if npc.EnragedUntil > 0 && nowMs >= npc.EnragedUntil {
		action.ClearEnraged = true
	}
	if !isEnraged && closestDist > config.AggroRange {
		// 超出仇恨范围，回归出生点
		action.NewTargetID = 0
		return computeReturnToSpawn(npc, config, dt, isInSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY, action)
	}

	// 4. 锁定目标
	action.NewTargetID = closest.ID

	// 5. 在攻击范围内则攻击，否则追踪
	if closestDist <= config.AttackRange {
		return computeAttack(npc, config, closest, nowMs, action)
	}
	return computeChase(npc, config, closest, dt, isInSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY, action)
}

// ---------- 内部辅助函数 ----------

// ellipseDistance 计算 2.5D 椭圆距离（与前端判定一致）。
// Y 轴压缩 0.5，等效距离 = sqrt(dx² + (dy*2)²) / 2
func ellipseDistance(x1, y1, x2, y2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	return math.Sqrt(dx*dx+(dy*2)*(dy*2)) / 2
}

// computeDirection 根据移动方向计算朝向字符串
func computeDirection(dx, dy float64) string {
	if math.Abs(dx) > math.Abs(dy) {
		if dx > 0 {
			return "r"
		}
		return "l"
	}
	if dy > 0 {
		return "d"
	}
	return "u"
}

// clampPosition 将坐标限制在竞技场边界内
func clampPosition(x, y, minX, maxX, minY, maxY float64) (float64, float64) {
	x = math.Max(minX, math.Min(maxX, x))
	y = math.Max(minY, math.Min(maxY, y))
	return x, y
}

// computeFearMove 计算恐惧状态下的移动
func computeFearMove(
	npc *NPCState, config *NPCAIConfig, dt float64,
	isInSafeZone func(x, y float64) bool,
	arenaMinX, arenaMaxX, arenaMinY, arenaMaxY float64,
) NPCAIAction {
	moveSpeed := config.Speed * dt * 1.5 // 恐惧时跑得更快
	newX := npc.X + npc.FearDirX*moveSpeed
	newY := npc.Y + npc.FearDirY*moveSpeed

	action := NPCAIAction{
		Type:        "fear_move",
		NewTargetID: 0,
	}

	// NPC 不能进入安全区
	if isInSafeZone != nil && !isInSafeZone(newX, newY) {
		action.MoveX = newX
		action.MoveY = newY
	} else if isInSafeZone != nil && isInSafeZone(newX, newY) {
		// 停在原地
		action.MoveX = npc.X
		action.MoveY = npc.Y
	} else {
		action.MoveX = newX
		action.MoveY = newY
	}

	// 边界限制
	action.MoveX, action.MoveY = clampPosition(action.MoveX, action.MoveY, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	action.MoveDir = computeDirection(npc.FearDirX, npc.FearDirY)
	return action
}

// computeReturnToSpawn 计算脱战回归出生点的行为
func computeReturnToSpawn(
	npc *NPCState, config *NPCAIConfig, dt float64,
	isInSafeZone func(x, y float64) bool,
	arenaMinX, arenaMaxX, arenaMinY, arenaMaxY float64,
	action NPCAIAction,
) NPCAIAction {
	spawnDist := Distance(npc.X, npc.Y, npc.SpawnX, npc.SpawnY)
	if spawnDist <= 5 {
		action.Type = "idle"
		return action
	}

	// 缓慢回归出生点
	dx := npc.SpawnX - npc.X
	dy := npc.SpawnY - npc.Y
	d := math.Sqrt(dx*dx + dy*dy)
	moveSpeed := config.Speed * dt * 0.5 // 回归速度减半
	if moveSpeed > d {
		moveSpeed = d
	}
	newX := npc.X + (dx/d)*moveSpeed
	newY := npc.Y + (dy/d)*moveSpeed

	action.Type = "return"

	// NPC 不能进入安全区
	if isInSafeZone != nil && !isInSafeZone(newX, newY) {
		action.MoveX = newX
		action.MoveY = newY
	} else {
		action.MoveX = npc.X
		action.MoveY = npc.Y
	}

	action.MoveDir = computeDirection(dx, dy)

	// 回归时回血
	if npc.HP < npc.MaxHP {
		action.HealAmount = npc.MaxHP * 0.05 * dt
		if npc.HP+action.HealAmount > npc.MaxHP {
			action.HealAmount = npc.MaxHP - npc.HP
		}
	}

	return action
}

// computeAttack 计算攻击行为
func computeAttack(
	npc *NPCState, config *NPCAIConfig, target NPCAITarget, nowMs int64,
	action NPCAIAction,
) NPCAIAction {
	// 检查攻击冷却
	cdMs := int64(config.AttackCD * 1000)
	if nowMs-npc.LastAttackAt < cdMs {
		action.Type = "idle"
		return action
	}

	// 计算伤害
	dmg := CalcDamage(npc.Attack, target.Defense)
	isCrit := rand.Float64() < config.CritRate
	if isCrit {
		dmg *= config.CritDmg
	}
	dmg = math.Round(dmg)

	action.Type = "attack"
	action.TargetID = target.ID
	action.Damage = dmg
	action.IsCrit = isCrit
	action.NewLastAttackAt = nowMs
	return action
}

// computeChase 计算追踪移动行为
func computeChase(
	npc *NPCState, config *NPCAIConfig, target NPCAITarget, dt float64,
	isInSafeZone func(x, y float64) bool,
	arenaMinX, arenaMaxX, arenaMinY, arenaMaxY float64,
	action NPCAIAction,
) NPCAIAction {
	dx := target.X - npc.X
	dy := target.Y - npc.Y
	d := math.Sqrt(dx*dx + dy*dy) // 实际移动用欧几里得距离
	moveSpeed := config.Speed * dt
	stopDist := config.AttackRange * 0.8
	if moveSpeed > d-stopDist {
		moveSpeed = d - stopDist
	}
	if moveSpeed <= 0 {
		action.Type = "idle"
		return action
	}

	newX := npc.X + (dx/d)*moveSpeed
	newY := npc.Y + (dy/d)*moveSpeed

	action.Type = "chase"
	action.TargetID = target.ID

	// NPC 不能进入安全区
	if isInSafeZone != nil && !isInSafeZone(newX, newY) {
		action.MoveX = newX
		action.MoveY = newY
	} else {
		// 停在安全区边界外
		action.Type = "idle"
		action.MoveX = npc.X
		action.MoveY = npc.Y
		return action
	}

	action.MoveDir = computeDirection(dx, dy)
	return action
}
