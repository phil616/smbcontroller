package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Smb      SmbConfig      `yaml:"smb"`
	Session  SessionConfig  `yaml:"session"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Listen string `yaml:"listen"`
	// Domain is an optional allow-list of public origins, for example:
	// ["https://smb.example.com", "http://smb.example.com"].
	Domain  []string `yaml:"domain"`
	TLSCert string   `yaml:"tls_cert"`
	TLSKey  string   `yaml:"tls_key"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type SmbConfig struct {
	ConfPath          string   `yaml:"conf_path"`
	BackupDir         string   `yaml:"backup_dir"`
	BackupMaxCount    int      `yaml:"backup_max_count"`
	ManagedGroup      string   `yaml:"managed_group"`
	AllowedShareRoots []string `yaml:"allowed_share_roots"`
	ServerMinProtocol string   `yaml:"server_min_protocol"`
	ServerMaxProtocol string   `yaml:"server_max_protocol"`
	ReloadCommand     string   `yaml:"reload_command"`
	RestartCommand    string   `yaml:"restart_command"`
}

type SessionConfig struct {
	TTLHours int `yaml:"ttl_hours"`
}

type LogConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Default() Config {
	return Config{
		Server:   ServerConfig{Listen: "127.0.0.1:8080"},
		Database: DatabaseConfig{Path: "/var/lib/smb-controller/data.db"},
		Smb: SmbConfig{
			ConfPath:          "/etc/samba/smb.conf",
			BackupDir:         "/var/lib/smb-controller/conf-backups",
			BackupMaxCount:    5,
			ManagedGroup:      "smbctrl",
			AllowedShareRoots: []string{"/srv/samba", "/data", "/mnt", "/media"},
			ServerMinProtocol: "SMB2_02",
			ServerMaxProtocol: "SMB3",
			ReloadCommand:     "systemctl reload smbd",
			RestartCommand:    "systemctl restart smbd",
		},
		Session: SessionConfig{TTLHours: 8},
		Log:     LogConfig{Level: "info", File: "/var/log/smb-controller/app.log"},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		// #nosec G304 -- config path is supplied by the local service operator at process start.
		if b, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return cfg, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return cfg, err
		}
	}
	applyEnv(&cfg)
	if err := validateSMBProtocols(cfg.Smb.ServerMinProtocol, cfg.Smb.ServerMaxProtocol); err != nil {
		return cfg, err
	}
	cfg.Smb.ServerMinProtocol = normalizeSMBProtocol(cfg.Smb.ServerMinProtocol)
	cfg.Smb.ServerMaxProtocol = normalizeSMBProtocol(cfg.Smb.ServerMaxProtocol)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	setString("SMB_CTRL_SERVER_LISTEN", &cfg.Server.Listen)
	setStringSlice("SMB_CTRL_SERVER_DOMAIN", &cfg.Server.Domain)
	setString("SMB_CTRL_DATABASE_PATH", &cfg.Database.Path)
	setString("SMB_CTRL_SMB_CONF_PATH", &cfg.Smb.ConfPath)
	setString("SMB_CTRL_SMB_BACKUP_DIR", &cfg.Smb.BackupDir)
	setString("SMB_CTRL_SMB_MANAGED_GROUP", &cfg.Smb.ManagedGroup)
	setStringSlice("SMB_CTRL_SMB_ALLOWED_SHARE_ROOTS", &cfg.Smb.AllowedShareRoots)
	setString("SMB_CTRL_SMB_SERVER_MIN_PROTOCOL", &cfg.Smb.ServerMinProtocol)
	setString("SMB_CTRL_SMB_SERVER_MAX_PROTOCOL", &cfg.Smb.ServerMaxProtocol)
	setString("SMB_CTRL_SMB_RELOAD_COMMAND", &cfg.Smb.ReloadCommand)
	setString("SMB_CTRL_SMB_RESTART_COMMAND", &cfg.Smb.RestartCommand)
	setString("SMB_CTRL_LOG_LEVEL", &cfg.Log.Level)
	setString("SMB_CTRL_LOG_FILE", &cfg.Log.File)
	setInt("SMB_CTRL_SMB_BACKUP_MAX_COUNT", &cfg.Smb.BackupMaxCount)
	setInt("SMB_CTRL_SESSION_TTL_HOURS", &cfg.Session.TTLHours)
}

func setStringSlice(key string, target *[]string) {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		*target = out
	}
}

func setString(key string, target *string) {
	if value := os.Getenv(key); value != "" {
		*target = value
	}
}

func setInt(key string, target *int) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}

func validateSMBProtocols(minProtocol, maxProtocol string) error {
	minProtocol = normalizeSMBProtocol(minProtocol)
	maxProtocol = normalizeSMBProtocol(maxProtocol)
	if minProtocol == "" && maxProtocol == "" {
		return nil
	}
	if minProtocol != "" && !isKnownSMBProtocol(minProtocol) {
		return fmt.Errorf("invalid smb.server_min_protocol %q; allowed values: %s", minProtocol, strings.Join(smbProtocolValues, ", "))
	}
	if maxProtocol != "" && !isKnownSMBProtocol(maxProtocol) {
		return fmt.Errorf("invalid smb.server_max_protocol %q; allowed values: %s", maxProtocol, strings.Join(smbProtocolValues, ", "))
	}
	if minProtocol != "" && maxProtocol != "" && smbProtocolRank[minProtocol] > smbProtocolRank[maxProtocol] {
		return fmt.Errorf("invalid SMB protocol range: server_min_protocol %s is newer than server_max_protocol %s", minProtocol, maxProtocol)
	}
	return nil
}

func normalizeSMBProtocol(protocol string) string {
	return strings.ToUpper(strings.TrimSpace(protocol))
}

func isKnownSMBProtocol(protocol string) bool {
	_, ok := smbProtocolRank[protocol]
	return ok
}

var smbProtocolValues = []string{
	"CORE",
	"COREPLUS",
	"LANMAN1",
	"LANMAN2",
	"NT1",
	"SMB2",
	"SMB2_02",
	"SMB2_10",
	"SMB2_22",
	"SMB2_24",
	"SMB3",
	"SMB3_00",
	"SMB3_02",
	"SMB3_10",
	"SMB3_11",
}

var smbProtocolRank = map[string]int{
	"CORE":     0,
	"COREPLUS": 1,
	"LANMAN1":  2,
	"LANMAN2":  3,
	"NT1":      4,
	"SMB2":     5,
	"SMB2_02":  5,
	"SMB2_10":  6,
	"SMB2_22":  7,
	"SMB2_24":  8,
	"SMB3":     9,
	"SMB3_00":  9,
	"SMB3_02":  10,
	"SMB3_10":  11,
	"SMB3_11":  12,
}
