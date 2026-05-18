package smb

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"smb-controller/internal/config"
)

// maxPermissionWalkErrors caps how many per-entry chown/chmod failures we log
// before going silent for the rest of the walk. The walk itself keeps going so
// that one bad file inside a deep share does not leave the rest of the tree
// half-fixed (Windows clients otherwise see "denied" for unrelated paths).
const maxPermissionWalkErrors = 20

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
	gid, err := e.managedGroupGID()
	if err != nil {
		return err
	}
	return ensureShareTreePermissions(path, gid)
}

// EnsureShareTopPermissions only fixes ownership/mode on the share root
// directory itself. It is meant for fast paths (config reload, permission
// changes) where the full recursive walk done by EnsureSharePathPermissions
// would be prohibitively slow on large/deep trees and would also interfere
// with active SMB clients.
func (e *Executor) EnsureShareTopPermissions(path string) error {
	gid, err := e.managedGroupGID()
	if err != nil {
		return err
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
	return applyShareEntryPerms(path, info.Mode(), gid, true)
}

func (e *Executor) managedGroupGID() (int, error) {
	if err := e.EnsureManagedGroup(); err != nil {
		return 0, err
	}
	group, err := user.LookupGroup(e.managedGroup)
	if err != nil {
		return 0, err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, fmt.Errorf("invalid gid for group %s: %w", e.managedGroup, err)
	}
	return gid, nil
}

func ensureShareTreePermissions(path string, gid int) error {
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

	if err := applyShareEntryPerms(path, info.Mode(), gid, true); err != nil {
		return err
	}

	loggedErrors := 0
	return filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Tolerate per-entry walk errors (e.g. a file removed by an active
			// SMB session between readdir and lstat). Without this, one
			// transient ENOENT/EACCES aborted the entire share sync and left
			// the tree half-converted, which is exactly the failure mode
			// Windows users report as "无法使用".
			if entryPath == path {
				return walkErr
			}
			logWalkError(&loggedErrors, entryPath, walkErr)
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entryPath == path {
			// Root already handled above.
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			logWalkError(&loggedErrors, entryPath, err)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := applyShareEntryPerms(entryPath, entryInfo.Mode(), gid, false); err != nil {
			logWalkError(&loggedErrors, entryPath, err)
		}
		return nil
	})
}

// applyShareEntryPerms chowns a single entry to the managed group and ORs in
// the minimum permission bits a member of that group needs to use it. We never
// remove existing bits, so users who deliberately opened their files more
// widely are left alone.
//
// For files we OR in 0660 instead of just 0060 — without owner rw, a file
// created by user X with restrictive mode (e.g. 0040, 0006) becomes
// inaccessible to X over SMB even though group members can use it, because
// POSIX checks owner bits first when the accessing uid matches the owner.
func applyShareEntryPerms(entryPath string, mode os.FileMode, gid int, strict bool) error {
	if err := os.Chown(entryPath, -1, gid); err != nil {
		return fmt.Errorf("failed to set share group on %s: %w", entryPath, err)
	}
	switch {
	case mode.IsDir():
		// #nosec G302 -- Samba share directories intentionally need group traversal/write access and setgid inheritance.
		if err := os.Chmod(entryPath, mode|os.ModeSetgid|0770); err != nil {
			if strict {
				return fmt.Errorf("failed to set share directory permissions on %s: %w", entryPath, err)
			}
			return err
		}
	case mode.IsRegular():
		// #nosec G302 -- read/write SMB users need owner+group read/write on existing files; execute bits are preserved as-is.
		if err := os.Chmod(entryPath, mode|0660); err != nil {
			if strict {
				return fmt.Errorf("failed to set share file permissions on %s: %w", entryPath, err)
			}
			return err
		}
	}
	return nil
}

func logWalkError(counter *int, path string, err error) {
	if *counter >= maxPermissionWalkErrors {
		return
	}
	*counter++
	log.Printf("smb permission walk: skipping %s: %v", path, err)
	if *counter == maxPermissionWalkErrors {
		log.Printf("smb permission walk: suppressing further errors (>%d) for this run", maxPermissionWalkErrors)
	}
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
