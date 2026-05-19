package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smb-controller/internal/models"
	"smb-controller/internal/repository"
	"smb-controller/internal/smb"
)

type SMBService struct {
	repos             *repository.Repositories
	executor          *smb.Executor
	generator         *smb.Generator
	allowedShareRoots []string
	serverMinProtocol string
	serverMaxProtocol string
	mu                sync.Mutex
}

type CreateVolumeRequest struct {
	ShareName            string `json:"share_name"`
	Path                 string `json:"path"`
	Comment              string `json:"comment"`
	Browseable           bool   `json:"browseable"`
	GuestOK              bool   `json:"guest_ok"`
	CreateDirIfNotExists bool   `json:"create_dir_if_not_exists"`
}

type UpdateVolumeRequest struct {
	ShareName  string `json:"share_name"`
	Path       string `json:"path"`
	Comment    string `json:"comment"`
	Browseable bool   `json:"browseable"`
	GuestOK    bool   `json:"guest_ok"`
	Enabled    bool   `json:"enabled"`
}

type CreateUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type CreateTemporaryUserRequest struct {
	DurationHours int     `json:"duration_hours"`
	VolumeIDs     []int64 `json:"volume_ids"`
	Access        string  `json:"access"`
}

type TemporaryUserResponse struct {
	User      *models.SmbUser `json:"user"`
	Username  string          `json:"username"`
	Password  string          `json:"password"`
	ExpiresAt time.Time       `json:"expires_at"`
}

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
}

type PermissionOperationResult struct {
	Success         bool   `json:"success"`
	UserID          int64  `json:"user_id,omitempty"`
	VolumeID        int64  `json:"volume_id,omitempty"`
	Access          string `json:"access"`
	UpdatedCount    int    `json:"updated_count,omitempty"`
	ConfigApplied   bool   `json:"config_applied"`
	SmbConfReloaded bool   `json:"smb_conf_reloaded"`
	ConfigError     string `json:"config_error,omitempty"`
}

func (s *SMBService) ListVolumes(ctx context.Context) ([]models.Volume, error) {
	return s.repos.Volumes.List(ctx)
}

func (s *SMBService) GetVolume(ctx context.Context, id int64) (*models.VolumeDetail, error) {
	volume, err := s.repos.Volumes.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if volume == nil {
		return nil, errors.New("volume not found")
	}
	perms, err := s.repos.Permissions.List(ctx, 0, id)
	if err != nil {
		return nil, err
	}
	return &models.VolumeDetail{Volume: *volume, Permissions: perms}, nil
}

