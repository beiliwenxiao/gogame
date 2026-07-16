/*************************************************************
 * Copyright (c) 2026 Liu Xiao (beiliwenxiao)
 *
 * @project   YiJian18-Server 多人实时战斗游戏后端引擎
 * @author    刘枭 (beiliwenxiao)
 * @email     beiliwenxiao@qq.com
 * @date      2026-03-01
 * @blog      https://blog.csdn.net/beiliwenxiao
 * @repo      https://github.com/beiliwenxiao/yijian18-server
 *            https://gitee.com/coderaaa/yijian18-server
 *************************************************************/

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"yijian18-server/internal/combat"
	"yijian18-server/internal/engine"
)

// SkillInfo 技能运行时信息
type SkillInfo struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Class    string  `json:"class"`
	Damage   float64 `json:"damage"`
	MPCost   float64 `json:"mp_cost"`
	Cooldown float64 `json:"cooldown"`
	Range    float64 `json:"range"`
	AreaType string  `json:"area_type"`
	AreaSize float64 `json:"area_size"`
}

// safeZone 篝火安全区配置（2.5D 椭圆）
var safeZone = combat.ZoneConfig{
	CenterX: CampfireX, CenterY: CampfireY,
	Radius: CampfireRadius, Shape: "ellipse",
}

// isInSafeZone 判断坐标是否在篝火安全区内（2.5D 椭圆）
// 保留为 demo 层便捷函数，绑定 CampfireX/Y/Radius 常量
func isInSafeZone(x, y float64) bool {
	return combat.IsInZone(&safeZone, x, y)
}

func (s *DemoServer) makePlayerState(p *PlayerSession) map[string]interface{} {
	return map[string]interface{}{
		"char_id": p.charID, "name": p.charName, "class": p.charClass,
		"x": p.x, "y": p.y, "hp": p.hp, "max_hp": p.maxHP,
		"mp": p.mp, "max_mp": p.maxMP, "attack": p.attack,
		"defense": p.defense, "speed": p.speed, "level": p.level,
		"dead": p.dead, "direction": p.direction,
		"crit_rate": p.critRate, "crit_damage": p.critDmg,
	}
}

func (s *DemoServer) getArenaPlayersState() []map[string]interface{} {
	s.arena.mu.RLock()
	defer s.arena.mu.RUnlock()
	result := make([]map[string]interface{}, 0, len(s.arena.players))
	for _, p := range s.arena.players {
		result = append(result, s.makePlayerState(p))
	}
	return result
}


func (s *DemoServer) handleEnterArena(session *PlayerSession) {
	if session.charID == 0 {
		session.Send(ServerMessage{Type: MsgError, Data: "请先创建角色"})
		return
	}
	ch, err := s.db.GetCharacterByID(session.charID)
	if err != nil || ch == nil {
		session.Send(ServerMessage{Type: MsgError, Data: "角色数据异常"})
		return
	}
	total, _ := s.db.CalcTotalStats(ch)
	angle := rand.Float64() * 2 * math.Pi
	dist := 50.0 + rand.Float64()*100.0
	session.x = math.Max(-ArenaWidth+30, math.Min(ArenaWidth-30, CampfireX+math.Cos(angle)*dist))
	session.y = math.Max(30, math.Min(ArenaHeight-30, CampfireY+math.Sin(angle)*dist))
	session.hp = total.MaxHP
	session.maxHP = total.MaxHP
	session.mp = total.MaxMP
	session.maxMP = total.MaxMP
	session.attack = total.Attack
	session.defense = total.Defense
	session.speed = total.Speed
	session.level = total.Level
	session.critRate = total.CritRate
	session.critDmg = 1.5 // 默认暴击倍率，对齐后端 CombatAttributeComponent.CritDamage
	session.dead = false
	session.inArena = true

	// 从装备中读取武器攻击范围
	equips, _ := s.db.GetCharEquipments(session.charID)
	session.weaponAttackRange = 60.0  // 默认近战扇形角度（度数）
	session.weaponAttackDist = 100.0  // 默认近战距离（像素）
	session.weaponIsRanged = false
	if session.charClass == "archer" {
		session.weaponAttackRange = 30.0  // 弓箭手默认扇形角度（度数）
		session.weaponAttackDist = 250.0  // 弓箭手默认攻击距离（像素）
		session.weaponIsRanged = true
	}
	for _, eq := range equips {
		if eq.SlotType == "weapon" && eq.Def.AttackRange > 0 {
			session.weaponAttackRange = eq.Def.AttackRange
			session.weaponAttackDist = eq.Def.AttackDistance
			// 有 pierce 或 multi_arrow 效果的武器视为远程
			if eq.Def.Class == "archer" {
				session.weaponIsRanged = true
			}
		}
	}

	s.arena.mu.Lock()
	s.arena.players[session.charID] = session
	s.arena.mu.Unlock()
	skills, _ := s.db.GetSkillsByClass(session.charClass)
	players := s.getArenaPlayersState()
	inventory, _ := s.db.GetCharInventory(session.charID)
	session.Send(ServerMessage{Type: MsgArenaState, Data: map[string]interface{}{
		"players": players, "self_id": session.charID,
		"campfire": map[string]interface{}{"x": CampfireX, "y": CampfireY, "radius": CampfireRadius},
		"arena":    map[string]float64{"width": ArenaWidth, "height": ArenaHeight},
		"skills":   skills,
		"equipments": equips,
		"npcs":     s.getAllNPCStates(),
		"inventory": inventory,
	}})
	// 单独再发一次背包（兼容旧客户端）
	session.Send(ServerMessage{Type: MsgInventory, Data: inventory})
	s.arena.Broadcast(ServerMessage{Type: MsgPlayerJoined, Data: s.makePlayerState(session)}, session.charID)
	log.Printf("玩家 %s(%s) 进入修罗斗场", session.charName, session.charClass)
}

func (s *DemoServer) handleLeaveArena(session *PlayerSession) {
	if !session.inArena {
		return
	}
	session.inArena = false
	s.arena.mu.Lock()
	delete(s.arena.players, session.charID)
	s.arena.mu.Unlock()
	s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerLeft, Data: map[string]interface{}{
		"char_id": session.charID, "name": session.charName,
	}})
}

