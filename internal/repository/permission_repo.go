package repository

import (
	"context"
	"database/sql"
	"time"

	"smb-controller/internal/models"
)

type PermissionRepo struct {
	db *sql.DB
}

func (r *PermissionRepo) Upsert(ctx context.Context, userID, volumeID int64, access string) error {
	ts := now().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, `INSERT INTO permissions(user_id, volume_id, access, created_at, updated_at) VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(user_id, volume_id) DO UPDATE SET access = excluded.access, updated_at = excluded.updated_at`, userID, volumeID, access, ts, ts)
	return err
}

func (r *PermissionRepo) Delete(ctx context.Context, userID, volumeID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM permissions WHERE user_id = ? AND volume_id = ?`, userID, volumeID)
	return err
}

func (r *PermissionRepo) List(ctx context.Context, userID, volumeID int64) ([]models.PermissionView, error) {
	query := `SELECT p.id, p.user_id, u.username, u.display_name, p.volume_id, v.share_name, v.path, p.access
		FROM permissions p
		JOIN smb_users u ON u.id = p.user_id
		JOIN volumes v ON v.id = p.volume_id
		WHERE 1 = 1`
	var args []any
	if userID > 0 {
		query += ` AND p.user_id = ?`
		args = append(args, userID)
	}
	if volumeID > 0 {
		query += ` AND p.volume_id = ?`
		args = append(args, volumeID)
	}
	query += ` ORDER BY u.username, v.share_name`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var permissions []models.PermissionView
	for rows.Next() {
		var p models.PermissionView
		if err := rows.Scan(&p.ID, &p.UserID, &p.Username, &p.DisplayName, &p.VolumeID, &p.ShareName, &p.VolumePath, &p.Access); err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

func (r *PermissionRepo) ForConfig(ctx context.Context) (map[int64][]models.PermissionView, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT p.id, p.user_id, u.username, u.display_name, p.volume_id, v.share_name, v.path, p.access
		FROM permissions p
		JOIN smb_users u ON u.id = p.user_id
		JOIN volumes v ON v.id = p.volume_id
		WHERE u.enabled = 1 AND v.enabled = 1 AND p.access IN ('read', 'readwrite')
		ORDER BY u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]models.PermissionView)
	for rows.Next() {
		var p models.PermissionView
		if err := rows.Scan(&p.ID, &p.UserID, &p.Username, &p.DisplayName, &p.VolumeID, &p.ShareName, &p.VolumePath, &p.Access); err != nil {
			return nil, err
		}
		out[p.VolumeID] = append(out[p.VolumeID], p)
	}
	return out, rows.Err()
}

func (r *PermissionRepo) CountByVolume(ctx context.Context, volumeID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM permissions WHERE volume_id = ? AND access <> 'none'`, volumeID).Scan(&count)
	return count, err
}

func (r *PermissionRepo) CountByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM permissions WHERE user_id = ? AND access <> 'none'`, userID).Scan(&count)
	return count, err
}

func nullPermission(err error) bool {
	return err == sql.ErrNoRows
}
