package store

import "log"

// SeedDefaultEquipment 初始化默认装备和技能数据
func (s *Store) SeedDefaultEquipment() error {
	// 检查是否已有数据
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM equipment_defs").Scan(&count)
	if count > 0 {
		return nil
	}

	log.Println("初始化默认装备数据...")

	equipments := []EquipmentDef{
		// 战士武器
		{Name: "铁剑", SlotType: "weapon", Class: "warrior", Quality: "normal", Level: 1, Attack: 8},
		{Name: "钢剑", SlotType: "weapon", Class: "warrior", Quality: "rare", Level: 1, Attack: 15, CritRate: 0.03},
		{Name: "烈焰大剑", SlotType: "weapon", Class: "warrior", Quality: "epic", Level: 1, Attack: 25, CritRate: 0.05},
		// 弓箭手武器
		{Name: "短弓", SlotType: "weapon", Class: "archer", Quality: "normal", Level: 1, Attack: 6, Speed: 10},
		{Name: "长弓", SlotType: "weapon", Class: "archer", Quality: "rare", Level: 1, Attack: 12, Speed: 15, CritRate: 0.05},
		{Name: "暗影之弓", SlotType: "weapon", Class: "archer", Quality: "epic", Level: 1, Attack: 20, Speed: 20, CritRate: 0.1},
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
			`INSERT INTO equipment_defs (name,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			eq.Name, eq.SlotType, eq.Class, eq.Quality, eq.Level,
			eq.Attack, eq.Defense, eq.HP, eq.Speed, eq.CritRate,
		)
		if err != nil {
			return err
		}
	}

	log.Println("初始化默认技能数据...")

	skills := []SkillDef{
		// 战士技能
		{Name: "普通攻击", Class: "warrior", Damage: 1.0, MPCost: 0, Cooldown: 0.8, Range: 50, AreaType: "single"},
		{Name: "猛击", Class: "warrior", Damage: 2.0, MPCost: 15, Cooldown: 3.0, Range: 50, AreaType: "single"},
		{Name: "旋风斩", Class: "warrior", Damage: 1.5, MPCost: 25, Cooldown: 5.0, Range: 60, AreaType: "circle", AreaSize: 80},
		{Name: "战吼", Class: "warrior", Damage: 0, MPCost: 20, Cooldown: 10.0, Range: 0, AreaType: "circle", AreaSize: 120},
		// 弓箭手技能
		{Name: "射击", Class: "archer", Damage: 1.0, MPCost: 0, Cooldown: 0.6, Range: 200, AreaType: "single"},
		{Name: "多重射击", Class: "archer", Damage: 0.8, MPCost: 20, Cooldown: 4.0, Range: 180, AreaType: "fan", AreaSize: 60},
		{Name: "穿透箭", Class: "archer", Damage: 2.5, MPCost: 30, Cooldown: 6.0, Range: 250, AreaType: "single"},
		{Name: "闪避翻滚", Class: "archer", Damage: 0, MPCost: 15, Cooldown: 8.0, Range: 0, AreaType: "single"},
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
