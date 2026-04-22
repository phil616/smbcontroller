package smb

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"smb-controller/internal/config"
)

type Executor struct {
	reloadCommand  string
	restartCommand string
	managedGroup   string
}

func NewExecutor(cfg config.SmbConfig) *Executor {
	group := cfg.ManagedGroup
	if group == "" {
		group = "smbctrl"
	}
	return &Executor{reloadCommand: cfg.ReloadCommand, restartCommand: cfg.RestartCommand, managedGroup: group}
}

func (e *Executor) ManagedGroup() string {
	return e.managedGroup
}

func (e *Executor) CreateSystemUser(username string) (bool, error) {
	if e.SystemUserExists(username) {
		return false, e.AddUserToManagedGroup(username)
	}
	if err := run("useradd", "--no-create-home", "--shell", "/usr/sbin/nologin", username); err != nil {
		return false, err
	}
	return true, e.AddUserToManagedGroup(username)
}

func (e *Executor) DeleteSystemUser(username string) error {
	if !e.SystemUserExists(username) {
		return nil
	}
	return run("userdel", username)
}

func (e *Executor) SystemUserExists(username string) bool {
	_, err := user.Lookup(username)
	return err == nil
}

func (e *Executor) SetSmbPassword(username, password string) error {
	// #nosec G204 -- username is validated before reaching this layer and exec.Command does not invoke a shell.
	cmd := exec.Command("smbpasswd", "-a", "-s", username)
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	return runCmd(cmd)
}

func (e *Executor) EnsureManagedGroup() error {
	if _, err := user.LookupGroup(e.managedGroup); err == nil {
		return nil
	}
	return run("groupadd", "--system", e.managedGroup)
}

func (e *Executor) AddUserToManagedGroup(username string) error {
	if err := e.EnsureManagedGroup(); err != nil {
		return err
	}
	return run("usermod", "-aG", e.managedGroup, username)
}

func (e *Executor) EnsureSharePathPermissions(path string) error {
	if err := e.EnsureManagedGroup(); err != nil {
		return err
	}
	group, err := user.LookupGroup(e.managedGroup)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return fmt.Errorf("invalid gid for group %s: %w", e.managedGroup, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("share path must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("share path must be a directory: %s", path)
	}
	if err := os.Chown(path, -1, gid); err != nil {
		return fmt.Errorf("failed to set share group on %s: %w", path, err)
	}
	// #nosec G302 -- Samba share roots intentionally need group traversal and group write access.
	if err := os.Chmod(path, 02775); err != nil {
		return fmt.Errorf("failed to set share permissions on %s: %w", path, err)
	}
	return nil
}

func (e *Executor) DeleteSmbUser(username string) error {
	return run("smbpasswd", "-x", username)
}

func (e *Executor) SetSmbUserEnabled(username string, enabled bool) error {
	flag := "-d"
	if enabled {
		flag = "-e"
	}
	return run("smbpasswd", flag, username)
}

func (e *Executor) ReloadSmbd() error {
	if e.reloadCommand != "" {
		if err := runCommandLine(e.reloadCommand); err == nil {
			return nil
		}
	}
	if err := run("systemctl", "reload", "smbd"); err == nil {
		return nil
	}
	return run("pkill", "-HUP", "smbd")
}

func (e *Executor) RestartSmbd() error {
	if e.restartCommand != "" {
		return runCommandLine(e.restartCommand)
	}
	return run("systemctl", "restart", "smbd")
}

func (e *Executor) GetSmbdStatus() (string, error) {
	out, err := exec.Command("systemctl", "status", "smbd", "--no-pager").CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("systemctl status smbd failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func run(name string, args ...string) error {
	// #nosec G204 -- commands are called without a shell; user-controlled values are passed only as arguments after service validation.
	return runCmd(exec.Command(name, args...))
}

func runCmd(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s failed: %s", cmd.Path, msg)
	}
	return nil
}

func runCommandLine(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return errors.New("empty command")
	}
	return run(parts[0], parts[1:]...)
}
