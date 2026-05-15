package main

import (
	"encoding/json"
	"fmt"
	"log"

	"gogame/internal/network"
)

// initRouter 注册所有客户端消息处理器到 MessageRouter。
func (s *DemoServer) initRouter() {
	s.router = network.NewMessageRouter()

	// 带 data 参数的 handler，直接注册
	s.router.Register(MsgRegister, func(sess any, data json.RawMessage) {
		s.handleRegister(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgLogin, func(sess any, data json.RawMessage) {
		s.handleLogin(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgSelectClass, func(sess any, data json.RawMessage) {
		s.handleSelectClass(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgEquip, func(sess any, data json.RawMessage) {
		s.handleEquip(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgUnequip, func(sess any, data json.RawMessage) {
		s.handleUnequip(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgMove, func(sess any, data json.RawMessage) {
		s.handleMove(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgAttack, func(sess any, data json.RawMessage) {
		s.handleAttack(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgAttackNPC, func(sess any, data json.RawMessage) {
		s.handleAttackNPC(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgCastSkill, func(sess any, data json.RawMessage) {
		s.handleCastSkill(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgCastSkillNPC, func(sess any, data json.RawMessage) {
		s.handleCastSkillNPC(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgChat, func(sess any, data json.RawMessage) {
		s.handleChat(sess.(*PlayerSession), data)
	})
	s.router.Register(MsgUsePotion, func(sess any, data json.RawMessage) {
		s.handleUsePotion(sess.(*PlayerSession), data)
	})

	// 不带 data 参数的 handler，忽略 data
	s.router.Register(MsgEnterArena, func(sess any, data json.RawMessage) {
		s.handleEnterArena(sess.(*PlayerSession))
	})
	s.router.Register(MsgLeaveArena, func(sess any, data json.RawMessage) {
		s.handleLeaveArena(sess.(*PlayerSession))
	})
	s.router.Register(MsgGetCharInfo, func(sess any, data json.RawMessage) {
		s.handleGetCharInfo(sess.(*PlayerSession))
	})
	s.router.Register(MsgGetEquipList, func(sess any, data json.RawMessage) {
		s.handleGetEquipList(sess.(*PlayerSession))
	})
	s.router.Register(MsgGetInventory, func(sess any, data json.RawMessage) {
		s.handleGetInventory(sess.(*PlayerSession))
	})
}

func (s *DemoServer) handleMessage(session *PlayerSession, msg ClientMessage) {
	// 过滤高频 move 消息日志
	if msg.Type != MsgMove {
		log.Printf("收到消息: type=%s", msg.Type)
	}

	// 构造完整 JSON 消息用于 router 分发
	rawMsg, err := json.Marshal(msg)
	if err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "消息序列化失败"})
		return
	}

	if err := s.router.Dispatch(session, rawMsg); err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "未知消息类型"})
	}
}

// ---------- 注册 ----------

func (s *DemoServer) handleRegister(session *PlayerSession, data json.RawMessage) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "参数错误"})
		return
	}
	if len(req.Username) < 2 || len(req.Password) < 3 {
		session.Send(ServerMessage{Type: MsgError, Data: "用户名至少2字符，密码至少3字符"})
		return
	}
	acc, err := s.db.CreateAccount(req.Username, req.Password)
	if err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: fmt.Sprintf("注册失败: %v", err)})
		return
	}
	log.Printf("新用户注册: %s (ID=%d)", acc.Username, acc.ID)
	session.Send(ServerMessage{Type: MsgRegisterOK, Data: map[string]interface{}{
		"account_id": acc.ID,
		"username":   acc.Username,
	}})
}

// ---------- 登录 ----------

func (s *DemoServer) handleLogin(session *PlayerSession, data json.RawMessage) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "参数错误"})
		return
	}
	acc, err := s.db.Authenticate(req.Username, req.Password)
	if err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "用户名或密码错误"})
		return
	}

	// 查找角色
	ch, _ := s.db.GetCharacterByAccount(acc.ID)

	resp := map[string]interface{}{
		"account_id": acc.ID,
		"username":   acc.Username,
	}
	if ch != nil {
		session.charID = ch.ID
		session.charName = ch.Name
		session.charClass = ch.Class
		// 计算含装备的总属性
		total, _ := s.db.CalcTotalStats(ch)
		resp["character"] = total
		// 获取技能
		skills, _ := s.db.GetSkillsByClass(ch.Class)
		resp["skills"] = skills
		// 获取装备
		equips, _ := s.db.GetCharEquipments(ch.ID)
		resp["equipments"] = equips
	}

	session.Send(ServerMessage{Type: MsgLoginOK, Data: resp})
}

// ---------- 选择职业 ----------

