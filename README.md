# SMB Controller

Go-based Samba control panel with SQLite, session auth, embedded WebUI, and generated `smb.conf`.

## Security Warning

**This program is a root-privileged system administration tool. Treat it as highly sensitive infrastructure, not as a normal web app.**

SMB Controller manages Samba by editing `smb.conf`, creating Linux users, changing filesystem permissions, running `smbpasswd`, and reloading or restarting `smbd`. In normal operation it needs root privileges. If the WebUI or API is exposed incorrectly, a vulnerability may become a full system compromise.

Do not expose this service directly to the public Internet unless you fully understand the risks and have reviewed your deployment. In particular:

- A bug in input validation, path handling, command execution, authentication, or session handling could lead to privilege escalation or remote code execution.
- A malicious or compromised administrator session can modify Samba shares, create users, change filesystem permissions, and affect host security.
- Reverse proxy, trusted domain, TLS, firewall, and access-control mistakes can expose the root control plane to attackers.
- Samba share paths and permissions are powerful. Misconfiguration can leak or corrupt host files.

Recommended deployment posture:

- Bind the service to `127.0.0.1` or a private management network.
- Put it behind a hardened HTTPS reverse proxy with strong authentication and IP allow-lists.
- Configure `server.domain` with the exact trusted public origins.
- Restrict firewall access to trusted operators only.
- Keep the binary, dependencies, OS, and Samba packages patched.
- Back up `/etc/samba/smb.conf` and application data before first production use.
- Review `smb.allowed_share_roots` and only allow dedicated share directories such as `/srv/samba`.
- Do not run this on an Internet-facing host unless you are prepared to operate and monitor it as a root-level admin surface.

If you are not sure what these warnings mean, do not deploy SMB Controller on the public Internet.

## Build

```bash
go build -o smb-controller ./main.go
```

## Run

The service is intended to run as root on Linux with Samba installed:

```bash
sudo ./smb-controller --config ./config.yaml.example
```

The process sets `time.Local` to `Asia/Shanghai`; API timestamps and generated config timestamps use that timezone.

## Environment Overrides

Supported examples:

```bash
SMB_CTRL_SERVER_LISTEN="127.0.0.1:9090"
SMB_CTRL_DATABASE_PATH="/var/lib/smb-controller/data.db"
SMB_CTRL_SMB_CONF_PATH="/etc/samba/smb.conf"
```
