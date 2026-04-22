package database

import "database/sql"

func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS admins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS smb_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			is_temporary INTEGER NOT NULL DEFAULT 0,
			expires_at DATETIME,
			system_user_created INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS volumes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			share_name TEXT NOT NULL UNIQUE,
			path TEXT NOT NULL UNIQUE,
			comment TEXT NOT NULL DEFAULT '',
			browseable INTEGER NOT NULL DEFAULT 1,
			read_only INTEGER NOT NULL DEFAULT 0,
			guest_ok INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES smb_users(id) ON DELETE CASCADE,
			volume_id INTEGER NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
			access TEXT NOT NULL DEFAULT 'read',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(user_id, volume_id)
		);`,
		`CREATE TABLE IF NOT EXISTS system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	alters := []string{
		`ALTER TABLE smb_users ADD COLUMN is_temporary INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE smb_users ADD COLUMN expires_at DATETIME`,
		`ALTER TABLE smb_users ADD COLUMN system_user_created INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil {
			// SQLite returns duplicate column errors for existing databases; ignore them.
			continue
		}
	}
	defaults := map[string]string{
		"initialized":       "false",
		"smb_workgroup":     "WORKGROUP",
		"smb_server_string": "SMB Controller",
		"smb_netbios_name":  "",
		"last_reload_at":    "",
	}
	for key, value := range defaults {
		if _, err := db.Exec(`INSERT OR IGNORE INTO system_settings(key, value, updated_at) VALUES(?, ?, datetime('now'))`, key, value); err != nil {
			return err
		}
	}
	return nil
}
