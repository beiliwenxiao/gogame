package main

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"time"
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

func distance(x1, y1, x2, y2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	return math.Sqrt(dx*dx + dy*dy)
}

func calcDamage(atk, def float64) float64 {
	base := atk - def*0.5
	if base < 1 {
		base = 1
	}
	return base * (0.85 + rand.Float64()*0.3)
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
	s.arena.mu.Lock()
	s.arena.players[session.charID] = session
	s.arena.mu.Unlock()
	skills, _ := s.db.GetSkillsByClass(session.charClass)
	equips, _ := s.db.GetCharEquipments(session.charID)
	players := s.getArenaPlayersState()
	session.Send(ServerMessage{Type: MsgArenaState, Data: map[string]interface{}{
		"players": players, "self_id": session.charID,
		"campfire": map[string]float64{"x": CampfireX, "y": CampfireY},
		"arena":    map[string]float64{"width": ArenaWidth, "height": ArenaHeight},
		"skills":   skills,
		"equipments": equips,
	}})
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
	if !session.inArena || session.dead {
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
	dist := distance(session.x, session.y, target.x, target.y)
	maxRange := 60.0
	if session.charClass == "archer" {
		maxRange = 200.0
	}
	if dist > maxRange {
		session.Send(ServerMessage{Type: MsgError, Data: "超出攻击范围"})
		return
	}
	damage := calcDamage(session.attack, target.defense)
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
		go s.respawnPlayer(target)
	}
}


func (s *DemoServer) handleCastSkill(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
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
	s.arena.BroadcastAll(ServerMessage{Type: MsgSkillCasted, Data: map[string]interface{}{
		"caster_id": session.charID, "skill_id": skill.ID, "skill_name": skill.Name,
		"target_x": req.TargetX, "target_y": req.TargetY,
		"area_type": skill.AreaType, "area_size": skill.AreaSize,
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
			hit = id == req.TargetID && distance(session.x, session.y, p.x, p.y) <= skill.Range
		case "circle":
			hit = distance(req.TargetX, req.TargetY, p.x, p.y) <= skill.AreaSize
		case "fan":
			hit = distance(session.x, session.y, p.x, p.y) <= skill.Range
		}
		if hit {
			targets = append(targets, p)
		}
	}
	s.arena.mu.RUnlock()
	for _, t := range targets {
		dmg := math.Round(calcDamage(session.attack*skill.Damage, t.defense))
		if dmg < 1 {
			dmg = 1
		}
		isCrit := rand.Float64() < (session.critRate + 0.05) // 技能暴击率比普攻高5%
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
			go s.respawnPlayer(t)
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

func (s *DemoServer) respawnPlayer(session *PlayerSession) {
	time.Sleep(5 * time.Second)
	if !session.inArena {
		return
	}
	angle := rand.Float64() * 2 * math.Pi
	d := 50.0 + rand.Float64()*100.0
	session.x = math.Max(-ArenaWidth+30, math.Min(ArenaWidth-30, CampfireX+math.Cos(angle)*d))
	session.y = math.Max(30, math.Min(ArenaHeight-30, CampfireY+math.Sin(angle)*d))
	session.hp = session.maxHP
	session.mp = session.maxMP
	session.dead = false
	s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerRespawn, Data: map[string]interface{}{
		"char_id": session.charID, "name": session.charName,
		"x": session.x, "y": session.y,
		"hp": session.hp, "max_hp": session.maxHP, "mp": session.mp, "max_mp": session.maxMP,
	}})
	log.Printf("玩家 %s 已复活", session.charName)
}

// arenaTick 竞技场状态同步 tick 循环
// 每 1/TickRate 秒广播一次全量玩家状态，确保多端一致
func (s *DemoServer) arenaTick() {
	ticker := time.NewTicker(time.Second / StateSyncHz)
	defer ticker.Stop()

	for range ticker.C {
		s.arena.mu.RLock()
		n := len(s.arena.players)
		if n == 0 {
			s.arena.mu.RUnlock()
			continue
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
}
