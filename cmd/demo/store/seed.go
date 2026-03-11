package store

import "log"

// SeedDefaultEquipment 初始化默认装备和技能数据
func (s *Store) SeedDefaultEquipment() error {
	// 检查是否已有数据
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM equipment_defs").Scan(&count)
	if count > 0 {
		// 检查是否有 icon_id 字段数据
		var hasIconID int
		s.db.QueryRow("SELECT COUNT(*) FROM equipment_defs WHERE icon_id != '' AND icon_id IS NOT NULL").Scan(&hasIconID)
		// 检查技能数据版本：猛击的 area_type 是否已更新为 fan
		var skillVersion string
		s.db.QueryRow("SELECT area_type FROM skill_defs WHERE name='猛击' LIMIT 1").Scan(&skillVersion)
		// 检查战吼伤害是否已更新为 0.3
		var warcryDmg float64
		s.db.QueryRow("SELECT damage FROM skill_defs WHERE name='战吼' LIMIT 1").Scan(&warcryDmg)
		// 检查弓箭手技能是否已更新为闪电箭
		var archerSkillCount int
		s.db.QueryRow("SELECT COUNT(*) FROM skill_defs WHERE name='闪电箭'").Scan(&archerSkillCount)
		// 检查防具防御值是否已更新（皮甲 defense >= 15）
		var leatherDef int
		s.db.QueryRow("SELECT defense FROM equipment_defs WHERE name='皮甲' LIMIT 1").Scan(&leatherDef)
		// 检查铁箭是否已添加
		var ironArrowCount int
		s.db.QueryRow("SELECT COUNT(*) FROM equipment_defs WHERE name='铁箭'").Scan(&ironArrowCount)
		// 检查弓箭手武器攻击角度是否已更新（短弓 attack_range = 30）
		var bowRange float64
		s.db.QueryRow("SELECT attack_range FROM equipment_defs WHERE name='短弓' LIMIT 1").Scan(&bowRange)
		if hasIconID == 0 || skillVersion != "fan" || warcryDmg < 0.29 || archerSkillCount == 0 || leatherDef < 15 || ironArrowCount == 0 || bowRange > 50 {
			// 旧数据，删除重建
			log.Println("检测到旧数据，重新初始化...")
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
		{Name: "铁剑", IconID: "iron_sword", SlotType: "weapon", Class: "warrior", Quality: "normal", Level: 1, Attack: 18,
			AttackInterval: 1.0, AttackRange: 90, AttackDistance: 100},
		{Name: "钢剑", IconID: "steel_sword", SlotType: "weapon", Class: "warrior", Quality: "rare", Level: 1, Attack: 45, CritRate: 0.03,
			AttackInterval: 0.9, AttackRange: 90, AttackDistance: 100},
		{Name: "烈焰大剑", IconID: "flame_sword", SlotType: "weapon", Class: "warrior", Quality: "epic", Level: 1, Attack: 125, CritRate: 0.05,
			AttackInterval: 0.8, AttackRange: 100, AttackDistance: 110},
		// 弓箭手武器（远程，攻击角度30度，攻击距离250，有穿透和多重箭属性）
		{Name: "短弓", IconID: "short_bow", SlotType: "weapon", Class: "archer", Quality: "normal", Level: 1, Attack: 16, Speed: 10,
			Pierce: 1, MultiArrow: 1, AttackInterval: 1.5, AttackRange: 30, AttackDistance: 250},
		{Name: "长弓", IconID: "long_bow", SlotType: "weapon", Class: "archer", Quality: "rare", Level: 1, Attack: 32, Speed: 15, CritRate: 0.05,
			Pierce: 2, MultiArrow: 2, AttackInterval: 1.3, AttackRange: 30, AttackDistance: 270},
		{Name: "暗影之弓", IconID: "shadow_bow", SlotType: "weapon", Class: "archer", Quality: "epic", Level: 1, Attack: 80, Speed: 20, CritRate: 0.1,
			Pierce: 3, MultiArrow: 3, AttackInterval: 1.5, AttackRange: 30, AttackDistance: 300},
		// 通用防具
		{Name: "皮甲", IconID: "leather_armor", SlotType: "armor", Class: "all", Quality: "normal", Level: 1, Defense: 15, HP: 30},
		{Name: "锁子甲", IconID: "chain_mail", SlotType: "armor", Class: "all", Quality: "rare", Level: 1, Defense: 30, HP: 60},
		{Name: "皮靴", IconID: "leather_boots", SlotType: "boots", Class: "all", Quality: "normal", Level: 1, Defense: 8, Speed: 15},
		{Name: "疾风靴", IconID: "swift_boots", SlotType: "boots", Class: "all", Quality: "rare", Level: 1, Defense: 15, Speed: 30},
		{Name: "铁盔", IconID: "iron_helmet", SlotType: "helmet", Class: "all", Quality: "normal", Level: 1, Defense: 10, HP: 20},
		{Name: "精钢盔", IconID: "steel_helmet", SlotType: "helmet", Class: "all", Quality: "rare", Level: 1, Defense: 20, HP: 40},
		// 弓箭手弹药
		{Name: "木箭", IconID: "wooden_arrow", SlotType: "ammo", Class: "archer", Quality: "normal", Level: 1},
		{Name: "铁箭", IconID: "iron_arrow", SlotType: "ammo", Class: "archer", Quality: "normal", Level: 1, Attack: 2},
	}

	for _, eq := range equipments {
		_, err := s.db.Exec(
			`INSERT INTO equipment_defs (name,icon_id,slot_type,class,quality,level,attack,defense,hp,speed,crit_rate,
			 pierce,multi_arrow,attack_interval,attack_range,attack_distance)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			eq.Name, eq.IconID, eq.SlotType, eq.Class, eq.Quality, eq.Level,
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
		// 猛击：200% 伤害，扇形范围与普攻一致（由后端动态读取武器范围）
		{Name: "猛击", Class: "warrior", Damage: 2.0, MPCost: 15, Cooldown: 3.0, Range: 0, AreaType: "fan"},
		// 旋风斩：80% 伤害/秒，以玩家为中心的椭圆范围（武器距离），后端动态设置 AreaSize
		{Name: "旋风斩", Class: "warrior", Damage: 0.8, MPCost: 25, Cooldown: 5.0, Range: 0, AreaType: "ellipse", AreaSize: 0},
		// 战吼：30% 伤害 + 恐惧逃跑3秒，椭圆范围 = 武器距离×3，后端动态设置 AreaSize
		{Name: "战吼", Class: "warrior", Damage: 0.3, MPCost: 20, Cooldown: 10.0, Range: 0, AreaType: "ellipse", AreaSize: 0},
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

// GrantInitialEquipments 给新角色发放初始装备（普通品质一套）
func (s *Store) GrantInitialEquipments(charID int64, class string) error {
	// 查找该职业可用的普通品质装备（每个槽位取第一个，排除弹药）
	rows, err := s.db.Query(
		`SELECT id, slot_type FROM equipment_defs
		 WHERE (class=? OR class='all') AND quality='normal' AND slot_type != 'ammo'
		 ORDER BY slot_type, id`,
		class,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	equipped := map[string]bool{}
	for rows.Next() {
		var defID int64
		var slotType string
		if err := rows.Scan(&defID, &slotType); err != nil {
			continue
		}
		// 每个槽位只装备一件
		if equipped[slotType] {
			continue
		}
		equipped[slotType] = true
		s.db.Exec(
			"INSERT INTO char_equipments (character_id, equip_def_id, slot_type, quantity) VALUES (?,?,?,1)",
			charID, defID, slotType,
		)
	}

	// 弓箭手额外发放4捆铁箭（每捆99支）：1捆装备副手 + 3捆放背包
	if class == "archer" {
		var ironArrowID int64
		s.db.QueryRow("SELECT id FROM equipment_defs WHERE name='铁箭' LIMIT 1").Scan(&ironArrowID)
		if ironArrowID > 0 {
			for i := 0; i < 4; i++ {
				s.db.Exec(
					"INSERT INTO char_equipments (character_id, equip_def_id, slot_type, quantity) VALUES (?,?,'ammo',99)",
					charID, ironArrowID,
				)
			}
		}
	}

	log.Printf("角色 %d 发放初始装备: %v", charID, equipped)
	return nil
}
