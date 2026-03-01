package store

import (
	"database/sql"
	"fmt"
)

// 职业基础属性
var classBaseStats = map[string]Character{
	"warrior": {
		HP: 200, MaxHP: 200, MP: 60, MaxMP: 60,
		Attack: 18, Defense: 12, Speed: 130, CritRate: 0.05,
	},
	"archer": {
		HP: 130, MaxHP: 130, MP: 100, MaxMP: 100,
		Attack: 14, Defense: 6, Speed: 180, CritRate: 0.12,
	},
}

// CreateCharacter 创建角色
func (s *Store) CreateCharacter(accountID int64, name, class string) (*Character, error) {
	base, ok := classBaseStats[class]
	if !ok {
		return nil, fmt.Errorf("无效职业: %s", class)
	}
	result, err := s.db.Exec(
		`INSERT INTO characters (account_id,name,class,level,hp,max_hp,mp,max_mp,attack,defense,speed,crit_rate)
		 VALUES (?,?,?,1,?,?,?,?,?,?,?,?)`,
		accountID, name, class,
		base.HP, base.MaxHP, base.MP, base.MaxMP,
		base.Attack, base.Defense, base.Speed, base.CritRate,
	)
	if err != nil {
		return nil, fmt.Errorf("创建角色失败: %w", err)
	}
	id, _ := result.LastInsertId()
	ch := base
	ch.ID = id
	ch.AccountID = accountID
	ch.Name = name
	ch.Class = class
	ch.Level = 1
	return &ch, nil
}

// GetCharacterByAccount 获取账号下的角色
func (s *Store) GetCharacterByAccount(accountID int64) (*Character, error) {
	var ch Character
	err := s.db.QueryRow(
		`SELECT id,account_id,name,class,level,exp,hp,max_hp,mp,max_mp,attack,defense,speed,crit_rate
		 FROM characters WHERE account_id=? LIMIT 1`, accountID,
	).Scan(&ch.ID, &ch.AccountID, &ch.Name, &ch.Class, &ch.Level, &ch.Exp,
		&ch.HP, &ch.MaxHP, &ch.MP, &ch.MaxMP, &ch.Attack, &ch.Defense, &ch.Speed, &ch.CritRate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// GetCharacterByID 根据ID获取角色
func (s *Store) GetCharacterByID(charID int64) (*Character, error) {
	var ch Character
	err := s.db.QueryRow(
		`SELECT id,account_id,name,class,level,exp,hp,max_hp,mp,max_mp,attack,defense,speed,crit_rate
		 FROM characters WHERE id=?`, charID,
	).Scan(&ch.ID, &ch.AccountID, &ch.Name, &ch.Class, &ch.Level, &ch.Exp,
		&ch.HP, &ch.MaxHP, &ch.MP, &ch.MaxMP, &ch.Attack, &ch.Defense, &ch.Speed, &ch.CritRate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// UpdateCharacter 更新角色数据
func (s *Store) UpdateCharacter(ch *Character) error {
	_, err := s.db.Exec(
		`UPDATE characters SET level=?,exp=?,hp=?,max_hp=?,mp=?,max_mp=?,
		 attack=?,defense=?,speed=?,crit_rate=? WHERE id=?`,
		ch.Level, ch.Exp, ch.HP, ch.MaxHP, ch.MP, ch.MaxMP,
		ch.Attack, ch.Defense, ch.Speed, ch.CritRate, ch.ID,
	)
	return err
}
