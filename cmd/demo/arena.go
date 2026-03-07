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

	// 从装备中读取武器攻击范围
	equips, _ := s.db.GetCharEquipments(session.charID)
	session.weaponAttackRange = 60.0  // 默认近战范围
	session.weaponAttackDist = 100.0
	if session.charClass == "archer" {
		session.weaponAttackRange = 200.0
		session.weaponAttackDist = 250.0
	}
	for _, eq := range equips {
		if eq.SlotType == "weapon" && eq.Def.AttackRange > 0 {
			session.weaponAttackRange = eq.Def.AttackRange
			session.weaponAttackDist = eq.Def.AttackDistance
		}
	}

	s.arena.mu.Lock()
	s.arena.players[session.charID] = session
	s.arena.mu.Unlock()
	skills, _ := s.db.GetSkillsByClass(session.charClass)
	players := s.getArenaPlayersState()
	session.Send(ServerMessage{Type: MsgArenaState, Data: map[string]interface{}{
		"players": players, "self_id": session.charID,
		"campfire": map[string]float64{"x": CampfireX, "y": CampfireY},
		"arena":    map[string]float64{"width": ArenaWidth, "height": ArenaHeight},
		"skills":   skills,
		"equipments": equips,
		"npcs":     s.getAllNPCStates(),
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
	dist := distance(session.x, session.y, target.x, target.y)
	maxRange := session.weaponAttackRange
	if maxRange <= 0 {
		maxRange = 60.0
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

	// 旋风斩特殊处理：使用武器攻击范围
	if skill.Name == "旋风斩" && session.weaponAttackRange > 0 {
		skill.AreaSize = session.weaponAttackRange
	}

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

// arenaTick 竞技场状态同步 tick 循环
func (s *DemoServer) arenaTick() {
	ticker := time.NewTicker(time.Second / StateSyncHz)
	defer ticker.Stop()

	// NPC 刷新定时器：每60秒检查并刷新
	npcTicker := time.NewTicker(60 * time.Second)
	defer npcTicker.Stop()

	// NPC AI 定时器
	aiTicker := time.NewTicker(time.Second / NPCAITickHz)
	defer aiTicker.Stop()

	// 篝火倒计时（每秒 -1，到 0 时触发复活并重置）
	campfireCountdownTicker := time.NewTicker(time.Second)
	defer campfireCountdownTicker.Stop()
	campfireCountdown := CampfireRespawnHz // 初始倒计时

	// 首次立即刷新一波 NPC
	s.spawnNPCWave()

	for {
		select {
		case <-npcTicker.C:
			s.spawnNPCWave()
		case <-aiTicker.C:
			s.npcAITick()
		case <-ticker.C:
			s.doStateSync()
		case <-campfireCountdownTicker.C:
			campfireCountdown--
			if campfireCountdown <= 0 {
				campfireCountdown = CampfireRespawnHz
				s.campfireRespawnTick()
			}
			s.arena.BroadcastAll(ServerMessage{Type: MsgCampfireTick, Data: map[string]interface{}{
				"countdown": campfireCountdown,
			}})
		}
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

		// 随机位置（在竞技场范围内，避开火堆中心）
		angle := rand.Float64() * 2 * math.Pi
		dist := 150.0 + rand.Float64()*600.0
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

// tryNPCDrop NPC死亡时随机掉落红瓶或蓝瓶（50%几率）
func (s *DemoServer) tryNPCDrop(npc *ArenaNPC, killerID int64) {
	if rand.Float64() > 0.5 {
		return // 50% 不掉落
	}
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
}

// handleAttackNPC 处理玩家攻击 NPC
func (s *DemoServer) handleAttackNPC(session *PlayerSession, data json.RawMessage) {
	if !session.inArena || session.dead {
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

	dist := distance(session.x, session.y, npc.X, npc.Y)
	maxRange := session.weaponAttackRange
	if maxRange <= 0 {
		maxRange = 60.0
	}
	if dist > maxRange {
		s.arena.mu.Unlock()
		session.Send(ServerMessage{Type: MsgError, Data: "超出攻击范围"})
		return
	}

	damage := calcDamage(session.attack, npc.Defense)
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

	// 旋风斩特殊处理：使用武器攻击范围
	if skill.Name == "旋风斩" && session.weaponAttackRange > 0 {
		skill.AreaSize = session.weaponAttackRange
	}

	// 广播技能释放
	s.arena.BroadcastAll(ServerMessage{Type: MsgSkillCasted, Data: map[string]interface{}{
		"caster_id": session.charID, "skill_id": skill.ID, "skill_name": skill.Name,
		"target_x": req.TargetX, "target_y": req.TargetY,
		"area_type": skill.AreaType, "area_size": skill.AreaSize,
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
			hit = npc.ID == req.TargetID && distance(session.x, session.y, npc.X, npc.Y) <= skill.Range
		case "circle":
			hit = distance(req.TargetX, req.TargetY, npc.X, npc.Y) <= skill.AreaSize
		case "fan":
			hit = distance(session.x, session.y, npc.X, npc.Y) <= skill.Range
		}
		if hit {
			hits = append(hits, npcHit{npc: npc})
		}
	}

	for _, h := range hits {
		dmg := math.Round(calcDamage(session.attack*skill.Damage, h.npc.Defense))
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
		}

		s.arena.mu.Unlock()
		s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
			"attacker_id":    session.charID,
			"target_id":      h.npc.ID,
			"damage":         dmg,
			"is_crit":        isCrit,
			"target_hp":      h.npc.HP,
			"target_max_hp":  h.npc.MaxHP,
			"target_is_npc":  true,
			"skill_name":     skill.Name,
		}})
		if dead {
			s.arena.BroadcastAll(ServerMessage{Type: MsgNPCDied, Data: map[string]interface{}{
				"id":        h.npc.ID,
				"name":      h.npc.Name,
				"killer_id": session.charID,
				"killer":    session.charName,
			}})
			s.tryNPCDrop(h.npc, session.charID)
		}
		s.arena.mu.Lock()
	}

	// 战吼恐惧效果：让范围内NPC远离战士，四处逃跑3秒
	if skill.Name == "战吼" {
		for _, h := range hits {
			if !h.npc.Dead {
				// 计算远离方向
				dx := h.npc.X - session.x
				dy := h.npc.Y - session.y
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < 1 {
					dist = 1
				}
				// 设置恐惧状态：NPC 向远离方向逃跑
				h.npc.FearUntil = time.Now().UnixMilli() + 3000
				h.npc.FearDirX = dx / dist
				h.npc.FearDirY = dy / dist
			}
		}
	}

	s.arena.mu.Unlock()
}

// respawnNPC 5秒后复活 NPC
// npcAITick NPC AI 主循环：寻敌、移动、攻击
func (s *DemoServer) npcAITick() {
	now := time.Now().UnixMilli()

	s.arena.mu.Lock()
	if len(s.arena.npcs) == 0 || len(s.arena.players) == 0 {
		s.arena.mu.Unlock()
		return
	}

	// 收集存活玩家
	alivePlayers := make([]*PlayerSession, 0, len(s.arena.players))
	for _, p := range s.arena.players {
		if !p.dead {
			alivePlayers = append(alivePlayers, p)
		}
	}
	if len(alivePlayers) == 0 {
		s.arena.mu.Unlock()
		return
	}

	// 收集需要广播的事件
	type npcMove struct {
		id   int64
		x, y float64
		dir  string
	}
	type npcAttack struct {
		npcID     int64
		npcName   string
		targetID  int64
		damage    float64
		isCrit    bool
		targetHP  float64
		targetMax float64
		targetDead bool
		targetName string
	}
	var moves []npcMove
	var attacks []npcAttack

	dt := 1.0 / float64(NPCAITickHz) // 每次 tick 的时间间隔（秒）

	for _, npc := range s.arena.npcs {
		if npc.Dead {
			continue
		}

		// 0. 检查恐惧状态（战吼效果）
		if npc.FearUntil > 0 && now < npc.FearUntil {
			// 恐惧中：向远离方向逃跑
			moveSpeed := npc.Speed * dt * 1.5 // 恐惧时跑得更快
			npc.X += npc.FearDirX * moveSpeed
			npc.Y += npc.FearDirY * moveSpeed
			// 边界限制
			npc.X = math.Max(-ArenaWidth+30, math.Min(ArenaWidth-30, npc.X))
			npc.Y = math.Max(30, math.Min(ArenaHeight-30, npc.Y))
			// 计算方向
			dir := "d"
			if math.Abs(npc.FearDirX) > math.Abs(npc.FearDirY) {
				if npc.FearDirX > 0 { dir = "r" } else { dir = "l" }
			} else {
				if npc.FearDirY > 0 { dir = "d" } else { dir = "u" }
			}
			moves = append(moves, npcMove{id: npc.ID, x: npc.X, y: npc.Y, dir: dir})
			npc.TargetID = 0
			continue
		}
		// 恐惧结束，清除状态
		if npc.FearUntil > 0 && now >= npc.FearUntil {
			npc.FearUntil = 0
		}

		// 1. 寻找最近的存活玩家
		var closest *PlayerSession
		closestDist := math.MaxFloat64
		for _, p := range alivePlayers {
			d := distance(npc.X, npc.Y, p.x, p.y)
			if d < closestDist {
				closestDist = d
				closest = p
			}
		}
		if closest == nil {
			continue
		}

		// 2. 检查是否在仇恨范围内
		if closestDist > npc.AggroRange {
			// 超出仇恨范围，检查是否需要回归出生点
			spawnDist := distance(npc.X, npc.Y, npc.SpawnX, npc.SpawnY)
			if spawnDist > 5 {
				// 缓慢回归出生点
				dx := npc.SpawnX - npc.X
				dy := npc.SpawnY - npc.Y
				d := math.Sqrt(dx*dx + dy*dy)
				moveSpeed := npc.Speed * dt * 0.5 // 回归速度减半
				if moveSpeed > d {
					moveSpeed = d
				}
				npc.X += (dx / d) * moveSpeed
				npc.Y += (dy / d) * moveSpeed
				// 回归时回血
				if npc.HP < npc.MaxHP {
					npc.HP = math.Min(npc.MaxHP, npc.HP+npc.MaxHP*0.05*dt)
				}
			}
			npc.TargetID = 0
			continue
		}

		// 3. 锁定目标
		npc.TargetID = closest.charID

		// 4. 在攻击范围内则攻击，否则移动靠近
		if closestDist <= npc.AttackRange {
			// 检查攻击冷却
			cdMs := int64(npc.AttackCD * 1000)
			if now-npc.LastAttackAt >= cdMs {
				npc.LastAttackAt = now
				// 计算伤害
				dmg := calcDamage(npc.Attack, closest.defense)
				isCrit := rand.Float64() < npc.CritRate
				if isCrit {
					dmg *= npc.CritDmg
				}
				dmg = math.Round(dmg)
				closest.hp -= dmg
				if closest.hp < 0 {
					closest.hp = 0
				}
				dead := closest.hp <= 0
				if dead {
					closest.dead = true
				}
				attacks = append(attacks, npcAttack{
					npcID:      npc.ID,
					npcName:    npc.Name,
					targetID:   closest.charID,
					damage:     dmg,
					isCrit:     isCrit,
					targetHP:   closest.hp,
					targetMax:  closest.maxHP,
					targetDead: dead,
					targetName: closest.charName,
				})
			}
		} else {
			// 移动靠近目标
			dx := closest.x - npc.X
			dy := closest.y - npc.Y
			d := math.Sqrt(dx*dx + dy*dy)
			moveSpeed := npc.Speed * dt
			if moveSpeed > d-npc.AttackRange*0.8 {
				moveSpeed = d - npc.AttackRange*0.8
			}
			if moveSpeed > 0 {
				npc.X += (dx / d) * moveSpeed
				npc.Y += (dy / d) * moveSpeed
				// 计算方向
				dir := "d"
				if math.Abs(dx) > math.Abs(dy) {
					if dx > 0 {
						dir = "r"
					} else {
						dir = "l"
					}
				} else {
					if dy > 0 {
						dir = "d"
					} else {
						dir = "u"
					}
				}
				moves = append(moves, npcMove{id: npc.ID, x: npc.X, y: npc.Y, dir: dir})
			}
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

	// 广播 NPC 攻击伤害
	for _, atk := range attacks {
		s.arena.BroadcastAll(ServerMessage{Type: MsgDamageDealt, Data: map[string]interface{}{
			"attacker_id":    atk.npcID,
			"target_id":      atk.targetID,
			"damage":         atk.damage,
			"is_crit":        atk.isCrit,
			"target_hp":      atk.targetHP,
			"target_max_hp":  atk.targetMax,
			"attacker_is_npc": true,
		}})
		if atk.targetDead {
			s.arena.BroadcastAll(ServerMessage{Type: MsgPlayerDied, Data: map[string]interface{}{
				"char_id": atk.targetID,
				"name":    atk.targetName,
				"killer":  atk.npcName,
			}})
			// 玩家进入灵魂状态，等待篝火复活
		}
	}
}


