package combat

import (
	"math"
	"testing"
)

// ---------- 测试辅助 ----------

func makeNPC(x, y float64) *NPCState {
	return &NPCState{
		ID: -1, X: x, Y: y,
		HP: 100, MaxHP: 100, Attack: 20, Defense: 5,
		SpawnX: x, SpawnY: y,
	}
}

func makeConfig() *NPCAIConfig {
	return &NPCAIConfig{
		AttackRange: 50,
		AggroRange:  150,
		LeashRange:  400,
		AttackCD:    2.0,
		CritRate:    0.0, // 测试中禁用暴击
		CritDmg:     1.5,
		Speed:       60,
	}
}

func makeTarget(id int64, x, y float64) NPCAITarget {
	return NPCAITarget{ID: id, X: x, Y: y, HP: 100, Defense: 5}
}

// neverSafeZone 永远不在安全区
func neverSafeZone(x, y float64) bool { return false }

// alwaysSafeZone 永远在安全区
func alwaysSafeZone(x, y float64) bool { return true }

const (
	arenaMinX = -930.0
	arenaMaxX = 930.0
	arenaMinY = 30.0
	arenaMaxY = 930.0
	dt        = 0.2 // 5Hz tick
)

// ---------- 测试：死亡 NPC 不行动 ----------

func TestComputeNPCAI_DeadNPC(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.Dead = true
	cfg := makeConfig()
	targets := []NPCAITarget{makeTarget(1, 110, 100)}

	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "idle" {
		t.Errorf("死亡 NPC 应返回 idle，实际: %s", action.Type)
	}
}

// ---------- 测试：昏迷状态 ----------

