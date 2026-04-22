package smb

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"time"
)

type DetectResult struct {
	SmbdInstalled  bool      `json:"smbd_installed"`
	SmbdRunning    bool      `json:"smbd_running"`
	SmbdVersion    string    `json:"smbd_version"`
	ConfPath       string    `json:"conf_path"`
	ConfExists     bool      `json:"conf_exists"`
	ConfParsed     bool      `json:"conf_parsed"`
	ExistingShares []string  `json:"existing_shares"`
	SystemUsers    []string  `json:"system_users"`
	DetectedAt     time.Time `json:"detected_at"`
}

var ConfSearchPaths = []string{"/etc/samba/smb.conf", "/usr/local/etc/samba/smb.conf", "/etc/smb.conf"}

type Detector struct{}

func NewDetector() *Detector { return &Detector{} }

func (d *Detector) Detect(confPath string) DetectResult {
	result := DetectResult{
		SmbdInstalled: execLookPath("smbd"),
		SmbdRunning:   detectSmbdRunning(),
		ConfPath:      firstConfPath(confPath),
		DetectedAt:    time.Now().In(time.Local),
	}
	result.SmbdVersion = smbdVersion()
	if st, err := os.Stat(result.ConfPath); err == nil && !st.IsDir() {
		result.ConfExists = true
		shares, err := parseShares(result.ConfPath)
		if err == nil {
			result.ExistingShares = shares
			result.ConfParsed = true
		}
	}
	result.SystemUsers = pdbUsers()
	return result
}

func execLookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func detectSmbdRunning() bool {
	out, err := exec.Command("systemctl", "is-active", "smbd").Output()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	out, err = exec.Command("pgrep", "smbd").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func smbdVersion() string {
	out, err := exec.Command("smbd", "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "Version "))
}

func firstConfPath(preferred string) string {
	if preferred != "" {
		return preferred
	}
	for _, path := range ConfSearchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ConfSearchPaths[0]
}

func parseShares(path string) ([]string, error) {
	// #nosec G304 -- smb.conf path comes from trusted service configuration and is read-only detection input.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var shares []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			if !strings.EqualFold(name, "global") {
				shares = append(shares, name)
			}
		}
	}
	return shares, scanner.Err()
}

func pdbUsers() []string {
	out, err := exec.Command("pdbedit", "-L").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	users := make([]string, 0, len(lines))
	for _, line := range lines {
		if parts := strings.SplitN(line, ":", 2); len(parts) > 0 && parts[0] != "" {
			users = append(users, parts[0])
		}
	}
	return users
}