func (s *DemoServer) handleMove(session *PlayerSession, data json.RawMessage) {
	if !session.inArena {
		return
	}
	var req struct {
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Direction string  `json:"direction"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}
	session.x = math.Round(math.Max(-ArenaWidth+10, math.Min(ArenaWidth-10, req.X))*10) / 10
	session.y = math.Round(math.Max(-ArenaHeight+10, math.Min(ArenaHeight-10, req.Y))*10) / 10
	if req.Direction != "" {
		session.direction = req.Direction
	}
	// 死亡（灵魂状态）玩家也广播位置，让其他人看到灵魂移动
	s.arena.Broadcast(ServerMessage{Type: MsgPlayerMoved, Data: map[string]interface{}{
		"char_id": session.charID, "x": session.x, "y": session.y, "direction": session.direction,
	}}, session.charID)
}


func (s *DemoServer) handleAttack(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
		return
	}
	var req struct {
		TargetID int64 `json:"target_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}
	s.arena.mu.RLock()
	target, ok := s.arena.players[req.TargetID]
	s.arena.mu.RUnlock()
	if !ok || target.dead {
		return
	}
	// 安全区检查：攻击者在安全区内不能攻击
	if isInSafeZone(session.x, session.y) {
		session.Send(ServerMessage{Type: MsgError, Data: "安全区内禁止攻击"})
		return
	}
	maxDist := session.weaponAttackDist
	if maxDist <= 0 {
		maxDist = 100.0
	}
	if !combat.IsInEllipseRange(session.x, session.y, target.x, target.y, maxDist) {
		session.Send(ServerMessage{Type: MsgError, Data: "超出攻击范围"})
		return
	}
	damage := combat.CalcDamage(session.attack, target.defense)
	damage *= 0.5 // PVP 伤害减半
	isCrit := rand.Float64() < session.critRate
	if isCrit {
		damage *= session.critDmg
	}
	damage = math.Round(damage)
	target.hp -= damage
	if target.hp < 0 {
		target.hp = 0
	}
	s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
		"attacker_id": session.charID, "target_id": req.TargetID,
		"damage": damage, "is_crit": isCrit,
		"target_hp": target.hp, "target_max_hp": target.maxHP,
		"attacker_class": session.charClass,
	}})
	if target.hp <= 0 {
		target.dead = true
		s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
			"char_id": req.TargetID, "name": target.charName,
			"killer_id": session.charID, "killer": session.charName,
		}})
	}
}


