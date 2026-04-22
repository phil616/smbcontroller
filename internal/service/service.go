package service

import (
	"smb-controller/internal/repository"
	"smb-controller/internal/smb"
)

type Services struct {
	Admin  *AdminService
	SMB    *SMBService
	System *SystemService
}

func NewServices(repos *repository.Repositories, executor *smb.Executor, generator *smb.Generator, detector *smb.Detector, detectResult smb.DetectResult, allowedShareRoots []string) *Services {
	smbSvc := &SMBService{repos: repos, executor: executor, generator: generator, allowedShareRoots: allowedShareRoots}
	return &Services{
		Admin:  &AdminService{repos: repos},
		SMB:    smbSvc,
		System: &SystemService{repos: repos, executor: executor, generator: generator, detector: detector, detectResult: detectResult, smbSvc: smbSvc},
	}
}