func (s *SMBService) CreateVolume(ctx context.Context, req CreateVolumeRequest) (*models.Volume, error) {
	if err := validateShareName(req.ShareName); err != nil {
		return nil, err
	}
	if err := validatePath(req.Path); err != nil {
		return nil, err
	}
	if err := s.validateShareRoot(req.Path); err != nil {
		return nil, err
	}
	if err := validateConfigValue(req.Comment, "comment"); err != nil {
		return nil, err
	}
	if _, err := os.Stat(req.Path); err != nil {
		if os.IsNotExist(err) && req.CreateDirIfNotExists {
			if err := os.MkdirAll(req.Path, 0750); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if err := s.executor.EnsureSharePathPermissions(req.Path); err != nil {
		return nil, err
	}
	volume, err := s.repos.Volumes.Create(ctx, models.Volume{ShareName: req.ShareName, Path: req.Path, Comment: req.Comment, Browseable: req.Browseable, GuestOK: req.GuestOK, Enabled: true})
	if err != nil {
		return nil, err
	}
	if err := s.applyConfig(ctx); err != nil {
		return nil, err
	}
	return volume, nil
}

func (s *SMBService) UpdateVolume(ctx context.Context, id int64, req UpdateVolumeRequest) (*models.Volume, error) {
	if err := validateShareName(req.ShareName); err != nil {
		return nil, err
	}
	if err := validatePath(req.Path); err != nil {
		return nil, err
	}
	if err := s.validateShareRoot(req.Path); err != nil {
		return nil, err
	}
	if err := validateConfigValue(req.Comment, "comment"); err != nil {
		return nil, err
	}
	volume, err := s.repos.Volumes.Get(ctx, id)
	if err != nil || volume == nil {
		return nil, err
	}
	volume.ShareName = req.ShareName
	volume.Path = req.Path
	volume.Comment = req.Comment
	volume.Browseable = req.Browseable
	volume.GuestOK = req.GuestOK
	volume.Enabled = req.Enabled
	if err := s.executor.EnsureSharePathPermissions(req.Path); err != nil {
		return nil, err
	}
	updated, err := s.repos.Volumes.Update(ctx, *volume)
	if err != nil {
		return nil, err
	}
	if err := s.applyConfig(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *SMBService) DeleteVolume(ctx context.Context, id int64) error {
	if err := s.repos.Volumes.Delete(ctx, id); err != nil {
		return err
	}
	return s.applyConfig(ctx)
}

// RepairVolumePermissions force-runs the full recursive chown+chmod+ACL pass
// on a single share. This is the user-facing escape hatch for the
// "files created by another user are inaccessible" problem: it does the same
// work as CreateVolume's initial walk but on demand, so an operator can fix
// an existing share without deleting and recreating it.
func (s *SMBService) RepairVolumePermissions(ctx context.Context, id int64) (*models.Volume, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	volume, err := s.repos.Volumes.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if volume == nil {
		return nil, errors.New("volume not found")
	}
	if err := s.executor.EnsureSharePathPermissions(volume.Path); err != nil {
		return nil, err
	}
	return volume, nil
}

// RepairAllVolumes walks every enabled volume and re-applies ownership, mode,
// and ACL. Meant to be called once on service startup so that an operator who
// upgrades the binary and restarts the service automatically gets a clean
// permission state — they do not need to remember to click "repair" on each
// share. Failures on individual volumes are logged but do not abort the rest,
// since one broken mount should not prevent the others from being fixed.
func (s *SMBService) RepairAllVolumes(ctx context.Context) (repaired int, failed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	volumes, err := s.repos.Volumes.Enabled(ctx)
	if err != nil {
		log.Printf("startup repair: list volumes failed: %v", err)
		return 0, 0
	}
	for _, volume := range volumes {
		if err := s.executor.EnsureSharePathPermissions(volume.Path); err != nil {
			log.Printf("startup repair: volume %d (%s) at %s failed: %v", volume.ID, volume.ShareName, volume.Path, err)
			failed++
			continue
		}
		repaired++
	}
	return repaired, failed
}

func (s *SMBService) ListUsers(ctx context.Context) ([]models.SmbUser, error) {
	return s.repos.Users.List(ctx)
}

func (s *SMBService) GetUser(ctx context.Context, id int64) (*models.UserDetail, error) {
	user, err := s.repos.Users.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}
	perms, err := s.repos.Permissions.List(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	return &models.UserDetail{SmbUser: *user, Permissions: perms}, nil
}

func (s *SMBService) CreateUser(ctx context.Context, req CreateUserRequest) (*models.SmbUser, error) {
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := ValidatePassword(req.Password); err != nil {
		return nil, err
	}
	systemUserCreated, err := s.executor.CreateSystemUser(req.Username)
	if err != nil {
		return nil, err
	}
	if err := s.executor.AddUserToManagedGroup(req.Username); err != nil {
		return nil, err
	}
	if err := s.executor.SetSmbPassword(req.Username, req.Password); err != nil {
		return nil, err
	}
	user, err := s.repos.Users.Create(ctx, models.SmbUser{Username: req.Username, DisplayName: req.DisplayName, Enabled: true, SystemUserCreated: systemUserCreated})
	if err != nil {
		return nil, err
	}
	if err := s.applyConfig(ctx); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *SMBService) CreateTemporaryUser(ctx context.Context, req CreateTemporaryUserRequest) (*TemporaryUserResponse, error) {
	if req.DurationHours < 1 || req.DurationHours > 24*14 {
		return nil, errors.New("临时用户有效期必须在 1 小时到 14 天之间")
	}
	access := req.Access
	if access == "" {
		access = models.AccessRead
	}
	if err := validateAccess(access); err != nil {
		return nil, err
	}
	username := fmt.Sprintf("tmp_%s", randomToken(8))
	password := temporaryPassword()
	expiresAt := time.Now().In(time.Local).Add(time.Duration(req.DurationHours) * time.Hour).Truncate(time.Second)
	systemUserCreated, err := s.executor.CreateSystemUser(username)
	if err != nil {
		return nil, err
	}
	if err := s.executor.SetSmbPassword(username, password); err != nil {
		return nil, err
	}
	user, err := s.repos.Users.Create(ctx, models.SmbUser{
		Username:          username,
		DisplayName:       "临时用户",
		Enabled:           true,
		IsTemporary:       true,
		ExpiresAt:         &expiresAt,
		SystemUserCreated: systemUserCreated,
	})
	if err != nil {
		return nil, err
	}
	if len(req.VolumeIDs) > 0 && access != models.AccessNone {
		for _, volumeID := range req.VolumeIDs {
			if _, err := s.SetPermission(ctx, user.ID, volumeID, access); err != nil {
				return nil, err
			}
		}
	} else if err := s.applyConfig(ctx); err != nil {
		return nil, err
	}
	return &TemporaryUserResponse{User: user, Username: username, Password: password, ExpiresAt: expiresAt}, nil
}

func (s *SMBService) UpdateUser(ctx context.Context, id int64, req UpdateUserRequest) (*models.SmbUser, error) {
	user, err := s.repos.Users.Get(ctx, id)
	if err != nil || user == nil {
		return nil, err
	}
	user.DisplayName = req.DisplayName
	user.Enabled = req.Enabled
	updated, err := s.repos.Users.Update(ctx, *user)
	if err != nil {
		return nil, err
	}
	if err := s.executor.SetSmbUserEnabled(user.Username, req.Enabled); err != nil {
		return nil, err
	}
	if err := s.applyConfig(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *SMBService) ChangeUserPassword(ctx context.Context, id int64, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	user, err := s.repos.Users.Get(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}
	return s.executor.SetSmbPassword(user.Username, password)
}

func (s *SMBService) SetUserEnabled(ctx context.Context, id int64, enabled bool) error {
	user, err := s.repos.Users.Get(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}
	if err := s.executor.SetSmbUserEnabled(user.Username, enabled); err != nil {
		return err
	}
	if err := s.repos.Users.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	return s.applyConfig(ctx)
}

func (s *SMBService) DeleteUser(ctx context.Context, id int64) error {
	user, err := s.repos.Users.Get(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}
	_ = s.executor.DeleteSmbUser(user.Username)
	if err := s.repos.Users.Delete(ctx, id); err != nil {
		return err
	}
	if err := s.applyConfig(ctx); err != nil {
		return err
	}
	if user.SystemUserCreated {
		return s.executor.DeleteSystemUser(user.Username)
	}
	return nil
}

func (s *SMBService) CleanupExpiredTemporaryUsers(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired, err := s.repos.Users.ExpiredTemporary(ctx, time.Now().In(time.Local))
	if err != nil {
		return 0, err
	}
	for _, user := range expired {
		_ = s.executor.DeleteSmbUser(user.Username)
		if err := s.repos.Users.Delete(ctx, user.ID); err != nil {
			return 0, err
		}
		if user.SystemUserCreated {
			_ = s.executor.DeleteSystemUser(user.Username)
		}
	}
	if len(expired) > 0 {
		if err := s.applyConfig(ctx); err != nil {
			return len(expired), err
		}
	}
	return len(expired), nil
}

func (s *SMBService) ListPermissions(ctx context.Context, userID, volumeID int64) ([]models.PermissionView, error) {
	return s.repos.Permissions.List(ctx, userID, volumeID)
}

func (s *SMBService) SetPermission(ctx context.Context, userID, volumeID int64, access string) (PermissionOperationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := PermissionOperationResult{Success: true, UserID: userID, VolumeID: volumeID, Access: access}
	if err := validateAccess(access); err != nil {
		return result, err
	}
	if access == models.AccessNone {
		if err := s.removePermissionOnly(ctx, userID, volumeID); err != nil {
			return result, err
		}
		return s.applyPermissionConfig(ctx, result), nil
	}
	if user, err := s.repos.Users.Get(ctx, userID); err != nil || user == nil {
		return result, errors.New("user not found")
	} else if err := s.executor.AddUserToManagedGroup(user.Username); err != nil {
		return result, err
	}
	if volume, err := s.repos.Volumes.Get(ctx, volumeID); err != nil || volume == nil {
		return result, errors.New("volume not found")
	} else if err := s.executor.EnsureShareTopPermissions(volume.Path); err != nil {
		return result, err
	}
	if err := s.repos.Permissions.Upsert(ctx, userID, volumeID, access); err != nil {
		return result, err
	}
	return s.applyPermissionConfig(ctx, result), nil
}

func (s *SMBService) BulkSetPermissions(ctx context.Context, userIDs, volumeIDs []int64, access string) (PermissionOperationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := PermissionOperationResult{Success: true, Access: access}
	if err := validateAccess(access); err != nil {
		return result, err
	}
	if len(userIDs) == 0 || len(volumeIDs) == 0 {
		return result, errors.New("至少选择一个用户和一个卷")
	}
	for _, userID := range userIDs {
		user, err := s.repos.Users.Get(ctx, userID)
		if err != nil {
			return result, err
		}
		if user == nil {
			return result, errors.New("用户不存在")
		}
		if err := s.executor.AddUserToManagedGroup(user.Username); err != nil {
			return result, err
		}
	}
	for _, volumeID := range volumeIDs {
		volume, err := s.repos.Volumes.Get(ctx, volumeID)
		if err != nil {
			return result, err
		}
		if volume == nil {
			return result, errors.New("卷不存在")
		}
		if err := s.executor.EnsureShareTopPermissions(volume.Path); err != nil {
			return result, err
		}
	}
	for _, userID := range userIDs {
		for _, volumeID := range volumeIDs {
			if access == models.AccessNone {
				if err := s.repos.Permissions.Delete(ctx, userID, volumeID); err != nil {
					return result, err
				}
			} else if err := s.repos.Permissions.Upsert(ctx, userID, volumeID, access); err != nil {
				return result, err
			}
			result.UpdatedCount++
		}
	}
	return s.applyPermissionConfig(ctx, result), nil
}

func (s *SMBService) RemovePermission(ctx context.Context, userID, volumeID int64) error {
	result, err := s.SetPermission(ctx, userID, volumeID, models.AccessNone)
	if err != nil {
		return err
	}
	if !result.ConfigApplied {
		return errors.New(result.ConfigError)
	}
	return nil
}

func (s *SMBService) removePermissionOnly(ctx context.Context, userID, volumeID int64) error {
	if user, err := s.repos.Users.Get(ctx, userID); err != nil || user == nil {
		return errors.New("user not found")
	}
	if volume, err := s.repos.Volumes.Get(ctx, volumeID); err != nil || volume == nil {
		return errors.New("volume not found")
	}
	if err := s.repos.Permissions.Delete(ctx, userID, volumeID); err != nil {
		return err
	}
	return nil
}

func (s *SMBService) applyPermissionConfig(ctx context.Context, result PermissionOperationResult) PermissionOperationResult {
	if err := s.applyConfig(ctx); err != nil {
		result.ConfigApplied = false
		result.SmbConfReloaded = false
		result.ConfigError = err.Error()
		return result
	}
	result.ConfigApplied = true
	result.SmbConfReloaded = true
	return result
}

func (s *SMBService) applyConfig(ctx context.Context) error {
	settings, err := s.repos.Settings.SystemSettings(ctx)
	if err != nil {
		return err
	}
	volumes, err := s.repos.Volumes.Enabled(ctx)
	if err != nil {
		return err
	}
	perms, err := s.repos.Permissions.ForConfig(ctx)
	if err != nil {
		return err
	}
	data := smb.ConfigData{
		GeneratedAt: time.Now().In(time.Local).Format(time.RFC3339),
		GlobalSection: smb.GlobalSection{
			Workgroup:         settings.Workgroup,
			ServerString:      settings.ServerString,
			NetbiosName:       settings.NetbiosName,
			ServerMinProtocol: s.serverMinProtocol,
			ServerMaxProtocol: s.serverMaxProtocol,
		},
	}
	for _, volume := range volumes {
		// Only ensure the share root itself is sane on every reload — a full
		// recursive walk of every share on every config apply would block the
		// API for minutes on large trees and disrupt active SMB sessions.
		// The recursive walk runs once when the volume is created/updated.
		if err := s.executor.EnsureShareTopPermissions(volume.Path); err != nil {
			return err
		}
		share := smb.ShareSection{ShareName: volume.ShareName, Path: volume.Path, Comment: volume.Comment, Browseable: volume.Browseable, GuestOK: volume.GuestOK, ForceGroup: s.executor.ManagedGroup()}
		for _, perm := range perms[volume.ID] {
			if err := s.executor.AddUserToManagedGroup(perm.Username); err != nil {
				return err
			}
			share.ValidUsers = append(share.ValidUsers, perm.Username)
			if perm.Access == models.AccessReadWrite {
				share.WriteList = append(share.WriteList, perm.Username)
			}
			if perm.Access == models.AccessRead {
				share.ReadList = append(share.ReadList, perm.Username)
			}
		}
		if !volume.GuestOK && len(share.ValidUsers) == 0 {
			share.DenyAll = true
		}
		data.Shares = append(data.Shares, share)
	}
	if err := s.generator.Write(data); err != nil {
		return err
	}
	if err := s.executor.ReloadSmbd(); err != nil {
		return err
	}
	return s.repos.Settings.Set(ctx, "last_reload_at", data.GeneratedAt)
}

func randomToken(bytesLen int) string {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func temporaryPassword() string {
	return "Tmp" + randomToken(12) + "9"
}

func (s *SMBService) validateShareRoot(path string) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == "/" {
		return errors.New("共享路径不能是根目录 /。请选择专用共享目录，例如 /srv/samba/data")
	}
	for _, forbidden := range forbiddenSharePaths {
		if cleanPath == forbidden || strings.HasPrefix(cleanPath, forbidden+"/") {
			return fmt.Errorf("共享路径不能位于系统关键目录 %s 下。请使用 /srv/samba、/data、/mnt 或配置允许的专用目录", forbidden)
		}
	}
	if len(s.allowedShareRoots) == 0 {
		return nil
	}
	for _, root := range s.allowedShareRoots {
		root = filepath.Clean(root)
		if root == "/" || root == "." {
			continue
		}
		if cleanPath == root || strings.HasPrefix(cleanPath, root+"/") {
			return nil
		}
	}
	return fmt.Errorf("共享路径不在允许目录内。当前允许根目录：%s。请修改路径或在配置 smb.allowed_share_roots 中添加专用共享根目录", strings.Join(s.allowedShareRoots, ", "))
}

var forbiddenSharePaths = []string{
	"/bin",
	"/boot",
	"/dev",
	"/etc",
	"/lib",
	"/lib32",
	"/lib64",
	"/libx32",
	"/proc",
	"/root",
	"/run",
	"/sbin",
	"/sys",
	"/usr",
	"/var",
}
