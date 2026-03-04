// Package store 提供 Demo 的数据存储层，支持 SQLite/MySQL 切换。
package store

import "time"

// Account 用户账号
type Account struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // 哈希后的密码
	CreatedAt time.Time `json:"created_at"`
}

// Character 角色数据
type Character struct {
	ID        int64   `json:"id"`
	AccountID int64   `json:"account_id"`
	Name      string  `json:"name"`
	Class     string  `json:"class"`     // warrior, archer
	Level     int     `json:"level"`
	Exp       int64   `json:"exp"`
	HP        float64 `json:"hp"`
	MaxHP     float64 `json:"max_hp"`
	MP        float64 `json:"mp"`
	MaxMP     float64 `json:"max_mp"`
	Attack    float64 `json:"attack"`
	Defense   float64 `json:"defense"`
	Speed     float64 `json:"speed"`
	CritRate  float64 `json:"crit_rate"`
}

// EquipmentDef 装备定义（模板）
type EquipmentDef struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	SlotType      string  `json:"slot_type"` // weapon, helmet, armor, boots
	Class         string  `json:"class"`     // warrior, archer, all
	Quality       string  `json:"quality"`   // normal, rare, epic
	Level         int     `json:"level"`
	Attack        float64 `json:"attack"`
	Defense       float64 `json:"defense"`
	HP            float64 `json:"hp"`
	Speed         float64 `json:"speed"`
	CritRate      float64 `json:"crit_rate"`
	Pierce        int     `json:"pierce"`          // 穿透数（弓箭类）
	MultiArrow    int     `json:"multi_arrow"`     // 多重箭数（弓箭类）
	AttackInterval float64 `json:"attack_interval"` // 攻击间隔（秒）
	AttackRange   float64 `json:"attack_range"`    // 攻击范围（像素）
	AttackDistance float64 `json:"attack_distance"` // 攻击距离（像素）
}

// CharEquipment 角色已装备的装备
type CharEquipment struct {
	ID          int64  `json:"id"`
	CharacterID int64  `json:"character_id"`
	EquipDefID  int64  `json:"equip_def_id"`
	SlotType    string `json:"slot_type"`
}

// SkillDef 技能定义
type SkillDef struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Class    string  `json:"class"`
	Damage   float64 `json:"damage"`
	MPCost   float64 `json:"mp_cost"`
	Cooldown float64 `json:"cooldown"` // 秒
	Range    float64 `json:"range"`
	AreaType string  `json:"area_type"` // single, circle, fan
	AreaSize float64 `json:"area_size"`
}
