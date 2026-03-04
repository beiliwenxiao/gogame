package store

import "fmt"

// GetEquipmentDefs 获取角色可用的装备列表
func (s *Store) GetEquipmentDefs(class string) ([]EquipmentDef, error) {
	rows, err := s.db.Query(
		`SELECT id,name,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate,
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
		if err := rows.Scan(&eq.ID, &eq.Name, &eq.SlotType, &eq.Class, &eq.Quality, &eq.Level,
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
		`SELECT id,name,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate,
		        pierce,multi_arrow,attack_interval,attack_range,attack_distance
		 FROM equipment_defs WHERE id=?`, id,
	).Scan(&eq.ID, &eq.Name, &eq.SlotType, &eq.Class, &eq.Quality, &eq.Level,
		&eq.Attack, &eq.Defense, &eq.HP, &eq.Speed, &eq.CritRate,
		&eq.Pierce, &eq.MultiArrow, &eq.AttackInterval, &eq.AttackRange, &eq.AttackDistance)
	if err != nil {
		return nil, err
	}
	return &eq, nil
}

// EquipItem 装备物品
func (s *Store) EquipItem(charID, equipDefID int64, slotType string) error {
	// 先卸下同槽位的装备
	s.db.Exec("DELETE FROM char_equipments WHERE character_id=? AND slot_type=?", charID, slotType)
	_, err := s.db.Exec(
		"INSERT INTO char_equipments (character_id, equip_def_id, slot_type) VALUES (?,?,?)",
		charID, equipDefID, slotType,
	)
	if err != nil {
		return fmt.Errorf("装备失败: %w", err)
	}
	return nil
}

// UnequipItem 卸下装备
func (s *Store) UnequipItem(charID int64, slotType string) error {
	_, err := s.db.Exec(
		"DELETE FROM char_equipments WHERE character_id=? AND slot_type=?",
		charID, slotType,
	)
	return err
}

// GetCharEquipments 获取角色已装备的装备
func (s *Store) GetCharEquipments(charID int64) ([]CharEquipWithDef, error) {
	rows, err := s.db.Query(
		`SELECT ce.id, ce.slot_type, ed.id, ed.name, ed.slot_type, ed.class, ed.quality, ed.level,
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
		if err := rows.Scan(&item.ID, &item.SlotType,
			&item.Def.ID, &item.Def.Name, &item.Def.SlotType, &item.Def.Class, &item.Def.Quality, &item.Def.Level,
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
