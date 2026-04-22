package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"smb-controller/internal/models"
)

type UserRepo struct {
	db *sql.DB
}

func (r *UserRepo) Create(ctx context.Context, user models.SmbUser) (*models.SmbUser, error) {
	ts := now()
	var expiresAt any
	if user.ExpiresAt != nil {
		expiresAt = user.ExpiresAt.In(time.Local).Format(time.RFC3339)
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO smb_users(username, display_name, enabled, is_temporary, expires_at, system_user_created, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		user.Username, user.DisplayName, boolInt(user.Enabled), boolInt(user.IsTemporary), expiresAt, boolInt(user.SystemUserCreated), ts.Format(time.RFC3339), ts.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	user.ID = id
	user.CreatedAt = ts
	user.UpdatedAt = ts
	return &user, nil
}

func (r *UserRepo) List(ctx context.Context) ([]models.SmbUser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, display_name, enabled, is_temporary, expires_at, system_user_created, created_at, updated_at FROM smb_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.SmbUser
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *UserRepo) Get(ctx context.Context, id int64) (*models.SmbUser, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, username, display_name, enabled, is_temporary, expires_at, system_user_created, created_at, updated_at FROM smb_users WHERE id = ?`, id)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) Update(ctx context.Context, user models.SmbUser) (*models.SmbUser, error) {
	ts := now()
	_, err := r.db.ExecContext(ctx, `UPDATE smb_users SET display_name = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		user.DisplayName, boolInt(user.Enabled), ts.Format(time.RFC3339), user.ID)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, user.ID)
}

func (r *UserRepo) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE smb_users SET enabled = ?, updated_at = ? WHERE id = ?`, boolInt(enabled), now().Format(time.RFC3339), id)
	return err
}

func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM smb_users WHERE id = ?`, id)
	return err
}

func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM smb_users`).Scan(&count)
	return count, err
}

func (r *UserRepo) ExpiredTemporary(ctx context.Context, before time.Time) ([]models.SmbUser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, display_name, enabled, is_temporary, expires_at, system_user_created, created_at, updated_at
		FROM smb_users
		WHERE is_temporary = 1 AND expires_at IS NOT NULL AND expires_at <= ?
		ORDER BY expires_at`, before.In(time.Local).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.SmbUser
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (models.SmbUser, error) {
	var user models.SmbUser
	var enabled, isTemporary, systemUserCreated int
	var expiresRaw sql.NullString
	var createdRaw, updatedRaw any
	err := row.Scan(&user.ID, &user.Username, &user.DisplayName, &enabled, &isTemporary, &expiresRaw, &systemUserCreated, &createdRaw, &updatedRaw)
	user.Enabled = intBool(enabled)
	user.IsTemporary = intBool(isTemporary)
	user.SystemUserCreated = intBool(systemUserCreated)
	if expiresRaw.Valid && expiresRaw.String != "" {
		if expiresAt, parseErr := time.ParseInLocation(time.RFC3339, expiresRaw.String, time.Local); parseErr == nil {
			expiresAt = expiresAt.In(time.Local)
			user.ExpiresAt = &expiresAt
		}
	}
	user.CreatedAt = scanTime(createdRaw)
	user.UpdatedAt = scanTime(updatedRaw)
	return user, err
}
