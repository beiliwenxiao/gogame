package store

import "log"

// SeedDefaultEquipment 初始化默认装备和技能数据
func (s *Store) SeedDefaultEquipment() error {
	// 检查是否已有数据
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM equipment_defs").Scan(&count)
	if count > 0 {
		// 检查是否有新字段（attack_range），如果旧数据没有则需要更新
		var hasNewFields int
		s.db.QueryRow("SELECT COUNT(*) FROM equipment_defs WHERE attack_range > 0").Scan(&hasNewFields)
		if hasNewFields == 0 {
			// 旧数据，删除重建
			log.Println("检测到旧装备数据，重新初始化...")
			s.db.Exec("DELETE FROM equipment_defs")
			s.db.Exec("DELETE FROM skill_defs")
			s.db.Exec("DELETE FROM char_equipments")
		} else {
			return nil
		}
	}

	log.Println("初始化默认装备数据...")

	equipments := []EquipmentDef{
		// 战士武器（近战，攻击范围90，攻击距离100）
		{Name: "铁剑", SlotType: "weapon", Class: "warrior", Quality: "normal", Level: 1, Attack: 8,
			AttackInterval: 1.0, AttackRange: 90, AttackDistance: 100},
		{Name: "钢剑", SlotType: "weapon", Class: "warrior", Quality: "rare", Level: 1, Attack: 15, CritRate: 0.03,
			AttackInterval: 0.9, AttackRange: 90, AttackDistance: 100},
		{Name: "烈焰大剑", SlotType: "weapon", Class: "warrior", Quality: "epic", Level: 1, Attack: 25, CritRate: 0.05,
			AttackInterval: 0.8, AttackRange: 100, AttackDistance: 110},
		// 弓箭手武器（远程，攻击范围200，攻击距离250，有穿透和多重箭属性）
		{Name: "短弓", SlotType: "weapon", Class: "archer", Quality: "normal", Level: 1, Attack: 6, Speed: 10,
			Pierce: 1, MultiArrow: 1, AttackInterval: 1.5, AttackRange: 200, AttackDistance: 250},
		{Name: "长弓", SlotType: "weapon", Class: "archer", Quality: "rare", Level: 1, Attack: 12, Speed: 15, CritRate: 0.05,
			Pierce: 2, MultiArrow: 2, AttackInterval: 1.3, AttackRange: 220, AttackDistance: 270},
		{Name: "暗影之弓", SlotType: "weapon", Class: "archer", Quality: "epic", Level: 1, Attack: 20, Speed: 20, CritRate: 0.1,
			Pierce: 3, MultiArrow: 3, AttackInterval: 1.5, AttackRange: 250, AttackDistance: 300},
		// 通用防具
		{Name: "皮甲", SlotType: "armor", Class: "all", Quality: "normal", Level: 1, Defense: 5, HP: 20},
		{Name: "锁子甲", SlotType: "armor", Class: "all", Quality: "rare", Level: 1, Defense: 12, HP: 50},
		{Name: "皮靴", SlotType: "boots", Class: "all", Quality: "normal", Level: 1, Defense: 2, Speed: 15},
		{Name: "疾风靴", SlotType: "boots", Class: "all", Quality: "rare", Level: 1, Defense: 4, Speed: 30},
		{Name: "铁盔", SlotType: "helmet", Class: "all", Quality: "normal", Level: 1, Defense: 3, HP: 15},
		{Name: "精钢盔", SlotType: "helmet", Class: "all", Quality: "rare", Level: 1, Defense: 7, HP: 35},
	}

	for _, eq := range equipments {
		_, err := s.db.Exec(
			`INSERT INTO equipment_defs (name,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate,
			 pierce,multi_arrow,attack_interval,attack_range,attack_distance)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			eq.Name, eq.SlotType, eq.Class, eq.Quality, eq.Level,
			eq.Attack, eq.Defense, eq.HP, eq.Speed, eq.CritRate,
			eq.Pierce, eq.MultiArrow, eq.AttackInterval, eq.AttackRange, eq.AttackDistance,
		)
		if err != nil {
			return err
		}
	}

	log.Println("初始化默认技能数据...")

	skills := []SkillDef{
		// 战士技能
		{Name: "普通攻击", Class: "warrior", Damage: 1.0, MPCost: 0, Cooldown: 0.8, Range: 50, AreaType: "single"},
		{Name: "猛击", Class: "warrior", Damage: 3.0, MPCost: 15, Cooldown: 3.0, Range: 50, AreaType: "single"},
		{Name: "旋风斩", Class: "warrior", Damage: 1.5, MPCost: 25, Cooldown: 5.0, Range: 60, AreaType: "circle", AreaSize: 80},
		{Name: "战吼", Class: "warrior", Damage: 0.3, MPCost: 20, Cooldown: 10.0, Range: 0, AreaType: "circle", AreaSize: 120},
		// 弓箭手技能
		{Name: "射击", Class: "archer", Damage: 1.0, MPCost: 0, Cooldown: 0.6, Range: 200, AreaType: "single"},
		{Name: "多重射击", Class: "archer", Damage: 0.8, MPCost: 20, Cooldown: 4.0, Range: 180, AreaType: "circle", AreaSize: 10},
		{Name: "闪电箭", Class: "archer", Damage: 2.5, MPCost: 30, Cooldown: 6.0, Range: 250, AreaType: "circle", AreaSize: 20},
		{Name: "天降箭雨", Class: "archer", Damage: 1.8, MPCost: 40, Cooldown: 8.0, Range: 300, AreaType: "circle", AreaSize: 30},
	}

	for _, sk := range skills {
		_, err := s.db.Exec(
			`INSERT INTO skill_defs (name,class,damage,mp_cost,cooldown,range_val,area_type,area_size)
			 VALUES (?,?,?,?,?,?,?,?)`,
			sk.Name, sk.Class, sk.Damage, sk.MPCost, sk.Cooldown, sk.Range, sk.AreaType, sk.AreaSize,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
