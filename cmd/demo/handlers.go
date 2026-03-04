package main

import (
	"encoding/json"
	"fmt"
	"log"
)
func (s *DemoServer) handleMessage(session *PlayerSession, msg ClientMessage) {
	// 过滤高频 move 消息日志
	if msg.Type != MsgMove {
		log.Printf("收到消息: type=%s", msg.Type)
	}
	switch msg.Type {
	case MsgRegister:
		s.handleRegister(session, msg.Data)
	case MsgLogin:
		s.handleLogin(session, msg.Data)
	case MsgSelectClass:
		s.handleSelectClass(session, msg.Data)
	case MsgEquip:
		s.handleEquip(session, msg.Data)
	case MsgUnequip:
		s.handleUnequip(session, msg.Data)
	case MsgEnterArena:
		s.handleEnterArena(session)
	case MsgLeaveArena:
		s.handleLeaveArena(session)
	case MsgMove:
		s.handleMove(session, msg.Data)
	case MsgAttack:
		s.handleAttack(session, msg.Data)
	case MsgCastSkill:
		s.handleCastSkill(session, msg.Data)
	case MsgChat:
		s.handleChat(session, msg.Data)
	case MsgGetCharInfo:
		s.handleGetCharInfo(session)
	case MsgGetEquipList:
		s.handleGetEquipList(session)
	default:
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

	skills, _ := s.db.GetSkillsByClass(ch.Class)

	session.Send(ServerMessage{Type: MsgClassSelected, Data: map[string]interface{}{
		"character": ch,
		"skills":    skills,
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
	if err := s.db.EquipItem(session.charID, req.EquipDefID, eqDef.SlotType); err != nil {
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
	s.db.UnequipItem(session.charID, req.SlotType)
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
	}

	session.Send(ServerMessage{Type: MsgCharInfo, Data: map[string]interface{}{
		"character":  total,
		"equipments": equips,
		"skills":     skills,
	}})
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
