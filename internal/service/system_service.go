package service

import (
	"context"
	"time"

	"smb-controller/internal/models"
	"smb-controller/internal/repository"
	"smb-controller/internal/smb"
)

type SystemService struct {
	repos        *repository.Repositories
	executor     *smb.Executor
	generator    *smb.Generator
	detector     *smb.Detector
	detectResult smb.DetectResult
	smbSvc       *SMBService
}

type Status struct {
	smb.DetectResult
	LastReloadAt       string `json:"last_reload_at"`
	ManagedSharesCount int    `json:"managed_shares_count"`
	ManagedUsersCount  int    `json:"managed_users_count"`
}

func (s *SystemService) Status(ctx context.Context) (Status, error) {
	result := s.detector.Detect(s.detectResult.ConfPath)
	shares, err := s.repos.Volumes.Count(ctx)
	if err != nil {
		return Status{}, err
	}
	users, err := s.repos.Users.Count(ctx)
	if err != nil {
		return Status{}, err
	}
	lastReload, _ := s.repos.Settings.Get(ctx, "last_reload_at")
	return Status{DetectResult: result, LastReloadAt: lastReload, ManagedSharesCount: shares, ManagedUsersCount: users}, nil
}

func (s *SystemService) Reload(ctx context.Context) error {
	return s.smbSvc.applyConfig(ctx)
}

func (s *SystemService) Restart(ctx context.Context) error {
	if err := s.smbSvc.applyConfig(ctx); err != nil {
		return err
	}
	if err := s.executor.RestartSmbd(); err != nil {
		return err
	}
	return s.repos.Settings.Set(ctx, "last_reload_at", time.Now().In(time.Local).Format(time.RFC3339))
}

func (s *SystemService) Conf() (string, error) {
	return s.generator.ReadCurrent()
}

func (s *SystemService) Settings(ctx context.Context) (models.SystemSettings, error) {
	return s.repos.Settings.SystemSettings(ctx)
}

func (s *SystemService) UpdateSettings(ctx context.Context, settings models.SystemSettings) error {
	if settings.Workgroup == "" {
		settings.Workgroup = "WORKGROUP"
	}
	if settings.ServerString == "" {
		settings.ServerString = "SMB Controller"
	}
	if err := validateConfigValue(settings.Workgroup, "smb_workgroup"); err != nil {
		return err
	}
	if err := validateConfigValue(settings.ServerString, "smb_server_string"); err != nil {
		return err
	}
	if err := validateConfigValue(settings.NetbiosName, "smb_netbios_name"); err != nil {
		return err
	}
	if err := s.repos.Settings.UpdateSystemSettings(ctx, settings); err != nil {
		return err
	}
	return s.smbSvc.applyConfig(ctx)
}