func TestComputeNPCAI_Stunned(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.StunUntil = 5000
	cfg := makeConfig()
	targets := []NPCAITarget{makeTarget(1, 110, 100)}

	// 昏迷中（now < StunUntil）
	action := ComputeNPCAI(npc, cfg, targets, 3000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "idle" {
		t.Errorf("昏迷中应返回 idle，实际: %s", action.Type)
	}
	if action.ClearStun {
		t.Error("昏迷未结束不应清除昏迷标志")
	}

	// 昏迷结束（now >= StunUntil）
	action = ComputeNPCAI(npc, cfg, targets, 5000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if !action.ClearStun {
		t.Error("昏迷结束应设置 ClearStun=true")
	}
}

// ---------- 测试：恐惧逃跑 ----------

func TestComputeNPCAI_FearMove(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.FearUntil = 5000
	npc.FearDirX = 1.0
	npc.FearDirY = 0.0
	cfg := makeConfig()
	targets := []NPCAITarget{makeTarget(1, 90, 100)}

	// 恐惧中
	action := ComputeNPCAI(npc, cfg, targets, 3000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "fear_move" {
		t.Errorf("恐惧中应返回 fear_move，实际: %s", action.Type)
	}
	if action.MoveX <= npc.X {
		t.Errorf("恐惧方向为正 X，MoveX 应大于当前 X: MoveX=%.1f, X=%.1f", action.MoveX, npc.X)
	}
	if action.NewTargetID != 0 {
		t.Error("恐惧中应清除目标")
	}
	if action.MoveDir != "r" {
		t.Errorf("恐惧方向为正 X，方向应为 r，实际: %s", action.MoveDir)
	}
}

func TestComputeNPCAI_FearEnd(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.FearUntil = 3000
	npc.FearDirX = 1.0
	npc.FearDirY = 0.0
	cfg := makeConfig()
	targets := []NPCAITarget{makeTarget(1, 110, 100)}

	// 恐惧结束
	action := ComputeNPCAI(npc, cfg, targets, 3000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.ClearFear != true {
		t.Error("恐惧结束应设置 ClearFear=true")
	}
	if action.ResetSpawn != true {
		t.Error("恐惧结束应重置出生点")
	}
	if action.SetEnragedUntil == 0 {
		t.Error("恐惧结束应设置激怒时间")
	}
}

// ---------- 测试：追踪目标 ----------

func TestComputeNPCAI_Chase(t *testing.T) {
	npc := makeNPC(100, 100)
	cfg := makeConfig()
	cfg.AggroRange = 300
	// 目标在仇恨范围内但超出攻击范围（dx=300, 椭圆距离=150 > AttackRange=50）
	target := makeTarget(1, 400, 100)
	targets := []NPCAITarget{target}

	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "chase" {
		t.Errorf("应追踪目标，实际: %s", action.Type)
	}
	if action.TargetID != target.ID {
		t.Errorf("追踪目标 ID 应为 %d，实际: %d", target.ID, action.TargetID)
	}
	if action.MoveX <= npc.X {
		t.Errorf("追踪方向为正 X，MoveX 应大于当前 X: MoveX=%.1f, X=%.1f", action.MoveX, npc.X)
	}
	if action.MoveDir != "r" {
		t.Errorf("追踪方向为正 X，方向应为 r，实际: %s", action.MoveDir)
	}
}

// ---------- 测试：攻击目标 ----------

func TestComputeNPCAI_Attack(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.LastAttackAt = 0 // 从未攻击过
	cfg := makeConfig()
	cfg.AttackCD = 2.0
	// 目标在攻击范围内（dx=30, 椭圆距离=15 <= AttackRange=50）
	target := makeTarget(1, 130, 100)
	targets := []NPCAITarget{target}

	// now=5000, lastAttack=0, cd=2000ms → 5000-0=5000 >= 2000 → 可攻击
	action := ComputeNPCAI(npc, cfg, targets, 5000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "attack" {
		t.Errorf("应攻击目标，实际: %s", action.Type)
	}
	if action.TargetID != target.ID {
		t.Errorf("攻击目标 ID 应为 %d，实际: %d", target.ID, action.TargetID)
	}
	if action.Damage <= 0 {
		t.Errorf("攻击伤害应大于 0，实际: %.1f", action.Damage)
	}
	if action.NewLastAttackAt != 5000 {
		t.Errorf("应更新 LastAttackAt 为 5000，实际: %d", action.NewLastAttackAt)
	}
}

func TestComputeNPCAI_AttackCooldown(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.LastAttackAt = 500 // 上次攻击在 500ms
	cfg := makeConfig()
	cfg.AttackCD = 2.0 // 冷却 2 秒 = 2000ms
	target := makeTarget(1, 130, 100)
	targets := []NPCAITarget{target}

	// 冷却中（now=1000, lastAttack=500, cd=2000ms, 1000-500=500 < 2000）
	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "idle" {
		t.Errorf("攻击冷却中应返回 idle，实际: %s", action.Type)
	}

	// 冷却结束（now=3000, 3000-500=2500 >= 2000）
	action = ComputeNPCAI(npc, cfg, targets, 3000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "attack" {
		t.Errorf("冷却结束应攻击，实际: %s", action.Type)
	}
}

// ---------- 测试：脱战回归 ----------

func TestComputeNPCAI_ReturnToSpawn(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.SpawnX = 300
	npc.SpawnY = 300
	cfg := makeConfig()
	// 目标超出仇恨范围
	target := makeTarget(1, 800, 800)
	targets := []NPCAITarget{target}

	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "return" {
		t.Errorf("超出仇恨范围应回归，实际: %s", action.Type)
	}
	if action.NewTargetID != 0 {
		t.Error("回归时应清除目标")
	}
	// 应朝出生点移动
	dxBefore := npc.SpawnX - npc.X
	dyBefore := npc.SpawnY - npc.Y
	dxAfter := npc.SpawnX - action.MoveX
	dyAfter := npc.SpawnY - action.MoveY
	distBefore := math.Sqrt(dxBefore*dxBefore + dyBefore*dyBefore)
	distAfter := math.Sqrt(dxAfter*dxAfter + dyAfter*dyAfter)
	if distAfter >= distBefore {
		t.Errorf("回归后应更接近出生点: before=%.1f, after=%.1f", distBefore, distAfter)
	}
}

func TestComputeNPCAI_ReturnHeal(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.HP = 50 // 半血
	npc.SpawnX = 300
	npc.SpawnY = 300
	cfg := makeConfig()
	targets := []NPCAITarget{} // 无目标

	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "return" {
		t.Errorf("无目标且远离出生点应回归，实际: %s", action.Type)
	}
	if action.HealAmount <= 0 {
		t.Error("回归时半血 NPC 应回血")
	}
}

// ---------- 测试：安全区回避 ----------

func TestComputeNPCAI_SafeZoneAvoidChase(t *testing.T) {
	npc := makeNPC(100, 100)
	cfg := makeConfig()
	target := makeTarget(1, 200, 100)
	targets := []NPCAITarget{target}

	// 追踪时新位置在安全区内 → 停止
	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, alwaysSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "idle" {
		t.Errorf("追踪进入安全区应停止，实际: %s", action.Type)
	}
}

func TestComputeNPCAI_SafeZoneAvoidFear(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.FearUntil = 5000
	npc.FearDirX = 1.0
	npc.FearDirY = 0.0
	cfg := makeConfig()
	targets := []NPCAITarget{}

	// 恐惧移动时新位置在安全区内 → 停在原地
	action := ComputeNPCAI(npc, cfg, targets, 3000, dt, alwaysSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "fear_move" {
		t.Errorf("恐惧中应返回 fear_move，实际: %s", action.Type)
	}
	if action.MoveX != npc.X || action.MoveY != npc.Y {
		t.Errorf("安全区内恐惧移动应停在原地: MoveX=%.1f, MoveY=%.1f, X=%.1f, Y=%.1f",
			action.MoveX, action.MoveY, npc.X, npc.Y)
	}
}

// ---------- 测试：无目标时 idle ----------

func TestComputeNPCAI_NoTargets_AtSpawn(t *testing.T) {
	npc := makeNPC(100, 100)
	cfg := makeConfig()
	targets := []NPCAITarget{}

	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type != "idle" {
		t.Errorf("无目标且在出生点应 idle，实际: %s", action.Type)
	}
}

// ---------- 测试：激怒状态无视仇恨范围 ----------

func TestComputeNPCAI_EnragedIgnoresAggroRange(t *testing.T) {
	npc := makeNPC(100, 100)
	npc.EnragedUntil = 10000
	cfg := makeConfig()
	cfg.AggroRange = 50 // 很小的仇恨范围
	// 目标远超仇恨范围但激怒状态下仍追踪
	target := makeTarget(1, 200, 100)
	targets := []NPCAITarget{target}

	action := ComputeNPCAI(npc, cfg, targets, 5000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.Type == "return" || action.Type == "idle" {
		t.Errorf("激怒状态应无视仇恨范围追踪目标，实际: %s", action.Type)
	}
}

// ---------- 测试：选择最近目标 ----------

func TestComputeNPCAI_SelectClosestTarget(t *testing.T) {
	npc := makeNPC(100, 100)
	cfg := makeConfig()
	cfg.AggroRange = 500
	targets := []NPCAITarget{
		makeTarget(1, 300, 100), // 较远
		makeTarget(2, 130, 100), // 较近
	}

	action := ComputeNPCAI(npc, cfg, targets, 1000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.NewTargetID != 2 {
		t.Errorf("应选择最近目标 ID=2，实际: %d", action.NewTargetID)
	}
}

// ---------- 测试：ellipseDistance ----------

func TestEllipseDistance(t *testing.T) {
	// 纯水平距离：dx=100, dy=0 → sqrt(100²+0²)/2 = 50
	d := ellipseDistance(0, 0, 100, 0)
	if math.Abs(d-50) > 0.01 {
		t.Errorf("水平距离应为 50，实际: %.2f", d)
	}

	// 纯垂直距离：dx=0, dy=50 → sqrt(0²+100²)/2 = 50
	d = ellipseDistance(0, 0, 0, 50)
	if math.Abs(d-50) > 0.01 {
		t.Errorf("垂直距离应为 50，实际: %.2f", d)
	}
}

// ---------- 测试：边界限制 ----------

func TestComputeNPCAI_BoundaryClamp(t *testing.T) {
	// NPC 在边界附近，恐惧方向朝边界外
	npc := makeNPC(arenaMaxX-5, 100)
	npc.FearUntil = 5000
	npc.FearDirX = 1.0 // 朝右边界
	npc.FearDirY = 0.0
	cfg := makeConfig()
	cfg.Speed = 1000 // 很快的速度确保超出边界
	targets := []NPCAITarget{}

	action := ComputeNPCAI(npc, cfg, targets, 3000, dt, neverSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)
	if action.MoveX > arenaMaxX {
		t.Errorf("移动后 X 不应超出边界: MoveX=%.1f, max=%.1f", action.MoveX, arenaMaxX)
	}
}
