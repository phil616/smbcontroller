package models

import "time"

const (
	AccessRead      = "read"
	AccessReadWrite = "readwrite"
	AccessNone      = "none"
)

type Admin struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SmbUser struct {
	ID                int64      `json:"id"`
	Username          string     `json:"username"`
	DisplayName       string     `json:"display_name"`
	Enabled           bool       `json:"enabled"`
	IsTemporary       bool       `json:"is_temporary"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	SystemUserCreated bool       `json:"system_user_created"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Volume struct {
	ID         int64     `json:"id"`
	ShareName  string    `json:"share_name"`
	Path       string    `json:"path"`
	Comment    string    `json:"comment"`
	Browseable bool      `json:"browseable"`
	ReadOnly   bool      `json:"read_only"`
	GuestOK    bool      `json:"guest_ok"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Permission struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VolumeID  int64     `json:"volume_id"`
	Access    string    `json:"access"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PermissionView struct {
	ID          int64  `json:"id,omitempty"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	VolumeID    int64  `json:"volume_id"`
	ShareName   string `json:"share_name"`
	VolumePath  string `json:"volume_path,omitempty"`
	Access      string `json:"access"`
	DisplayName string `json:"display_name,omitempty"`
}

type VolumeDetail struct {
	Volume
	Permissions []PermissionView `json:"permissions"`
}

type UserDetail struct {
	SmbUser
	Permissions []PermissionView `json:"permissions"`
}

type SystemSettings struct {
	Workgroup    string `json:"smb_workgroup"`
	ServerString string `json:"smb_server_string"`
	NetbiosName  string `json:"smb_netbios_name"`
}
