package store

import "fmt"

// GetCharInventory 获取角色背包
func (s *Store) GetCharInventory(charID int64) ([]CharInventoryItem, error) {
	rows, err := s.db.Query(
		`SELECT ci.id, ci.equip_def_id, ci.quantity, ci.slot_index,
		        ed.id, ed.name, ed.icon_id, ed.slot_type, ed.class, ed.quality, ed.level,
		        ed.attack, ed.defense, ed.hp, ed.speed, ed.crit_rate,
		        ed.pierce, ed.multi_arrow, ed.attack_interval, ed.attack_range, ed.attack_distance
		 FROM char_inventory ci
		 JOIN equipment_defs ed ON ci.equip_def_id = ed.id
		 WHERE ci.character_id=?
		 ORDER BY ci.slot_index`, charID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CharInventoryItem
	for rows.Next() {
		var item CharInventoryItem
		item.CharacterID = charID
		if err := rows.Scan(
			&item.ID, &item.EquipDefID, &item.Quantity, &item.SlotIndex,
			&item.Def.ID, &item.Def.Name, &item.Def.IconID, &item.Def.SlotType, &item.Def.Class, &item.Def.Quality, &item.Def.Level,
			&item.Def.Attack, &item.Def.Defense, &item.Def.HP, &item.Def.Speed, &item.Def.CritRate,
			&item.Def.Pierce, &item.Def.MultiArrow, &item.Def.AttackInterval, &item.Def.AttackRange, &item.Def.AttackDistance,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// AddToInventory 添加物品到背包（自动堆叠，maxStack=99 for ammo，1 for equipment）
func (s *Store) AddToInventory(charID, equipDefID int64, quantity int, maxStack int) error {
	if maxStack <= 0 {
		maxStack = 1
	}
	remaining := quantity
	// 先尝试堆叠到已有格子
	rows, err := s.db.Query(
		"SELECT id, quantity FROM char_inventory WHERE character_id=? AND equip_def_id=? ORDER BY slot_index",
		charID, equipDefID,
	)
	if err != nil {
		return err
	}
	type slot struct{ id int64; qty int }
	var slots []slot
	for rows.Next() {
		var sl slot
		rows.Scan(&sl.id, &sl.qty)
		slots = append(slots, sl)
	}
	rows.Close()

	for _, sl := range slots {
		if remaining <= 0 {
			break
		}
		canAdd := maxStack - sl.qty
		if canAdd <= 0 {
			continue
		}
		add := remaining
		if add > canAdd {
			add = canAdd
		}
		s.db.Exec("UPDATE char_inventory SET quantity=? WHERE id=?", sl.qty+add, sl.id)
		remaining -= add
	}

	// 剩余放新格子
	for remaining > 0 {
		add := remaining
		if add > maxStack {
			add = maxStack
		}
		_, err := s.db.Exec(
			"INSERT INTO char_inventory (character_id, equip_def_id, quantity, slot_index) VALUES (?,?,?,-1)",
			charID, equipDefID, add,
		)
		if err != nil {
			return fmt.Errorf("添加背包失败: %w", err)
		}
		remaining -= add
	}
	return nil
}

// RemoveFromInventory 从背包移除物品（按数量）
func (s *Store) RemoveFromInventory(charID, equipDefID int64, quantity int) error {
	rows, err := s.db.Query(
		"SELECT id, quantity FROM char_inventory WHERE character_id=? AND equip_def_id=? ORDER BY slot_index",
		charID, equipDefID,
	)
	if err != nil {
		return err
	}
	type slot struct{ id int64; qty int }
	var slots []slot
	for rows.Next() {
		var sl slot
		rows.Scan(&sl.id, &sl.qty)
		slots = append(slots, sl)
	}
	rows.Close()

	remaining := quantity
	for _, sl := range slots {
		if remaining <= 0 {
			break
		}
		if sl.qty <= remaining {
			s.db.Exec("DELETE FROM char_inventory WHERE id=?", sl.id)
			remaining -= sl.qty
		} else {
			s.db.Exec("UPDATE char_inventory SET quantity=? WHERE id=?", sl.qty-remaining, sl.id)
			remaining = 0
		}
	}
	return nil
}

// GetInventoryCount 获取背包中某物品的总数量
func (s *Store) GetInventoryCount(charID, equipDefID int64) int {
	var total int
	s.db.QueryRow(
		"SELECT COALESCE(SUM(quantity),0) FROM char_inventory WHERE character_id=? AND equip_def_id=?",
		charID, equipDefID,
	).Scan(&total)
	return total
}
