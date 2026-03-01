package store

import "fmt"

// AutoMigrate 自动创建数据库表
func (s *Store) AutoMigrate() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS characters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			class TEXT NOT NULL DEFAULT 'warrior',
			level INTEGER DEFAULT 1,
			exp INTEGER DEFAULT 0,
			hp REAL DEFAULT 100,
			max_hp REAL DEFAULT 100,
			mp REAL DEFAULT 50,
			max_mp REAL DEFAULT 50,
			attack REAL DEFAULT 10,
			defense REAL DEFAULT 5,
			speed REAL DEFAULT 150,
			crit_rate REAL DEFAULT 0.05,
			FOREIGN KEY (account_id) REFERENCES accounts(id)
		)`,
		`CREATE TABLE IF NOT EXISTS equipment_defs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slot_type TEXT NOT NULL,
			class TEXT DEFAULT 'all',
			quality TEXT DEFAULT 'normal',
			level INTEGER DEFAULT 1,
			attack REAL DEFAULT 0,
			defense REAL DEFAULT 0,
			hp REAL DEFAULT 0,
			speed REAL DEFAULT 0,
			crit_rate REAL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS char_equipments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id INTEGER NOT NULL,
			equip_def_id INTEGER NOT NULL,
			slot_type TEXT NOT NULL,
			FOREIGN KEY (character_id) REFERENCES characters(id),
			FOREIGN KEY (equip_def_id) REFERENCES equipment_defs(id)
		)`,
		`CREATE TABLE IF NOT EXISTS skill_defs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			class TEXT NOT NULL,
			damage REAL DEFAULT 0,
			mp_cost REAL DEFAULT 0,
			cooldown REAL DEFAULT 1.0,
			range_val REAL DEFAULT 40,
			area_type TEXT DEFAULT 'single',
			area_size REAL DEFAULT 0
		)`,
	}
	for _, ddl := range tables {
		if _, err := s.db.Exec(ddl); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}
