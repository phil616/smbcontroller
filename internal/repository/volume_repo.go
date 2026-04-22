package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"smb-controller/internal/models"
)

type VolumeRepo struct {
	db *sql.DB
}

func (r *VolumeRepo) Create(ctx context.Context, volume models.Volume) (*models.Volume, error) {
	ts := now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO volumes(share_name, path, comment, browseable, read_only, guest_ok, enabled, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		volume.ShareName, volume.Path, volume.Comment, boolInt(volume.Browseable), boolInt(volume.ReadOnly), boolInt(volume.GuestOK), boolInt(volume.Enabled), ts.Format(time.RFC3339), ts.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	volume.ID = id
	volume.CreatedAt = ts
	volume.UpdatedAt = ts
	return &volume, nil
}

func (r *VolumeRepo) List(ctx context.Context) ([]models.Volume, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, share_name, path, comment, browseable, read_only, guest_ok, enabled, created_at, updated_at FROM volumes ORDER BY share_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var volumes []models.Volume
	for rows.Next() {
		volume, err := scanVolume(rows)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, volume)
	}
	return volumes, rows.Err()
}

func (r *VolumeRepo) Enabled(ctx context.Context) ([]models.Volume, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, share_name, path, comment, browseable, read_only, guest_ok, enabled, created_at, updated_at FROM volumes WHERE enabled = 1 ORDER BY share_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var volumes []models.Volume
	for rows.Next() {
		volume, err := scanVolume(rows)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, volume)
	}
	return volumes, rows.Err()
}

func (r *VolumeRepo) Get(ctx context.Context, id int64) (*models.Volume, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, share_name, path, comment, browseable, read_only, guest_ok, enabled, created_at, updated_at FROM volumes WHERE id = ?`, id)
	volume, err := scanVolume(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &volume, nil
}

func (r *VolumeRepo) Update(ctx context.Context, volume models.Volume) (*models.Volume, error) {
	_, err := r.db.ExecContext(ctx, `UPDATE volumes SET share_name = ?, path = ?, comment = ?, browseable = ?, read_only = ?, guest_ok = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		volume.ShareName, volume.Path, volume.Comment, boolInt(volume.Browseable), boolInt(volume.ReadOnly), boolInt(volume.GuestOK), boolInt(volume.Enabled), now().Format(time.RFC3339), volume.ID)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, volume.ID)
}

func (r *VolumeRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM volumes WHERE id = ?`, id)
	return err
}

func (r *VolumeRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM volumes`).Scan(&count)
	return count, err
}

func scanVolume(row rowScanner) (models.Volume, error) {
	var volume models.Volume
	var browseable, readOnly, guestOK, enabled int
	var createdRaw, updatedRaw any
	err := row.Scan(&volume.ID, &volume.ShareName, &volume.Path, &volume.Comment, &browseable, &readOnly, &guestOK, &enabled, &createdRaw, &updatedRaw)
	volume.Browseable = intBool(browseable)
	volume.ReadOnly = intBool(readOnly)
	volume.GuestOK = intBool(guestOK)
	volume.Enabled = intBool(enabled)
	volume.CreatedAt = scanTime(createdRaw)
	volume.UpdatedAt = scanTime(updatedRaw)
	return volume, err
}
