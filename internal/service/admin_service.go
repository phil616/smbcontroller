package service

import (
	"context"
	"errors"

	"smb-controller/internal/models"
	"smb-controller/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type AdminService struct {
	repos *repository.Repositories
}

func (s *AdminService) IsInitialized(ctx context.Context) (bool, error) {
	return s.repos.Admins.IsInitialized(ctx)
}

func (s *AdminService) Init(ctx context.Context, username, password string) error {
	initialized, err := s.IsInitialized(ctx)
	if err != nil {
		return err
	}
	if initialized {
		return errors.New("system already initialized")
	}
	if err := validateUsername(username); err != nil {
		return err
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	if _, err := s.repos.Admins.Create(ctx, username, string(hash)); err != nil {
		return err
	}
	return s.repos.Settings.Set(ctx, "initialized", "true")
}

func (s *AdminService) Authenticate(ctx context.Context, username, password string) (*models.Admin, error) {
	admin, err := s.repos.Admins.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return admin, nil
}
