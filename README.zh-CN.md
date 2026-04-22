# SMB Controller

SMB Controller 是一个使用 Go 编写的 Samba 控制面板，内置 SQLite、Session 登录认证、嵌入式 WebUI，并可自动生成 `smb.conf`。

## 安全警告

**这是一个依赖 root 权限运行的系统管理程序。请把它当作高敏感基础设施，而不是普通 Web 应用。**

SMB Controller 会管理 Samba，并执行以下高权限操作：

- 编辑和替换 `smb.conf`
- 创建或删除 Linux 系统用户
- 修改共享目录的文件系统权限
- 调用 `smbpasswd`
- reload 或 restart `smbd`

如果 WebUI 或 API 被错误暴露，一个漏洞可能会演变成完整的系统入侵。

请不要在没有充分理解风险和审查部署方式的情况下，把它直接暴露到公网。尤其需要注意：

- 输入校验、路径处理、命令执行、认证或 Session 处理中的漏洞，可能导致权限提升或远程代码执行。
- 被盗用的管理员会话可以修改 Samba 共享、创建用户、修改文件系统权限，并影响主机安全。
- 反向代理、可信域名、TLS、防火墙或访问控制配置错误，可能把 root 级控制面暴露给攻击者。
- Samba 共享路径和权限非常敏感，错误配置可能泄露或破坏主机文件。

推荐部署方式：

- 默认监听 `127.0.0.1` 或私有管理网络。
- 放在经过加固的 HTTPS 反向代理后面。
- 使用强认证和 IP 白名单。
- 配置 `server.domain`，只允许可信访问域名。
- 只允许可信运维人员访问管理端口。
- 持续更新本程序、操作系统和 Samba 软件包。
- 首次生产使用前备份 `/etc/samba/smb.conf` 和应用数据。
- 审查 `smb.allowed_share_roots`，只允许专用共享目录，例如 `/srv/samba`。

如果你不清楚这些安全警告意味着什么，请不要把 SMB Controller 部署到公网。

## 构建

```bash
go build -o smb-controller ./main.go
```

## 运行

该服务设计为在已安装 Samba 的 Linux 系统上以 root 权限运行：

```bash
sudo ./smb-controller --config ./config.yaml.example
```

程序会将 `time.Local` 设置为 `Asia/Shanghai`，API 时间戳和生成配置中的时间也会使用该时区。

## 配置示例

```yaml
server:
  listen: "127.0.0.1:8080"
  domain:
    - "https://smb.example.com"

database:
  path: "/var/lib/smb-controller/data.db"

smb:
  conf_path: "/etc/samba/smb.conf"
  backup_dir: "/var/lib/smb-controller/conf-backups"
  backup_max_count: 5
  managed_group: "smbctrl"
  allowed_share_roots:
    - "/srv/samba"
    - "/data"
    - "/mnt"
    - "/media"
  reload_command: "systemctl reload smbd"
  restart_command: "systemctl restart smbd"

session:
  ttl_hours: 8
```

## 环境变量覆盖

示例：

```bash
SMB_CTRL_SERVER_LISTEN="127.0.0.1:9090"
SMB_CTRL_DATABASE_PATH="/var/lib/smb-controller/data.db"
SMB_CTRL_SMB_CONF_PATH="/etc/samba/smb.conf"
SMB_CTRL_SERVER_DOMAIN="https://smb.example.com,http://smb.example.com"
```

## 许可证

本项目使用 MIT License，详见 [LICENSE](./LICENSE)。

