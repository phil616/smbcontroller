package repository

import "database/sql"

type Repositories struct {
	DB          *sql.DB
	Admins      *AdminRepo
	Users       *UserRepo
	Volumes     *VolumeRepo
	Permissions *PermissionRepo
	Settings    *SettingsRepo
}

func NewRepositories(db *sql.DB) *Repositories {
	return &Repositories{
		DB:          db,
		Admins:      &AdminRepo{db: db},
		Users:       &UserRepo{db: db},
		Volumes:     &VolumeRepo{db: db},
		Permissions: &PermissionRepo{db: db},
		Settings:    &SettingsRepo{db: db},
	}
}
