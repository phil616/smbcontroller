package repository

import (
	"context"
	"database/sql"
	"time"

	"smb-controller/internal/models"
)

type SettingsRepo struct {
	db *sql.DB
}

func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO system_settings(key, value, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now().Format(time.RFC3339))
	return err
}

func (r *SettingsRepo) All(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func (r *SettingsRepo) SystemSettings(ctx context.Context) (models.SystemSettings, error) {
	values, err := r.All(ctx)
	if err != nil {
		return models.SystemSettings{}, err
	}
	return models.SystemSettings{
		Workgroup:    values["smb_workgroup"],
		ServerString: values["smb_server_string"],
		NetbiosName:  values["smb_netbios_name"],
	}, nil
}

func (r *SettingsRepo) UpdateSystemSettings(ctx context.Context, s models.SystemSettings) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now().Format(time.RFC3339)
	for key, value := range map[string]string{
		"smb_workgroup":     s.Workgroup,
		"smb_server_string": s.ServerString,
		"smb_netbios_name":  s.NetbiosName,
	} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO system_settings(key, value, updated_at) VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}
