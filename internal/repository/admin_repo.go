package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"smb-controller/internal/models"
)

type AdminRepo struct {
	db *sql.DB
}

func (r *AdminRepo) IsInitialized(ctx context.Context) (bool, error) {
	value, err := (&SettingsRepo{db: r.db}).Get(ctx, "initialized")
	if err != nil {
		return false, err
	}
	return value == "true", nil
}

func (r *AdminRepo) Create(ctx context.Context, username, passwordHash string) (*models.Admin, error) {
	ts := now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO admins(username, password, created_at, updated_at) VALUES(?, ?, ?, ?)`, username, passwordHash, ts.Format(time.RFC3339), ts.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &models.Admin{ID: id, Username: username, Password: passwordHash, CreatedAt: ts, UpdatedAt: ts}, nil
}

func (r *AdminRepo) GetByUsername(ctx context.Context, username string) (*models.Admin, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, username, password, created_at, updated_at FROM admins WHERE username = ?`, username)
	var admin models.Admin
	var createdRaw, updatedRaw any
	if err := row.Scan(&admin.ID, &admin.Username, &admin.Password, &createdRaw, &updatedRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	admin.CreatedAt = scanTime(createdRaw)
	admin.UpdatedAt = scanTime(updatedRaw)
	return &admin, nil
}
