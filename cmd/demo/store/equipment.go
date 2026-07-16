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

package store

import (
	"fmt"
	"log"
)

// GetEquipmentDefs 获取角色可用的装备列表
func (s *Store) GetEquipmentDefs(class string) ([]EquipmentDef, error) {
	rows, err := s.db.Query(
		`SELECT id,name,icon_id,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate,
		        pierce,multi_arrow,attack_interval,attack_range,attack_distance
		 FROM equipment_defs WHERE class=? OR class='all' ORDER BY quality DESC, attack DESC`,
		class,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EquipmentDef
	for rows.Next() {
		var eq EquipmentDef
		if err := rows.Scan(&eq.ID, &eq.Name, &eq.IconID, &eq.SlotType, &eq.Class, &eq.Quality, &eq.Level,
			&eq.Attack, &eq.Defense, &eq.HP, &eq.Speed, &eq.CritRate,
			&eq.Pierce, &eq.MultiArrow, &eq.AttackInterval, &eq.AttackRange, &eq.AttackDistance); err != nil {
			return nil, err
		}
		result = append(result, eq)
	}
	return result, nil
}

// GetEquipmentDefByID 根据ID获取装备定义
func (s *Store) GetEquipmentDefByID(id int64) (*EquipmentDef, error) {
	var eq EquipmentDef
	err := s.db.QueryRow(
		`SELECT id,name,icon_id,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate,
		        pierce,multi_arrow,attack_interval,attack_range,attack_distance
		 FROM equipment_defs WHERE id=?`, id,
	).Scan(&eq.ID, &eq.Name, &eq.IconID, &eq.SlotType, &eq.Class, &eq.Quality, &eq.Level,
		&eq.Attack, &eq.Defense, &eq.HP, &eq.Speed, &eq.CritRate,
		&eq.Pierce, &eq.MultiArrow, &eq.AttackInterval, &eq.AttackRange, &eq.AttackDistance)
	if err != nil {
		return nil, err
	}
	return &eq, nil
}

// EquipItem 装备物品（把旧装备放回背包）
func (s *Store) EquipItem(charID, equipDefID int64, slotType string) error {
	// 先把同槽位的旧装备放回背包
	var oldDefID int64
	s.db.QueryRow(
		"SELECT equip_def_id FROM char_equipments WHERE character_id=? AND slot_type=?",
		charID, slotType,
	).Scan(&oldDefID)
	if oldDefID > 0 {
		s.AddToInventory(charID, oldDefID, 1, 1)
	}
	s.db.Exec("DELETE FROM char_equipments WHERE character_id=? AND slot_type=?", charID, slotType)

	// 从背包移除新装备
	s.RemoveFromInventory(charID, equipDefID, 1)

	_, err := s.db.Exec(
		"INSERT INTO char_equipments (character_id, equip_def_id, slot_type) VALUES (?,?,?)",
		charID, equipDefID, slotType,
	)
	if err != nil {
		return fmt.Errorf("装备失败: %w", err)
	}
	return nil
}

// EquipAmmo 装备箭矢到副手（从背包取，不移动数量，只记录类型）
func (s *Store) EquipAmmo(charID, equipDefID int64, quantity int) error {
	// 检查背包中是否有该箭矢
	total := s.GetInventoryCount(charID, equipDefID)
	if total == 0 {
		// 背包无存量时，初始化放入背包再装备（首次发放场景）
		s.AddToInventory(charID, equipDefID, quantity, 99)
	}
	// 副手只记录类型，不移动数量
	s.db.Exec("DELETE FROM char_equipments WHERE character_id=? AND slot_type='ammo'", charID)
	_, err := s.db.Exec(
		"INSERT INTO char_equipments (character_id, equip_def_id, slot_type, quantity) VALUES (?,?,'ammo',0)",
		charID, equipDefID,
	)
	return err
}

// UnequipItem 卸下装备（放回背包）
func (s *Store) UnequipItem(charID int64, slotType string) error {
	log.Printf("[UnequipItem] charID=%d, slotType=%s", charID, slotType)
	if slotType == "ammo" {
		_, err := s.db.Exec(
			"DELETE FROM char_equipments WHERE character_id=? AND slot_type=?", charID, slotType,
		)
		return err
	}
	var defID int64
	err := s.db.QueryRow(
		"SELECT equip_def_id FROM char_equipments WHERE character_id=? AND slot_type=?",
		charID, slotType,
	).Scan(&defID)
	log.Printf("[UnequipItem] found defID=%d, err=%v", defID, err)
	if defID > 0 {
		s.AddToInventory(charID, defID, 1, 1)
		log.Printf("[UnequipItem] added to inventory: defID=%d", defID)
	}
	_, err = s.db.Exec(
		"DELETE FROM char_equipments WHERE character_id=? AND slot_type=?", charID, slotType,
	)
	log.Printf("[UnequipItem] deleted from char_equipments, err=%v", err)
	// 验证背包
	count := s.GetInventoryCount(charID, defID)
	log.Printf("[UnequipItem] inventory count for defID=%d: %d", defID, count)
	return err
}

// GetCharEquipments 获取角色已装备的装备
func (s *Store) GetCharEquipments(charID int64) ([]CharEquipWithDef, error) {
	rows, err := s.db.Query(
		`SELECT ce.id, ce.slot_type, ce.quantity, ed.id, ed.name, ed.icon_id, ed.slot_type, ed.class, ed.quality, ed.level,
		        ed.attack, ed.defense, ed.hp, ed.speed, ed.crit_rate,
		        ed.pierce, ed.multi_arrow, ed.attack_interval, ed.attack_range, ed.attack_distance
		 FROM char_equipments ce
		 JOIN equipment_defs ed ON ce.equip_def_id = ed.id
		 WHERE ce.character_id=?`, charID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CharEquipWithDef
	for rows.Next() {
		var item CharEquipWithDef
		if err := rows.Scan(&item.ID, &item.SlotType, &item.Quantity,
			&item.Def.ID, &item.Def.Name, &item.Def.IconID, &item.Def.SlotType, &item.Def.Class, &item.Def.Quality, &item.Def.Level,
			&item.Def.Attack, &item.Def.Defense, &item.Def.HP, &item.Def.Speed, &item.Def.CritRate,
			&item.Def.Pierce, &item.Def.MultiArrow, &item.Def.AttackInterval, &item.Def.AttackRange, &item.Def.AttackDistance); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// CharEquipWithDef 装备实例+定义
type CharEquipWithDef struct {
	ID       int64        `json:"id"`
	SlotType string       `json:"slot_type"`
	Quantity int          `json:"quantity"` // 数量（弹药类使用）
	Def      EquipmentDef `json:"def"`
}

// CalcTotalStats 计算角色总属性（基础+装备加成）
func (s *Store) CalcTotalStats(ch *Character) (*Character, error) {
	equips, err := s.GetCharEquipments(ch.ID)
	if err != nil {
		return nil, err
	}
	result := *ch
	for _, eq := range equips {
		result.Attack += eq.Def.Attack
		result.Defense += eq.Def.Defense
		result.MaxHP += eq.Def.HP
		result.HP += eq.Def.HP
		result.Speed += eq.Def.Speed
		result.CritRate += eq.Def.CritRate
	}
	return &result, nil
}

// GetSkillsByClass 获取职业技能列表
func (s *Store) GetSkillsByClass(class string) ([]SkillDef, error) {
	rows, err := s.db.Query(
		`SELECT id,name,class,damage,mp_cost,cooldown,range_val,area_type,area_size
		 FROM skill_defs WHERE class=? ORDER BY id`, class,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SkillDef
	for rows.Next() {
		var sk SkillDef
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Class, &sk.Damage, &sk.MPCost,
			&sk.Cooldown, &sk.Range, &sk.AreaType, &sk.AreaSize); err != nil {
			return nil, err
		}
		result = append(result, sk)
	}
	return result, nil
}