func (s *DemoServer) handleCastSkill(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
		return
	}
	// 安全区内禁止使用技能
	if isInSafeZone(session.x, session.y) {
		session.Send(ServerMessage{Type: MsgError, Data: "安全区内禁止使用技能"})
		return
	}
	var req struct {
		SkillID  int64   `json:"skill_id"`
		TargetID int64   `json:"target_id"`
		TargetX  float64 `json:"target_x"`
		TargetY  float64 `json:"target_y"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}
	skills, _ := s.db.GetSkillsByClass(session.charClass)
	var skill *SkillInfo
	for _, sk := range skills {
		if sk.ID == req.SkillID {
			skill = &SkillInfo{
				ID: sk.ID, Name: sk.Name, Class: sk.Class,
				Damage: sk.Damage, MPCost: sk.MPCost, Cooldown: sk.Cooldown,
				Range: sk.Range, AreaType: sk.AreaType, AreaSize: sk.AreaSize,
			}
			break
		}
	}
	if skill == nil {
		session.Send(ServerMessage{Type: MsgError, Data: "技能不存在"})
		return
	}
	if session.mp < skill.MPCost {
		session.Send(ServerMessage{Type: MsgError, Data: "MP不足"})
		return
	}
	session.mp -= skill.MPCost

	// 猛击：扇形范围（与普攻一致），动态设置范围
	if skill.Name == "猛击" {
		skill.Range = session.weaponAttackDist
		if skill.Range <= 0 {
			skill.Range = 100.0
		}
	}
	// 旋风斩：椭圆范围 = 武器距离
	if skill.Name == "旋风斩" && session.weaponAttackDist > 0 {
		skill.AreaSize = session.weaponAttackDist
	}
	// 战吼：椭圆范围 = 武器距离 × 3
	if skill.Name == "战吼" {
		skill.AreaSize = session.weaponAttackDist * 3
		if skill.AreaSize <= 0 {
			skill.AreaSize = 300.0
		}
	}

	s.arena.BroadcastAll(ServerMessage{Type: MsgSkillCasted, Data: map[string]interface{}{
		"caster_id": session.charID, "skill_id": skill.ID, "skill_name": skill.Name,
		"target_x": req.TargetX, "target_y": req.TargetY,
		"area_type": skill.AreaType, "area_size": skill.AreaSize,
		"caster_x": session.x, "caster_y": session.y,
		"caster_mp": session.mp, "caster_max_mp": session.maxMP,
	}})
	s.arena.mu.RLock()
	var targets []*PlayerSession
	for id, p := range s.arena.players {
		if id == session.charID || p.dead {
			continue
		}
		hit := false
		switch skill.AreaType {
		case "single":
			hit = id == req.TargetID && combat.IsInEllipseRange(session.x, session.y, p.x, p.y, skill.Range)
		case "fan":
			// 扇形：以施法者为中心，朝目标方向，半角 45°（π/4）
			dir := math.Atan2((req.TargetY-session.y)*2, req.TargetX-session.x)
			hit = combat.IsInFanRange(session.x, session.y, p.x, p.y, skill.Range, dir, math.Pi/4)
		case "ellipse":
			hit = combat.IsInEllipseRange(session.x, session.y, p.x, p.y, skill.AreaSize)
		case "circle":
			hit = combat.Distance(req.TargetX, req.TargetY, p.x, p.y) <= skill.AreaSize
		}
		if hit {
			targets = append(targets, p)
		}
	}
	s.arena.mu.RUnlock()

	// 旋风斩：设置持续状态（session 是当前玩家，无需 arena 锁）
	if skill.Name == "旋风斩" {
		now := time.Now().UnixMilli()
		session.WhirlwindUntil = now + 5000
		session.WhirlwindNextTick = now + 1000
		session.WhirlwindDamage = session.attack * skill.Damage * 0.5
		session.WhirlwindAreaSize = skill.AreaSize
	}

	for _, t := range targets {
		// 战吼：造成 30% 伤害 + 恐惧逃跑 3 秒
		if skill.Name == "战吼" {
			// 计算伤害
			dmg := math.Round(combat.CalcDamage(session.attack*skill.Damage, t.defense) * 0.5) // PVP 减半
			if dmg < 1 {
				dmg = 1
			}
			isCrit := rand.Float64() < (session.critRate + 0.05)
			if isCrit {
				dmg = math.Round(dmg * session.critDmg)
			}
			t.hp -= dmg
			if t.hp < 0 {
				t.hp = 0
			}
			s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
				"attacker_id": session.charID, "target_id": t.charID,
				"damage": dmg, "is_crit": isCrit,
				"target_hp": t.hp, "target_max_hp": t.maxHP, "skill_name": skill.Name,
			}})
			if t.hp <= 0 {
				t.dead = true
				s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
					"char_id": t.charID, "name": t.charName,
					"killer_id": session.charID, "killer": session.charName,
				}})
				continue
			}
			// 恐惧效果：让目标远离施法者逃跑 3 秒
			dx := t.x - session.x
			dy := t.y - session.y
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > 0 {
				t.FearDirX = dx / dist
				t.FearDirY = dy / dist
			} else {
				angle := rand.Float64() * math.Pi * 2
				t.FearDirX = math.Cos(angle)
				t.FearDirY = math.Sin(angle)
			}
			t.FearUntil = time.Now().UnixMilli() + 3000
			s.arena.BroadcastAll(ServerMessage{Type: MsgSkillCasted, Data: map[string]interface{}{
				"caster_id": session.charID, "skill_name": "战吼_fear",
				"target_id": t.charID,
				"fear_dir_x": t.FearDirX, "fear_dir_y": t.FearDirY,
			}})
			continue
		}
		dmg := math.Round(combat.CalcDamage(session.attack*skill.Damage, t.defense) * 0.5) // PVP 减半
		if dmg < 1 {
			dmg = 1
		}
		isCrit := rand.Float64() < (session.critRate + 0.05)
		if isCrit {
			dmg = math.Round(dmg * session.critDmg)
		}
		t.hp -= dmg
		if t.hp < 0 {
			t.hp = 0
		}
		s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
			"attacker_id": session.charID, "target_id": t.charID,
			"damage": dmg, "is_crit": isCrit,
			"target_hp": t.hp, "target_max_hp": t.maxHP, "skill_name": skill.Name,
		}})
		if t.hp <= 0 {
			t.dead = true
			s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
				"char_id": t.charID, "name": t.charName,
				"killer_id": session.charID, "killer": session.charName,
			}})
		}
	}
}


func (s *DemoServer) handleChat(session *PlayerSession, data json.RawMessage) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &req); err != nil || req.Content == "" {
		return
	}
	s.arena.BroadcastAll(ServerMessage{Type: MsgChatMsg, Data: map[string]interface{}{
		"char_id": session.charID, "name": session.charName,
		"content": req.Content, "time": time.Now().Format("15:04:05"),
	}})
}

// handleUsePotion 处理玩家使用药水
func (s *DemoServer) handleUsePotion(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
		return
	}
	var req struct {
		PotionType string `json:"potion_type"` // "health" or "mana"
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}
	switch req.PotionType {
	case "health":
		heal := 50.0
		session.hp = min(session.hp+heal, session.maxHP)
	case "mana":
		restore := 30.0
		session.mp = min(session.mp+restore, session.maxMP)
	default:
		return
	}
	// HP/MP 变化会通过 doStateSync 自动同步给前端
}

// respawnPlayerAt 在指定位置复活玩家并广播
func (s *DemoServer) respawnPlayerAt(session *PlayerSession, x, y float64) {
	session.x = x
	session.y = y
	session.hp = session.maxHP
	session.mp = session.maxMP
	session.dead = false
	s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerRespawn, Data: map[string]interface{}{
		"char_id": session.charID, "name": session.charName,
		"x": session.x, "y": session.y,
		"hp": session.hp, "max_hp": session.maxHP, "mp": session.mp, "max_mp": session.maxMP,
	}})
	log.Printf("玩家 %s 在篝火处复活 (%.0f, %.0f)", session.charName, x, y)
}

// campfireRespawnTick 每60秒复活所有死亡玩家（在篝火附近随机位置）
func (s *DemoServer) campfireRespawnTick() {
	// 先在写锁内收集需要复活的玩家并更新状态，避免持锁时调用 BroadcastAll 导致死锁
	type respawnInfo struct {
		session *PlayerSession
		x, y    float64
	}
	var toRespawn []respawnInfo

	s.arena.mu.Lock()
	for _, p := range s.arena.players {
		if !p.dead {
			continue
		}
		angle := rand.Float64() * 2 * math.Pi
		d := 20.0 + rand.Float64()*30.0
		rx := math.Max(-ArenaWidth+30, math.Min(ArenaWidth-30, CampfireX+math.Cos(angle)*d))
		ry := math.Max(30, math.Min(ArenaHeight-30, CampfireY+math.Sin(angle)*d))
		// 先更新状态
		p.x = rx
		p.y = ry
		p.hp = p.maxHP
		p.mp = p.maxMP
		p.dead = false
		toRespawn = append(toRespawn, respawnInfo{session: p, x: rx, y: ry})
	}
	s.arena.mu.Unlock()

	// 释放锁后再广播（BroadcastAll 内部会拿读锁）
	for _, r := range toRespawn {
		s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerRespawn, Data: map[string]interface{}{
			"char_id": r.session.charID, "name": r.session.charName,
			"x": r.x, "y": r.y,
			"hp": r.session.hp, "max_hp": r.session.maxHP,
			"mp": r.session.mp, "max_mp": r.session.maxMP,
		}})
		log.Printf("玩家 %s 在篝火处复活 (%.0f, %.0f)", r.session.charName, r.x, r.y)
	}
}

// whirlwindTick 处理旋风斩持续伤害（每秒触发一次，持续5秒）
func (s *DemoServer) whirlwindTick() {
	now := time.Now().UnixMilli()

	// 收集需要处理的玩家和目标
	type whirlwindHit struct {
		caster *PlayerSession
		npc    *ArenaNPC
		player *PlayerSession
	}
	var hits []whirlwindHit

	s.arena.mu.Lock()
	for _, session := range s.arena.players {
		if session.dead || session.WhirlwindUntil == 0 {
			continue
		}
		if now > session.WhirlwindUntil {
			// 持续时间结束，清除状态
			session.WhirlwindUntil = 0
			session.WhirlwindNextTick = 0
			continue
		}
		if now < session.WhirlwindNextTick {
			continue
		}
		// 到了下一次 tick 时间
		session.WhirlwindNextTick = now + 1000

		// 收集范围内 NPC
		for _, npc := range s.arena.npcs {
			if npc.Dead {
				continue
			}
			if combat.IsInEllipseRange(session.x, session.y, npc.X, npc.Y, session.WhirlwindAreaSize) {
				hits = append(hits, whirlwindHit{caster: session, npc: npc})
			}
		}
		// 收集范围内玩家（PVP）
		for id, p := range s.arena.players {
			if id == session.charID || p.dead {
				continue
			}
			if combat.IsInEllipseRange(session.x, session.y, p.x, p.y, session.WhirlwindAreaSize) {
				hits = append(hits, whirlwindHit{caster: session, player: p})
			}
		}
	}

	// 锁内计算伤害并应用
	type dmgResult struct {
		caster    *PlayerSession
		targetID  int64
		dmg       float64
		isCrit    bool
		targetHP  float64
		maxHP     float64
		isNPC     bool
		npcDead   bool
		npc       *ArenaNPC
		player    *PlayerSession
		playerDead bool
	}
	var results []dmgResult
	for _, h := range hits {
		caster := h.caster
		if h.npc != nil {
			npc := h.npc
			if npc.Dead {
				continue
			}
			dmg := math.Round(combat.CalcDamage(caster.WhirlwindDamage, npc.Defense))
			if dmg < 1 {
				dmg = 1
			}
			isCrit := rand.Float64() < (caster.critRate + 0.05)
			if isCrit {
				dmg = math.Round(dmg * caster.critDmg)
			}
			npc.HP -= dmg
			if npc.HP < 0 {
				npc.HP = 0
			}
			dead := npc.HP <= 0
			if dead {
				npc.Dead = true
				delete(s.arena.npcs, npc.ID)
			}
			results = append(results, dmgResult{
				caster: caster, targetID: npc.ID, dmg: dmg, isCrit: isCrit,
				targetHP: npc.HP, maxHP: npc.MaxHP, isNPC: true, npcDead: dead, npc: npc,
			})
		} else if h.player != nil {
			t := h.player
			if t.dead {
				continue
			}
			dmg := math.Round(combat.CalcDamage(caster.WhirlwindDamage, t.defense) * 0.5) // PVP 减半
			if dmg < 1 {
				dmg = 1
			}
			isCrit := rand.Float64() < (caster.critRate + 0.05)
			if isCrit {
				dmg = math.Round(dmg * caster.critDmg)
			}
			t.hp -= dmg
			if t.hp < 0 {
				t.hp = 0
			}
			dead := t.hp <= 0
			if dead {
				t.dead = true
			}
			results = append(results, dmgResult{
				caster: caster, targetID: t.charID, dmg: dmg, isCrit: isCrit,
				targetHP: t.hp, maxHP: t.maxHP, isNPC: false, player: t, playerDead: dead,
			})
		}
	}
	s.arena.mu.Unlock()

	// 锁外广播
	for _, r := range results {
		if r.isNPC {
			s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
				"attacker_id":   r.caster.charID,
				"target_id":     r.targetID,
				"damage":        r.dmg,
				"is_crit":       r.isCrit,
				"target_hp":     r.targetHP,
				"target_max_hp": r.maxHP,
				"target_is_npc": true,
				"skill_name":    "旋风斩",
			}})
			if r.npcDead {
				s.arena.BroadcastAll(ServerMessage{Type: MsgNPCDied, Data: map[string]interface{}{
					"id":        r.npc.ID,
					"name":      r.npc.Name,
					"killer_id": r.caster.charID,
					"killer":    r.caster.charName,
				}})
				s.tryNPCDrop(r.npc, r.caster.charID)
			}
		} else {
			s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
				"attacker_id": r.caster.charID, "target_id": r.targetID,
				"damage": r.dmg, "is_crit": r.isCrit,
				"target_hp": r.targetHP, "target_max_hp": r.maxHP, "skill_name": "旋风斩",
			}})
			if r.playerDead {
				s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
					"char_id": r.targetID, "name": r.player.charName,
					"killer_id": r.caster.charID, "killer": r.caster.charName,
				}})
			}
		}
	}
}

// arenaTick 竞技场状态同步 tick 循环
// 使用 engine.GameLoop 替代手动 time.NewTicker + select，
// 以 NPCAITickHz（5Hz）作为 TickRate，各定时任务注册为不同阶段 handler。
func (s *DemoServer) arenaTick() {
	gl := engine.NewGameLoop(engine.GameLoopConfig{
		TickRate: NPCAITickHz,
	})
	s.gameLoop = gl

	// 派生常量：基于 TickRate 计算各周期对应的 tick 数
	tickRate := uint64(NPCAITickHz)
	whirlwindInterval := tickRate                // 旋风斩：每 5 tick = 1 秒
	campfireInterval := tickRate                 // 篝火倒计时：每 5 tick = 1 秒
	npcSpawnInterval := uint64(60) * tickRate    // NPC 刷新：每 300 tick = 60 秒

	// 篝火倒计时状态（闭包内维护）
	campfireCountdown := CampfireRespawnHz

	// PhaseUpdate：NPC AI（每 tick）+ 旋风斩持续伤害（每秒）
	gl.RegisterPhase(engine.PhaseUpdate, func(tick uint64, dt time.Duration) {
		// NPC AI 每 tick 执行
		s.npcAITick()

		// 旋风斩：每秒执行一次（每 whirlwindInterval 个 tick）
		if tick%whirlwindInterval == 0 {
			s.whirlwindTick()
		}
	})

	// PhaseSync：状态同步（每 tick，因为 StateSyncHz == NPCAITickHz == 5）
	gl.RegisterPhase(engine.PhaseSync, func(tick uint64, dt time.Duration) {
		s.doStateSync()
	})

	// PhaseCleanup：篝火倒计时（每秒）+ NPC 刷新（每 60 秒）
	gl.RegisterPhase(engine.PhaseCleanup, func(tick uint64, dt time.Duration) {
		// 篝火倒计时：每秒递减一次
		if tick%campfireInterval == 0 {
			campfireCountdown--
			if campfireCountdown <= 0 {
				campfireCountdown = CampfireRespawnHz
				s.campfireRespawnTick()
			}
			s.arena.BroadcastAll(ServerMessage{Type: MsgCampfireTick, Data: map[string]interface{}{
				"countdown": campfireCountdown,
			}})
		}

		// NPC 刷新：每 60 秒执行一次
		if tick%npcSpawnInterval == 0 {
			s.spawnNPCWave()
		}
	})

	// 首次立即刷新一波 NPC（在 GameLoop 启动前）
	s.spawnNPCWave()

	// 启动 GameLoop（阻塞直到 Stop 被调用）
	if err := gl.Start(); err != nil {
		log.Printf("GameLoop 启动失败: %v", err)
	}
}

// doStateSync 执行一次状态同步
func (s *DemoServer) doStateSync() {
	s.arena.mu.RLock()
	n := len(s.arena.players)
	if n == 0 {
		s.arena.mu.RUnlock()
		return
	}

		// 收集所有玩家列表
		allPlayers := make([]*PlayerSession, 0, n)
		for _, p := range s.arena.players {
			allPlayers = append(allPlayers, p)
		}
		s.arena.mu.RUnlock()

		// 给每个接收者单独计算增量
		for _, receiver := range allPlayers {
			if receiver.lastSync == nil {
				receiver.lastSync = make(map[int64]*playerSnapshot)
			}

			changed := make([]map[string]interface{}, 0)
			currentIds := make(map[int64]bool)

			for _, p := range allPlayers {
				currentIds[p.charID] = true
				snap := receiver.lastSync[p.charID]

				// 构建增量数据
				diff := map[string]interface{}{
					"char_id": p.charID,
				}
				hasDiff := false

				if snap == nil {
					// 新玩家，发送全量
					diff["name"] = p.charName
					diff["class"] = p.charClass
					diff["x"] = p.x
					diff["y"] = p.y
					diff["hp"] = p.hp
					diff["max_hp"] = p.maxHP
					diff["mp"] = p.mp
					diff["max_mp"] = p.maxMP
					diff["attack"] = p.attack
					diff["defense"] = p.defense
					diff["crit_rate"] = p.critRate
					diff["crit_damage"] = p.critDmg
					diff["dead"] = p.dead
					diff["direction"] = p.direction
					hasDiff = true
				} else {
					// 只发送变化的字段
					if math.Abs(snap.x-p.x) > 0.1 || math.Abs(snap.y-p.y) > 0.1 {
						diff["x"] = p.x
						diff["y"] = p.y
						hasDiff = true
					}
					if snap.hp != p.hp || snap.maxHP != p.maxHP {
						diff["hp"] = p.hp
						diff["max_hp"] = p.maxHP
						hasDiff = true
					}
					if snap.mp != p.mp || snap.maxMP != p.maxMP {
						diff["mp"] = p.mp
						diff["max_mp"] = p.maxMP
						hasDiff = true
					}
					if snap.attack != p.attack || snap.defense != p.defense {
						diff["attack"] = p.attack
						diff["defense"] = p.defense
						hasDiff = true
					}
					if snap.critRate != p.critRate || snap.critDmg != p.critDmg {
						diff["crit_rate"] = p.critRate
						diff["crit_damage"] = p.critDmg
						hasDiff = true
					}
					if snap.dead != p.dead {
						diff["dead"] = p.dead
						hasDiff = true
					}
					if snap.direction != p.direction {
						diff["direction"] = p.direction
						hasDiff = true
					}
				}

				if hasDiff {
					changed = append(changed, diff)
				}

				// 更新快照
				receiver.lastSync[p.charID] = &playerSnapshot{
					x: p.x, y: p.y,
					hp: p.hp, maxHP: p.maxHP,
					mp: p.mp, maxMP: p.maxMP,
					attack: p.attack, defense: p.defense,
					critRate: p.critRate, critDmg: p.critDmg,
					dead: p.dead, direction: p.direction,
				}
			}

			// 清理已离开的玩家快照
			for id := range receiver.lastSync {
				if !currentIds[id] {
					delete(receiver.lastSync, id)
				}
			}

			// 有变化才发送
			if len(changed) > 0 {
				receiver.Send(ServerMessage{Type: MsgStateSync, Data: map[string]interface{}{
					"players": changed,
				}})
			}
		}
}


// NPC 模板定义
var npcTemplates = []struct {
	Name        string
	Template    string
	Level       int
	MaxHP       float64
	Attack      float64
	Defense     float64
	Speed       float64
	AttackRange float64
	AttackCD    float64 // 攻击冷却（秒）
	AggroRange  float64 // 仇恨范围
	CritRate    float64
	CritDmg     float64
}{
	{"野狗", "wild_dog", 1, 80, 8, 3, 60, 50, 2.0, 150, 0.05, 1.5},
	{"流民", "starving", 2, 120, 10, 4, 50, 50, 2.5, 120, 0.05, 1.5},
	{"山贼", "bandit", 3, 200, 15, 6, 70, 60, 2.0, 200, 0.08, 1.5},
	{"官兵", "soldier", 4, 300, 20, 10, 80, 60, 1.8, 250, 0.10, 1.5},
	{"精锐官兵", "government_soldier", 5, 500, 30, 15, 90, 70, 1.5, 300, 0.12, 1.8},
}

// spawnNPCWave 刷新一波 NPC（10-20个）
func (s *DemoServer) spawnNPCWave() {
	s.arena.mu.Lock()

	// 清除已死亡的旧 NPC
	for id, npc := range s.arena.npcs {
		if npc.Dead {
			delete(s.arena.npcs, id)
		}
	}

	// 检查 NPC 上限
	aliveCount := len(s.arena.npcs)
	if aliveCount >= MaxNPCCount {
		s.arena.mu.Unlock()
		log.Printf("NPC 数量已达上限 %d，跳过刷新", MaxNPCCount)
		return
	}

	// 随机 10-20 个，但不超过上限
	count := 10 + rand.Intn(11)
	if aliveCount+count > MaxNPCCount {
		count = MaxNPCCount - aliveCount
	}
	if count <= 0 {
		s.arena.mu.Unlock()
		return
	}

	spawned := make([]map[string]interface{}, 0, count)

	for i := 0; i < count; i++ {
		tpl := npcTemplates[rand.Intn(len(npcTemplates))]
		s.arena.npcSeq++
		npcID := -s.arena.npcSeq // 负数 ID 区分玩家

		// 随机位置（在竞技场范围内，避开安全区）
		angle := rand.Float64() * 2 * math.Pi
		dist := CampfireRadius + 50.0 + rand.Float64()*500.0
		x := CampfireX + math.Cos(angle)*dist
		y := CampfireY + math.Sin(angle)*dist
		x = math.Max(-ArenaWidth+30, math.Min(ArenaWidth-30, x))
		y = math.Max(30, math.Min(ArenaHeight-30, y))

		npc := &ArenaNPC{
			ID:       npcID,
			Name:     tpl.Name,
			Template: tpl.Template,
			Level:    tpl.Level,
			X:        x,
			Y:        y,
			HP:       tpl.MaxHP,
			MaxHP:    tpl.MaxHP,
			Attack:   tpl.Attack,
			Defense:  tpl.Defense,
			Speed:    tpl.Speed,
			Dead:     false,
			// AI 属性
			AttackRange: tpl.AttackRange,
			AttackCD:    tpl.AttackCD,
			AggroRange:  tpl.AggroRange,
			CritRate:    tpl.CritRate,
			CritDmg:     tpl.CritDmg,
			SpawnX:      x,
			SpawnY:      y,
			LeashRange:  400.0,
		}
		s.arena.npcs[npcID] = npc

		spawned = append(spawned, s.makeNPCState(npc))
	}
	s.arena.mu.Unlock()

	// 广播 NPC 刷新
	s.arena.BroadcastAll(ServerMessage{Type: MsgNPCSpawn, Data: map[string]interface{}{
		"npcs": spawned,
	}})
	log.Printf("刷新 %d 个 NPC，当前存活: %d", count, aliveCount+count)
}

func (s *DemoServer) makeNPCState(npc *ArenaNPC) map[string]interface{} {
	return map[string]interface{}{
		"id": npc.ID, "name": npc.Name, "template": npc.Template,
		"level": npc.Level, "x": npc.X, "y": npc.Y,
		"hp": npc.HP, "max_hp": npc.MaxHP,
		"attack": npc.Attack, "defense": npc.Defense,
		"speed": npc.Speed, "dead": npc.Dead,
	}
}

// getAllNPCStates 获取所有存活 NPC 状态（新玩家进入时发送）
func (s *DemoServer) getAllNPCStates() []map[string]interface{} {
	s.arena.mu.RLock()
	defer s.arena.mu.RUnlock()
	result := make([]map[string]interface{}, 0, len(s.arena.npcs))
	for _, npc := range s.arena.npcs {
		if !npc.Dead {
			result = append(result, s.makeNPCState(npc))
		}
	}
	return result
}

// tryNPCDrop NPC死亡时必定掉落红瓶或蓝瓶，额外30%几率掉落铁箭
func (s *DemoServer) tryNPCDrop(npc *ArenaNPC, killerID int64) {
	// 必定掉落：50%红瓶 / 50%蓝瓶
	dropType := "health_potion"
	dropName := "红瓶"
	if rand.Float64() < 0.5 {
		dropType = "mana_potion"
		dropName = "蓝瓶"
	}
	s.arena.BroadcastAll(ServerMessage{Type: MsgNPCDrop, Data: map[string]interface{}{
		"npc_id":    npc.ID,
		"x":         npc.X,
		"y":         npc.Y,
		"drop_type": dropType,
		"drop_name": dropName,
		"killer_id": killerID,
	}})

	// 额外30%几率掉落铁箭（1-50根）
	if rand.Float64() < 0.3 {
		arrowCount := rand.Intn(50) + 1
		s.arena.BroadcastAll(ServerMessage{Type: MsgNPCDrop, Data: map[string]interface{}{
			"npc_id":     npc.ID,
			"x":          npc.X,
			"y":          npc.Y,
			"drop_type":  "iron_arrow",
			"drop_name":  fmt.Sprintf("铁箭x%d", arrowCount),
			"drop_count": arrowCount,
			"killer_id":  killerID,
		}})
	}
}

// handleAttackNPC 处理玩家攻击 NPC
func (s *DemoServer) handleAttackNPC(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
		return
	}
	// 安全区内禁止攻击
	if isInSafeZone(session.x, session.y) {
		session.Send(ServerMessage{Type: MsgError, Data: "安全区内禁止攻击"})
		return
	}
	var req struct {
		TargetID int64 `json:"target_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	s.arena.mu.Lock()
	npc, ok := s.arena.npcs[req.TargetID]
	if !ok || npc.Dead {
		s.arena.mu.Unlock()
		return
	}

	maxDist := session.weaponAttackDist
	if maxDist <= 0 {
		maxDist = 100.0
	}
	// 2.5D 椭圆距离判定（与前端 attackAllInRange 保持一致）
	if !combat.IsInEllipseRange(session.x, session.y, npc.X, npc.Y, maxDist) {
		s.arena.mu.Unlock()
		session.Send(ServerMessage{Type: MsgError, Data: "超出攻击范围"})
		return
	}

	damage := combat.CalcDamage(session.attack, npc.Defense)
	isCrit := rand.Float64() < session.critRate
	if isCrit {
		damage *= session.critDmg
	}
	damage = math.Round(damage)
	npc.HP -= damage
	if npc.HP < 0 {
		npc.HP = 0
	}
	dead := npc.HP <= 0
	if dead {
		npc.Dead = true
		delete(s.arena.npcs, req.TargetID)
	}
	s.arena.mu.Unlock()

	s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
		"attacker_id":    session.charID,
		"target_id":      req.TargetID,
		"damage":         damage,
		"is_crit":        isCrit,
		"target_hp":      npc.HP,
		"target_max_hp":  npc.MaxHP,
		"target_is_npc":  true,
		"attacker_class": session.charClass,
	}})

	if dead {
		s.arena.BroadcastAll(ServerMessage{Type: MsgNPCDied, Data: map[string]interface{}{
			"id":        req.TargetID,
			"name":      npc.Name,
			"killer_id": session.charID,
			"killer":    session.charName,
		}})
		// NPC 死亡掉落红瓶或蓝瓶（50%几率）
		s.tryNPCDrop(npc, session.charID)
	}
}