func (s *DemoServer) handleSelectClass(session *PlayerSession, data json.RawMessage) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Name      string `json:"name"`
		Class     string `json:"class"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "参数错误"})
		return
	}
	if req.Class != "warrior" && req.Class != "archer" {
		session.Send(ServerMessage{Type: MsgError, Data: "无效职业，请选择 warrior 或 archer"})
		return
	}
	ch, err := s.db.CreateCharacter(req.AccountID, req.Name, req.Class)
	if err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: fmt.Sprintf("创建角色失败: %v", err)})
		return
	}
	session.charID = ch.ID
	session.charName = ch.Name
	session.charClass = ch.Class

	// 发放初始装备
	s.db.GrantInitialEquipments(ch.ID, ch.Class)

	skills, _ := s.db.GetSkillsByClass(ch.Class)
	equips, _ := s.db.GetCharEquipments(ch.ID)

	session.Send(ServerMessage{Type: MsgClassSelected, Data: map[string]interface{}{
		"character":  ch,
		"skills":     skills,
		"equipments": equips,
	}})
}

// ---------- 装备 ----------

func (s *DemoServer) handleEquip(session *PlayerSession, data json.RawMessage) {
	var req struct {
		EquipDefID int64 `json:"equip_def_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil || session.charID == 0 {
		session.Send(ServerMessage{Type: MsgError, Data: "参数错误或未登录"})
		return
	}
	eqDef, err := s.db.GetEquipmentDefByID(req.EquipDefID)
	if err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "装备不存在"})
		return
	}
	if eqDef.Class != "all" && eqDef.Class != session.charClass {
		session.Send(ServerMessage{Type: MsgError, Data: "该装备不适合你的职业"})
		return
	}
	// 近战职业不允许装备箭矢到副手
	if eqDef.SlotType == "ammo" && session.charClass != "archer" {
		session.Send(ServerMessage{Type: MsgError, Data: "近战职业无法装备箭矢"})
		return
	}
	// 箭矢装备时写入 quantity=99
	if eqDef.SlotType == "ammo" {
		if err := s.db.EquipAmmo(session.charID, eqDef.ID, 99); err != nil {
			session.Send(ServerMessage{Type: MsgError, Data: "装备失败"})
			return
		}
	} else if err := s.db.EquipItem(session.charID, req.EquipDefID, eqDef.SlotType); err != nil {
		session.Send(ServerMessage{Type: MsgError, Data: "装备失败"})
		return
	}
	s.sendCharFullInfo(session)
}

func (s *DemoServer) handleUnequip(session *PlayerSession, data json.RawMessage) {
	var req struct {
		SlotType string `json:"slot_type"`
	}
	if err := json.Unmarshal(data, &req); err != nil || session.charID == 0 {
		session.Send(ServerMessage{Type: MsgError, Data: "参数错误或未登录"})
		return
	}
	// 前端槽位名 → 后端 slot_type 映射
	slotMap := map[string]string{
		"mainhand": "weapon",
		"offhand":  "ammo",
	}
	slotType := req.SlotType
	if mapped, ok := slotMap[slotType]; ok {
		slotType = mapped
	}
	s.db.UnequipItem(session.charID, slotType)
	s.sendCharFullInfo(session)
}

func (s *DemoServer) sendCharFullInfo(session *PlayerSession) {
	ch, _ := s.db.GetCharacterByID(session.charID)
	if ch == nil {
		return
	}
	total, _ := s.db.CalcTotalStats(ch)
	equips, _ := s.db.GetCharEquipments(ch.ID)
	skills, _ := s.db.GetSkillsByClass(ch.Class)

	// 如果玩家在竞技场内，同步更新 session 的战斗属性
	// 这样 arenaTick 增量同步会自动将变化广播给其他玩家
	if session.inArena {
		session.attack = total.Attack
		session.defense = total.Defense
		session.speed = total.Speed
		session.critRate = total.CritRate
		// 保留 HP/MP 比例
		if session.maxHP > 0 {
			hpRatio := session.hp / session.maxHP
			session.maxHP = total.MaxHP
			session.hp = session.maxHP * hpRatio
		}
		if session.maxMP > 0 {
			mpRatio := session.mp / session.maxMP
			session.maxMP = total.MaxMP
			session.mp = session.maxMP * mpRatio
		}
		// 更新武器攻击范围
		session.weaponAttackRange = 60.0
		session.weaponAttackDist = 100.0
		session.weaponIsRanged = false
		if session.charClass == "archer" {
			session.weaponAttackRange = 30.0  // 弓箭手扇形角度（度数）
			session.weaponAttackDist = 250.0
			session.weaponIsRanged = true
		}
		for _, eq := range equips {
			if eq.SlotType == "weapon" && eq.Def.AttackRange > 0 {
				session.weaponAttackRange = eq.Def.AttackRange
				session.weaponAttackDist = eq.Def.AttackDistance
				if eq.Def.Class == "archer" {
					session.weaponIsRanged = true
				}
			}
		}
	}

	session.Send(ServerMessage{Type: MsgCharInfo, Data: map[string]interface{}{
		"character":  total,
		"equipments": equips,
		"skills":     skills,
	}})
	// 同步背包
	inventory, _ := s.db.GetCharInventory(ch.ID)
	session.Send(ServerMessage{Type: MsgInventory, Data: inventory})
}

func (s *DemoServer) handleGetCharInfo(session *PlayerSession) {
	if session.charID == 0 {
		session.Send(ServerMessage{Type: MsgError, Data: "未创建角色"})
		return
	}
	s.sendCharFullInfo(session)
}

func (s *DemoServer) handleGetEquipList(session *PlayerSession) {
	if session.charID == 0 {
		session.Send(ServerMessage{Type: MsgError, Data: "未创建角色"})
		return
	}
	equips, _ := s.db.GetEquipmentDefs(session.charClass)
	session.Send(ServerMessage{Type: MsgEquipList, Data: equips})
}

func (s *DemoServer) handleGetInventory(session *PlayerSession) {
	if session.charID == 0 {
		session.Send(ServerMessage{Type: MsgError, Data: "未创建角色"})
		return
	}
	inventory, _ := s.db.GetCharInventory(session.charID)
	session.Send(ServerMessage{Type: MsgInventory, Data: inventory})
}