// handleCastSkillNPC 处理玩家对 NPC 释放技能
func (s *DemoServer) handleCastSkillNPC(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
		return
	}
	// 安全区内禁止使用技能
	if isInSafeZone(session.x, session.y) {
		session.Send(ServerMessage{Type: MsgError, Data: "安全区内禁止使用技能"})
		return
	}
	var req struct {
		SkillID  int64   `json:"skill_id"`
		TargetID int64   `json:"target_id"`
		TargetX  float64 `json:"target_x"`
		TargetY  float64 `json:"target_y"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	skills, _ := s.db.GetSkillsByClass(session.charClass)
	var skill *SkillInfo
	for _, sk := range skills {
		if sk.ID == req.SkillID {
			skill = &SkillInfo{
				ID: sk.ID, Name: sk.Name, Class: sk.Class,
				Damage: sk.Damage, MPCost: sk.MPCost, Cooldown: sk.Cooldown,
				Range: sk.Range, AreaType: sk.AreaType, AreaSize: sk.AreaSize,
			}
			break
		}
	}
	if skill == nil {
		session.Send(ServerMessage{Type: MsgError, Data: "技能不存在"})
		return
	}
	if session.mp < skill.MPCost {
		session.Send(ServerMessage{Type: MsgError, Data: "MP不足"})
		return
	}
	session.mp -= skill.MPCost

	// 猛击：扇形范围（与普攻一致），动态设置范围
	if skill.Name == "猛击" {
		skill.Range = session.weaponAttackDist
		if skill.Range <= 0 {
			skill.Range = 100.0
		}
	}
	// 旋风斩：椭圆范围 = 武器距离
	if skill.Name == "旋风斩" && session.weaponAttackDist > 0 {
		skill.AreaSize = session.weaponAttackDist
	}
	// 战吼：椭圆范围 = 武器距离 × 3
	if skill.Name == "战吼" {
		skill.AreaSize = session.weaponAttackDist * 3
		if skill.AreaSize <= 0 {
			skill.AreaSize = 300.0
		}
	}

	// 广播技能释放
	s.arena.BroadcastAll(ServerMessage{Type: MsgSkillCasted, Data: map[string]interface{}{
		"caster_id": session.charID, "skill_id": skill.ID, "skill_name": skill.Name,
		"target_x": req.TargetX, "target_y": req.TargetY,
		"area_type": skill.AreaType, "area_size": skill.AreaSize,
		"caster_x": session.x, "caster_y": session.y,
		"caster_mp": session.mp, "caster_max_mp": session.maxMP,
	}})

	// 查找范围内的 NPC 目标
	s.arena.mu.Lock()
	type npcHit struct {
		npc *ArenaNPC
	}
	var hits []npcHit
	for _, npc := range s.arena.npcs {
		if npc.Dead {
			continue
		}
		hit := false
		switch skill.AreaType {
		case "single":
			hit = npc.ID == req.TargetID && combat.IsInEllipseRange(session.x, session.y, npc.X, npc.Y, skill.Range)
		case "fan":
			dir := math.Atan2((req.TargetY-session.y)*2, req.TargetX-session.x)
			hit = combat.IsInFanRange(session.x, session.y, npc.X, npc.Y, skill.Range, dir, math.Pi/4)
		case "ellipse":
			hit = combat.IsInEllipseRange(session.x, session.y, npc.X, npc.Y, skill.AreaSize)
		case "circle":
			hit = combat.Distance(req.TargetX, req.TargetY, npc.X, npc.Y) <= skill.AreaSize
		}
		if hit {
			hits = append(hits, npcHit{npc: npc})
		}
	}

	// 锁内计算伤害，锁外广播
	type npcDmgResult struct {
		npc     *ArenaNPC
		dmg     float64
		isCrit  bool
		dead    bool
	}
	var dmgResults []npcDmgResult
	for _, h := range hits {
		dmg := math.Round(combat.CalcDamage(session.attack*skill.Damage, h.npc.Defense))
		if dmg < 1 {
			dmg = 1
		}
		isCrit := rand.Float64() < (session.critRate + 0.05)
		if isCrit {
			dmg = math.Round(dmg * session.critDmg)
		}
		h.npc.HP -= dmg
		if h.npc.HP < 0 {
			h.npc.HP = 0
		}
		dead := h.npc.HP <= 0
		if dead {
			h.npc.Dead = true
			delete(s.arena.npcs, h.npc.ID)
		}
		// 战吼恐惧效果：设置 FearUntil（锁内）
		if skill.Name == "战吼" && !dead {
			dx := h.npc.X - session.x
			dy := h.npc.Y - session.y
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > 0 {
				h.npc.FearDirX = dx / dist
				h.npc.FearDirY = dy / dist
			} else {
				angle := rand.Float64() * math.Pi * 2
				h.npc.FearDirX = math.Cos(angle)
				h.npc.FearDirY = math.Sin(angle)
			}
			h.npc.FearUntil = time.Now().UnixMilli() + 3000
			h.npc.TargetID = 0
		}
		dmgResults = append(dmgResults, npcDmgResult{npc: h.npc, dmg: dmg, isCrit: isCrit, dead: dead})
	}

	// 旋风斩：锁内设置持续状态
	if skill.Name == "旋风斩" {
		now := time.Now().UnixMilli()
		session.WhirlwindUntil = now + 5000
		session.WhirlwindNextTick = now + 1000
		session.WhirlwindDamage = session.attack * skill.Damage * 0.5
		session.WhirlwindAreaSize = skill.AreaSize
	}

	// 战吼：收集范围内玩家目标（锁内）
	var playerTargets []*PlayerSession
	if skill.Name == "战吼" {
		for id, p := range s.arena.players {
			if id == session.charID || p.dead {
				continue
			}
			if combat.IsInEllipseRange(session.x, session.y, p.x, p.y, skill.AreaSize) {
				playerTargets = append(playerTargets, p)
			}
		}
		// 战吼对玩家的恐惧效果（锁内设置）
		for _, t := range playerTargets {
			dx := t.x - session.x
			dy := t.y - session.y
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > 0 {
				t.FearDirX = dx / dist
				t.FearDirY = dy / dist
			} else {
				angle := rand.Float64() * math.Pi * 2
				t.FearDirX = math.Cos(angle)
				t.FearDirY = math.Sin(angle)
			}
			t.FearUntil = time.Now().UnixMilli() + 3000
		}
	}
	s.arena.mu.Unlock()

	// 锁外广播 NPC 伤害
	for _, r := range dmgResults {
		s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
			"attacker_id":   session.charID,
			"target_id":     r.npc.ID,
			"damage":        r.dmg,
			"is_crit":       r.isCrit,
			"target_hp":     r.npc.HP,
			"target_max_hp": r.npc.MaxHP,
			"target_is_npc": true,
			"skill_name":    skill.Name,
		}})
		if r.dead {
			s.arena.BroadcastAll(ServerMessage{Type: MsgNPCDied, Data: map[string]interface{}{
				"id":        r.npc.ID,
				"name":      r.npc.Name,
				"killer_id": session.charID,
				"killer":    session.charName,
			}})
			s.tryNPCDrop(r.npc, session.charID)
		}
	}

	// 战吼：锁外广播玩家伤害 + 恐惧消息
	if skill.Name == "战吼" {
		for _, t := range playerTargets {
			dmg := math.Round(combat.CalcDamage(session.attack*skill.Damage, t.defense) * 0.5)
			if dmg < 1 {
				dmg = 1
			}
			isCrit := rand.Float64() < (session.critRate + 0.05)
			if isCrit {
				dmg = math.Round(dmg * session.critDmg)
			}
			t.hp -= dmg
			if t.hp < 0 {
				t.hp = 0
			}
			s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
				"attacker_id": session.charID, "target_id": t.charID,
				"damage": dmg, "is_crit": isCrit,
				"target_hp": t.hp, "target_max_hp": t.maxHP, "skill_name": skill.Name,
			}})
			if t.hp <= 0 {
				t.dead = true
				s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
					"char_id": t.charID, "name": t.charName,
					"killer_id": session.charID, "killer": session.charName,
				}})
				continue
			}
			s.arena.BroadcastAll(ServerMessage{Type: MsgSkillCasted, Data: map[string]interface{}{
				"caster_id": session.charID, "skill_name": "战吼_fear",
				"target_id": t.charID,
				"fear_dir_x": t.FearDirX, "fear_dir_y": t.FearDirY,
			}})
		}
	}
}

// npcAITick NPC AI 主循环：寻敌、移动、攻击
// AI 决策委托给 combat.ComputeNPCAI 纯函数，副作用（HP 修改、广播）在 demo 层执行。
func (s *DemoServer) npcAITick() {
	now := time.Now().UnixMilli()

	s.arena.mu.Lock()
	if len(s.arena.npcs) == 0 || len(s.arena.players) == 0 {
		s.arena.mu.Unlock()
		return
	}

	// 收集存活玩家（过滤死亡和安全区内的玩家）
	alivePlayers := make([]*PlayerSession, 0, len(s.arena.players))
	for _, p := range s.arena.players {
		if !p.dead && !isInSafeZone(p.x, p.y) {
			alivePlayers = append(alivePlayers, p)
		}
	}
	if len(alivePlayers) == 0 {
		s.arena.mu.Unlock()
		return
	}

	// 构建目标列表（NPCAITarget 切片）
	targets := make([]combat.NPCAITarget, 0, len(alivePlayers))
	for _, p := range alivePlayers {
		targets = append(targets, combat.NPCAITarget{
			ID: p.charID, X: p.x, Y: p.y,
			HP: p.hp, Defense: p.defense, Dead: p.dead,
		})
	}

	// 收集需要广播的事件
	type npcMove struct {
		id   int64
		x, y float64
		dir  string
	}
	type npcAttack struct {
		npcID      int64
		npcName    string
		targetID   int64
		target     *PlayerSession
		damage     float64
		isCrit     bool
		targetMax  float64
		targetName string
	}
	var moves []npcMove
	var attacks []npcAttack

	dt := 1.0 / float64(NPCAITickHz) // 每次 tick 的时间间隔（秒）
	arenaMinX := -ArenaWidth + 30.0
	arenaMaxX := ArenaWidth - 30.0
	arenaMinY := 30.0
	arenaMaxY := ArenaHeight - 30.0

	for _, npc := range s.arena.npcs {
		if npc.Dead {
			continue
		}

		// 构建 NPCState（只读快照）
		npcState := &combat.NPCState{
			ID: npc.ID, X: npc.X, Y: npc.Y,
			HP: npc.HP, MaxHP: npc.MaxHP,
			Attack: npc.Attack, Defense: npc.Defense,
			Dead: npc.Dead, TargetID: npc.TargetID,
			SpawnX: npc.SpawnX, SpawnY: npc.SpawnY,
			LastAttackAt: npc.LastAttackAt,
			FearUntil: npc.FearUntil, FearDirX: npc.FearDirX, FearDirY: npc.FearDirY,
			StunUntil: npc.StunUntil, EnragedUntil: npc.EnragedUntil,
		}
		config := &combat.NPCAIConfig{
			AttackRange: npc.AttackRange, AggroRange: npc.AggroRange,
			LeashRange: npc.LeashRange, AttackCD: npc.AttackCD,
			CritRate: npc.CritRate, CritDmg: npc.CritDmg,
			Speed: npc.Speed,
		}

		// 调用纯函数计算 AI 决策
		action := combat.ComputeNPCAI(npcState, config, targets, now, dt, isInSafeZone, arenaMinX, arenaMaxX, arenaMinY, arenaMaxY)

		// 应用状态更新提示
		npc.TargetID = action.NewTargetID
		if action.ClearStun {
			npc.StunUntil = 0
		}
		if action.ClearFear {
			npc.FearUntil = 0
		}
		if action.ClearEnraged {
			npc.EnragedUntil = 0
		}
		if action.SetEnragedUntil > 0 {
			npc.EnragedUntil = action.SetEnragedUntil
		}
		if action.ResetSpawn {
			npc.SpawnX = npc.X
			npc.SpawnY = npc.Y
		}
		if action.NewLastAttackAt > 0 {
			npc.LastAttackAt = action.NewLastAttackAt
		}

		// 根据决策类型执行副作用
		switch action.Type {
		case "fear_move":
			npc.X = action.MoveX
			npc.Y = action.MoveY
			moves = append(moves, npcMove{id: npc.ID, x: npc.X, y: npc.Y, dir: action.MoveDir})

		case "chase":
			npc.X = action.MoveX
			npc.Y = action.MoveY
			moves = append(moves, npcMove{id: npc.ID, x: npc.X, y: npc.Y, dir: action.MoveDir})

		case "return":
			npc.X = action.MoveX
			npc.Y = action.MoveY
			if action.HealAmount > 0 {
				npc.HP = math.Min(npc.MaxHP, npc.HP+action.HealAmount)
			}

		case "attack":
			// 查找目标玩家 session（用于锁外二次检查时扣 HP）
			var targetSession *PlayerSession
			for _, p := range alivePlayers {
				if p.charID == action.TargetID {
					targetSession = p
					break
				}
			}
			if targetSession != nil {
				attacks = append(attacks, npcAttack{
					npcID:      npc.ID,
					npcName:    npc.Name,
					targetID:   action.TargetID,
					target:     targetSession,
					damage:     action.Damage,
					isCrit:     action.IsCrit,
					targetMax:  targetSession.maxHP,
					targetName: targetSession.charName,
				})
			}

		case "idle":
			// 无操作
		}
	}
	s.arena.mu.Unlock()

	// 广播 NPC 移动（批量）
	if len(moves) > 0 {
		npcMoves := make([]map[string]interface{}, 0, len(moves))
		for _, m := range moves {
			npcMoves = append(npcMoves, map[string]interface{}{
				"id": m.id, "x": m.x, "y": m.y, "direction": m.dir,
			})
		}
		s.arena.BroadcastAll(ServerMessage{Type: MsgNPCUpdate, Data: map[string]interface{}{
			"npcs": npcMoves,
		}})
	}

	// 广播 NPC 攻击伤害（再次检查 NPC 是否仍存活，过滤时序竞态产生的幽灵攻击）
	for _, atk := range attacks {
		s.arena.mu.Lock()
		npc, npcAlive := s.arena.npcs[atk.npcID]
		stillAlive := npcAlive && !npc.Dead
		if !stillAlive {
			log.Printf("[npcAITick] 二次检查拦截幽灵攻击: npcID=%d, npcAlive=%v, targetID=%d", atk.npcID, npcAlive, atk.targetID)
			if npcAlive && npc.Dead {
				delete(s.arena.npcs, atk.npcID)
			}
			s.arena.mu.Unlock()
			continue // NPC 已被击杀，丢弃此攻击（不扣 HP）
		}
		// NPC 仍存活，此时才扣除玩家 HP
		// 攻击成功说明已追上玩家，清除激怒状态（恢复正常仇恨范围逻辑）
		if npc.EnragedUntil > 0 {
			npc.EnragedUntil = 0
			npc.SpawnX = npc.X
			npc.SpawnY = npc.Y
		}
		log.Printf("[npcAITick] NPC %d(%s) 攻击玩家 %d, damage=%.0f, 玩家HP: %.0f -> %.0f", atk.npcID, atk.npcName, atk.targetID, atk.damage, atk.target.hp, atk.target.hp-atk.damage)
		atk.target.hp -= atk.damage
		if atk.target.hp < 0 {
			atk.target.hp = 0
		}
		targetHP := atk.target.hp
		targetDead := atk.target.hp <= 0
		if targetDead {
			atk.target.dead = true
		}
		s.arena.mu.Unlock()

		s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
			"attacker_id":    atk.npcID,
			"target_id":      atk.targetID,
			"damage":         atk.damage,
			"is_crit":        atk.isCrit,
			"target_hp":      targetHP,
			"target_max_hp":  atk.targetMax,
			"attacker_is_npc": true,
		}})
		if targetDead {
			s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
				"char_id": atk.targetID,
				"name":    atk.targetName,
				"killer":  atk.npcName,
			}})
			// 玩家进入灵魂状态，等待篝火复活
		}
	}
}


