# SMB 控制面板 (SMB Controller) 技术设计文档

**版本**: v1.0  
**语言**: Go 1.22+  
**目标平台**: Linux (Ubuntu 20.04+, Debian 11+, CentOS 8+)  
**运行权限**: root

---

## 目录

1. [项目概述](#1-项目概述)
2. [系统架构](#2-系统架构)
3. [目录结构](#3-目录结构)
4. [数据库设计](#4-数据库设计)
5. [核心模块设计](#5-核心模块设计)
6. [API 接口设计](#6-api-接口设计)
7. [WebUI 设计](#7-webui-设计)
8. [SMB 配置管理](#8-smb-配置管理)
9. [初始化流程](#9-初始化流程)
10. [安全设计](#10-安全设计)
11. [配置文件](#11-配置文件)
12. [部署与运行](#12-部署与运行)
13. [编码规范与依赖](#13-编码规范与依赖)

---

## 1. 项目概述

### 1.1 背景

Samba (`smbd`) 是 Linux 上实现 SMB/CIFS 协议的标准服务，但其配置完全依赖手动编辑 `/etc/samba/smb.conf` 文本文件，缺乏可视化管理界面，对运维人员不友好。

### 1.2 目标

构建一个以 **root 权限**持续运行的 Go 后端服务，提供：

- 卷（共享目录）的增删改查
- SMB 用户的增删改查及权限分配
- 自动生成并同步 `smb.conf`
- WebUI 可视化管理界面
- 初始化超级管理员机制
- 自动检测系统现有 smbd 服务状态与配置

### 1.3 核心约束

| 约束项 | 要求 |
|--------|------|
| 编程语言 | Go 1.22+ |
| 数据库 | SQLite（`modernc.org/sqlite`，纯 Go，无 CGO） |
| SMB 服务 | 系统已安装 `samba` 包（smbd/nmbd） |
| 运行权限 | root |
| 前端 | 内嵌于 Go 二进制（`embed.FS`），纯 HTML/CSS/JS |
| 认证方式 | Session Cookie（服务端 session） |

---

## 2. 系统架构

```
┌─────────────────────────────────────────────────────────┐
│                    SMB Controller                        │
│                                                         │
│  ┌───────────┐    ┌─────────────┐    ┌───────────────┐  │
│  │  WebUI    │    │  HTTP API   │    │  SMB Engine   │  │
│  │ (embed)   │◄──►│  (chi router│◄──►│  (smb.conf    │  │
│  │ HTML/JS   │    │  + middleware│   │   generator)  │  │
│  └───────────┘    └──────┬──────┘    └───────┬───────┘  │
│                          │                   │          │
│                   ┌──────▼──────┐    ┌───────▼───────┐  │
│                   │  Service    │    │  OS Executor  │  │
│                   │  Layer      │    │  (exec.Cmd)   │  │
│                   └──────┬──────┘    └───────────────┘  │
│                          │                              │
│                   ┌──────▼──────┐                       │
│                   │  SQLite DB  │                       │
│                   │  (data.db)  │                       │
│                   └─────────────┘                       │
└─────────────────────────────────────────────────────────┘
         ▲
         │ 写入 /etc/samba/smb.conf
         │ 执行 smbpasswd / systemctl
         ▼
┌─────────────────────┐
│  Linux OS           │
│  smbd / nmbd        │
│  /etc/samba/        │
└─────────────────────┘
```

### 2.1 请求流程

```
Browser → HTTP Request
  → Middleware (Auth Check)
    → Router (chi)
      → Handler
        → Service Layer
          → Repository (SQLite)
          → SMB Engine (smb.conf + smbpasswd)
            → OS Exec (systemctl reload smbd)
```

---

## 3. 目录结构

```
smb-controller/
├── main.go                        # 程序入口，初始化所有模块
├── go.mod
├── go.sum
│
├── internal/
│   ├── config/
│   │   └── config.go              # 程序配置（端口、db路径等）
│   │
│   ├── database/
│   │   ├── database.go            # SQLite 连接、migrate
│   │   └── migrations.go          # 建表 SQL
│   │
│   ├── models/
│   │   ├── admin.go               # Admin 模型
│   │   ├── user.go                # SMB 用户模型
│   │   ├── volume.go              # 卷（共享）模型
│   │   └── permission.go          # 用户-卷权限模型
│   │
│   ├── repository/
│   │   ├── admin_repo.go
│   │   ├── user_repo.go
│   │   ├── volume_repo.go
│   │   └── permission_repo.go
│   │
│   ├── service/
│   │   ├── admin_service.go       # 管理员认证、初始化
│   │   ├── user_service.go        # SMB用户管理（含smbpasswd调用）
│   │   ├── volume_service.go      # 卷管理
│   │   ├── permission_service.go  # 权限管理
│   │   └── smb_service.go         # smb.conf生成、smbd控制
│   │
│   ├── handler/
│   │   ├── middleware.go          # Auth中间件、日志中间件
│   │   ├── admin_handler.go       # 登录、初始化
│   │   ├── user_handler.go
│   │   ├── volume_handler.go
│   │   ├── permission_handler.go
│   │   └── system_handler.go      # 系统状态、smbd检测
│   │
│   ├── session/
│   │   └── session.go             # 内存session管理
│   │
│   └── smb/
│       ├── detector.go            # 检测smbd、smb.conf
│       ├── generator.go           # 生成smb.conf
│       └── executor.go            # 执行系统命令
│
└── web/                           # 前端静态资源（embed到二进制）
    ├── index.html
    ├── setup.html                 # 首次初始化页面
    ├── login.html
    ├── static/
    │   ├── css/
    │   │   └── style.css
    │   └── js/
    │       ├── api.js             # 统一API调用封装
    │       ├── volumes.js
    │       ├── users.js
    │       └── permissions.js
    └── templates/                 # 可选：Go html/template
```

---

## 4. 数据库设计

数据库文件默认路径：`/var/lib/smb-controller/data.db`

### 4.1 表结构

#### `admins` — 管理员账户

```sql
CREATE TABLE IF NOT EXISTS admins (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    username    TEXT NOT NULL UNIQUE,
    password    TEXT NOT NULL,        -- bcrypt hash
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### `smb_users` — SMB 协议用户

```sql
CREATE TABLE IF NOT EXISTS smb_users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    username    TEXT NOT NULL UNIQUE,  -- Linux系统用户名
    display_name TEXT,
    enabled     INTEGER DEFAULT 1,    -- 0=禁用, 1=启用
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

> **注意**：SMB 用户对应 Linux 系统用户，密码通过 `smbpasswd` 管理，不存储在 DB 中。

#### `volumes` — SMB 共享卷

```sql
CREATE TABLE IF NOT EXISTS volumes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    share_name   TEXT NOT NULL UNIQUE,   -- SMB共享名，如 DataFolder
    path         TEXT NOT NULL UNIQUE,   -- 本地路径，如 /data
    comment      TEXT DEFAULT '',        -- 描述
    browseable   INTEGER DEFAULT 1,      -- 是否可浏览
    read_only    INTEGER DEFAULT 0,      -- 默认不只读（由权限控制）
    guest_ok     INTEGER DEFAULT 0,      -- 是否允许匿名
    enabled      INTEGER DEFAULT 1,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### `permissions` — 用户-卷权限映射

```sql
CREATE TABLE IF NOT EXISTS permissions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES smb_users(id) ON DELETE CASCADE,
    volume_id  INTEGER NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    access     TEXT NOT NULL DEFAULT 'read',  -- 'read' | 'readwrite' | 'none'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, volume_id)
);
```

#### `system_settings` — 系统配置 KV 表

```sql
CREATE TABLE IF NOT EXISTS system_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 初始化标志
-- INSERT OR IGNORE INTO system_settings(key, value) VALUES('initialized', 'false');
-- INSERT OR IGNORE INTO system_settings(key, value) VALUES('smb_workgroup', 'WORKGROUP');
-- INSERT OR IGNORE INTO system_settings(key, value) VALUES('smb_server_string', 'SMB Controller');
-- INSERT OR IGNORE INTO system_settings(key, value) VALUES('smb_netbios_name', '');
```

### 4.2 初始化判断逻辑

```go
// 检查是否已完成初始化
func (r *AdminRepo) IsInitialized(ctx context.Context) (bool, error) {
    var value string
    err := r.db.QueryRowContext(ctx,
        "SELECT value FROM system_settings WHERE key = 'initialized'",
    ).Scan(&value)
    if err != nil {
        return false, err
    }
    return value == "true", nil
}
```

---

## 5. 核心模块设计

### 5.1 `internal/smb/detector.go` — SMB 环境检测

启动时自动检测，结果缓存并可通过 API 查询。

```go
type SmbDetectResult struct {
    SmbdInstalled    bool   `json:"smbd_installed"`
    SmbdRunning      bool   `json:"smbd_running"`
    SmbdVersion      string `json:"smbd_version"`
    ConfPath         string `json:"conf_path"`       // 如 /etc/samba/smb.conf
    ConfExists       bool   `json:"conf_exists"`
    ConfParsed       bool   `json:"conf_parsed"`
    ExistingShares   []string `json:"existing_shares"` // 已存在的共享名
    SystemUsers      []string `json:"system_users"`    // 已存在的smb用户
    DetectedAt       time.Time `json:"detected_at"`
}

// 检测 smbd 是否已安装
func detectSmbdInstalled() bool {
    _, err := exec.LookPath("smbd")
    return err == nil
}

// 检测 smbd 是否在运行（通过 systemctl 或 /var/run）
func detectSmbdRunning() bool {
    out, err := exec.Command("systemctl", "is-active", "smbd").Output()
    if err != nil {
        // fallback: 检查进程
        out2, _ := exec.Command("pgrep", "smbd").Output()
        return len(strings.TrimSpace(string(out2))) > 0
    }
    return strings.TrimSpace(string(out)) == "active"
}

// 解析已有 smb.conf，提取已存在的共享配置（导入参考用）
func parseExistingConf(path string) ([]ExistingShare, error)
```

### 5.2 `internal/smb/generator.go` — smb.conf 生成器

每次卷/权限发生变化时，调用此模块重新生成配置文件并 reload smbd。

```go
// 配置文件结构
type SmbConfig struct {
    Global   GlobalSection
    Shares   []ShareSection
}

type GlobalSection struct {
    Workgroup    string
    ServerString string
    NetbiosName  string
    // 安全固定项（不对外暴露修改）:
    // security = user
    // map to guest = bad user
    // log level = 1
    // max log size = 1000
}

type ShareSection struct {
    ShareName   string
    Path        string
    Comment     string
    Browseable  bool
    GuestOk     bool
    // 由 permissions 表动态生成：
    ValidUsers  []string  // 有访问权限的用户列表
    WriteList   []string  // 有写权限的用户
    ReadList    []string  // 只有读权限的用户
}
```

**生成示例输出：**

```ini
[global]
    workgroup = WORKGROUP
    server string = SMB Controller
    netbios name = MYSERVER
    security = user
    map to guest = bad user
    log level = 1
    max log size = 1000

[DataFolder]
    path = /data
    comment = 
    browseable = yes
    guest ok = no
    valid users = user1 user2
    write list = user1
    read list = user2
```

**原子写入策略：**

```go
func WriteConfig(path string, config *SmbConfig) error {
    // 1. 先写入临时文件 smb.conf.tmp
    // 2. 用 testparm 验证语法
    // 3. 原子替换 smb.conf
    // 4. reload smbd
    tmpPath := path + ".tmp"
    // ... 写入 ...
    if err := validateWithTestparm(tmpPath); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("smb.conf validation failed: %w", err)
    }
    return os.Rename(tmpPath, path)
}
```

### 5.3 `internal/smb/executor.go` — 系统命令执行

```go
// 创建 Linux 系统用户（SMB用户必须是系统用户）
// useradd --no-create-home --shell /sbin/nologin {username}
func CreateSystemUser(username string) error

// 删除 Linux 系统用户
// userdel {username}
func DeleteSystemUser(username string) error

// 检查系统用户是否存在
func SystemUserExists(username string) bool

// 设置/更新 SMB 密码
// echo -e "{password}\n{password}" | smbpasswd -a -s {username}
func SetSmbPassword(username, password string) error

// 删除 SMB 密码（从 samba passdb 移除）
// smbpasswd -x {username}
func DeleteSmbUser(username string) error

// 启用/禁用 SMB 用户
// smbpasswd -e {username}  (enable)
// smbpasswd -d {username}  (disable)
func SetSmbUserEnabled(username string, enabled bool) error

// 重载 smbd 配置（不中断连接）
// systemctl reload smbd  (或 kill -HUP $(cat /var/run/samba/smbd.pid))
func ReloadSmbd() error

// 重启 smbd
func RestartSmbd() error

// 获取 smbd 状态
// systemctl status smbd
func GetSmbdStatus() (string, error)
```

### 5.4 `internal/session/session.go` — Session 管理

使用内存 Map 实现简单 Session，生产可替换为 Redis。

```go
type Session struct {
    ID        string
    AdminID   int64
    Username  string
    CreatedAt time.Time
    ExpiresAt time.Time
}

type SessionStore struct {
    mu       sync.RWMutex
    sessions map[string]*Session
    ttl      time.Duration  // 默认 8 小时
}

// 核心方法
func (s *SessionStore) Create(adminID int64, username string) *Session
func (s *SessionStore) Get(sessionID string) (*Session, bool)
func (s *SessionStore) Delete(sessionID string)
func (s *SessionStore) Cleanup()  // 定时清理过期session（goroutine）
```

### 5.5 `internal/service/smb_service.go` — SMB 服务编排

统一协调数据库操作和 OS 操作，是核心业务逻辑所在。

```go
// 创建新卷（共享）
// 步骤：1. 检查目录是否存在/创建 2. 写DB 3. 重新生成smb.conf 4. reload
func (s *SmbService) CreateVolume(ctx context.Context, req CreateVolumeRequest) (*models.Volume, error)

// 删除卷（从smb.conf移除，不删除实际目录）
func (s *SmbService) DeleteVolume(ctx context.Context, volumeID int64) error

// 创建SMB用户
// 步骤：1. 创建Linux系统用户 2. 设置smbpasswd 3. 写DB 4. 重新生成smb.conf
func (s *SmbService) CreateUser(ctx context.Context, req CreateUserRequest) (*models.SmbUser, error)

// 更改用户密码
func (s *SmbService) ChangeUserPassword(ctx context.Context, userID int64, newPassword string) error

// 设置权限（核心功能）
// 步骤：1. upsert permissions表 2. 重新生成smb.conf 3. reload
func (s *SmbService) SetPermission(ctx context.Context, userID, volumeID int64, access string) error

// 移除用户对某卷的权限
func (s *SmbService) RemovePermission(ctx context.Context, userID, volumeID int64) error

// 重新生成并应用 smb.conf（内部方法，每次变更后调用）
func (s *SmbService) applyConfig(ctx context.Context) error
```

---

## 6. API 接口设计

所有 API 路径以 `/api/v1` 为前缀，返回 JSON。

### 6.1 认证相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | `/api/v1/setup/status` | 查询是否已初始化 | 无 |
| POST | `/api/v1/setup/init` | 首次初始化（设置超管） | 无 |
| POST | `/api/v1/auth/login` | 登录 | 无 |
| POST | `/api/v1/auth/logout` | 登出 | 需要 |
| GET | `/api/v1/auth/me` | 获取当前登录信息 | 需要 |

#### POST `/api/v1/setup/init`

```json
// Request
{
    "username": "admin",
    "password": "YourStr0ngPassword!"
}

// Response 200
{
    "success": true,
    "message": "Initialized successfully"
}

// Response 400（已初始化）
{
    "success": false,
    "error": "System already initialized"
}
```

#### POST `/api/v1/auth/login`

```json
// Request
{
    "username": "admin",
    "password": "YourStr0ngPassword!"
}

// Response 200 — 设置 Set-Cookie: session_id=xxx; HttpOnly; Path=/
{
    "success": true,
    "username": "admin"
}

// Response 401
{
    "success": false,
    "error": "Invalid credentials"
}
```

### 6.2 卷管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/volumes` | 列出所有卷 |
| POST | `/api/v1/volumes` | 创建卷 |
| GET | `/api/v1/volumes/:id` | 获取卷详情（含权限列表） |
| PUT | `/api/v1/volumes/:id` | 更新卷配置 |
| DELETE | `/api/v1/volumes/:id` | 删除卷 |

#### POST `/api/v1/volumes`

```json
// Request
{
    "share_name": "DataFolder",   // SMB共享名
    "path": "/data",               // 本地绝对路径
    "comment": "数据共享目录",
    "browseable": true,
    "guest_ok": false,
    "create_dir_if_not_exists": true  // 是否自动创建目录
}

// Response 201
{
    "id": 1,
    "share_name": "DataFolder",
    "path": "/data",
    "comment": "数据共享目录",
    "browseable": true,
    "guest_ok": false,
    "enabled": true,
    "created_at": "2024-01-01T00:00:00Z"
}
```

#### GET `/api/v1/volumes/:id`

```json
// Response 200
{
    "id": 1,
    "share_name": "DataFolder",
    "path": "/data",
    "comment": "",
    "enabled": true,
    "permissions": [
        {
            "user_id": 1,
            "username": "user1",
            "access": "readwrite"
        },
        {
            "user_id": 2,
            "username": "user2",
            "access": "read"
        }
    ]
}
```

### 6.3 用户管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/users` | 列出所有 SMB 用户 |
| POST | `/api/v1/users` | 创建 SMB 用户 |
| GET | `/api/v1/users/:id` | 获取用户详情（含权限列表） |
| PUT | `/api/v1/users/:id` | 更新用户信息 |
| POST | `/api/v1/users/:id/password` | 更改密码 |
| PUT | `/api/v1/users/:id/enabled` | 启用/禁用用户 |
| DELETE | `/api/v1/users/:id` | 删除用户 |

#### POST `/api/v1/users`

```json
// Request
{
    "username": "user1",          // Linux用户名（字母数字下划线）
    "display_name": "用户一",
    "password": "UserPassword123!"
}

// Response 201
{
    "id": 1,
    "username": "user1",
    "display_name": "用户一",
    "enabled": true,
    "created_at": "2024-01-01T00:00:00Z"
}
```

#### POST `/api/v1/users/:id/password`

```json
// Request
{
    "new_password": "NewPassword456!"
}

// Response 200
{
    "success": true
}
```

### 6.4 权限管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/permissions` | 列出所有权限（支持 ?user_id=&volume_id= 过滤） |
| PUT | `/api/v1/permissions` | 设置或更新权限（upsert） |
| DELETE | `/api/v1/permissions` | 移除权限 |

#### PUT `/api/v1/permissions`

```json
// Request — 给 user1 设置对 DataFolder(/data) 的读写权限
{
    "user_id": 1,
    "volume_id": 1,
    "access": "readwrite"   // "read" | "readwrite" | "none"（none=移除）
}

// Response 200
{
    "success": true,
    "user_id": 1,
    "volume_id": 1,
    "access": "readwrite",
    "smb_conf_reloaded": true
}
```

### 6.5 系统管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/system/status` | 获取 smbd 运行状态和检测结果 |
| POST | `/api/v1/system/reload` | 手动 reload smbd |
| POST | `/api/v1/system/restart` | 重启 smbd |
| GET | `/api/v1/system/conf` | 获取当前生成的 smb.conf 内容 |
| GET | `/api/v1/system/settings` | 获取全局设置 |
| PUT | `/api/v1/system/settings` | 更新全局设置（workgroup等） |

#### GET `/api/v1/system/status`

```json
{
    "smbd_installed": true,
    "smbd_running": true,
    "smbd_version": "4.17.7",
    "conf_path": "/etc/samba/smb.conf",
    "conf_exists": true,
    "last_reload_at": "2024-01-01T12:00:00Z",
    "managed_shares_count": 3,
    "managed_users_count": 5
}
```

### 6.6 统一错误响应格式

```json
{
    "success": false,
    "error": "错误描述",
    "code": "ERROR_CODE",        // 可选，机器可读错误码
    "details": {}                // 可选，附加信息
}
```

---

## 7. WebUI 设计

所有前端文件通过 Go 的 `embed.FS` 打包进二进制，无需独立部署。

### 7.1 页面路由

| URL | 说明 |
|-----|------|
| `GET /` | 主应用（SPA 入口），若未初始化跳转 `/setup`，未登录跳转 `/login` |
| `GET /setup` | 初始化设置页 |
| `GET /login` | 登录页 |

后端路由：

```go
r.Get("/", serveIndex)
r.Get("/setup", serveSetup)
r.Get("/login", serveLogin)
r.PathPrefix("/static/").Handler(staticFS) // 静态资源
r.PathPrefix("/api/").Handler(apiRouter)   // API路由
```

### 7.2 主界面布局

```
┌─────────────────────────────────────────────┐
│  SMB Controller   [版本]        [admin ▾] [退出]│
├──────────┬──────────────────────────────────┤
│          │                                  │
│ 📁 卷管理 │   主内容区域                      │
│          │   （根据左侧导航动态切换）            │
│ 👤 用户管理│                                  │
│          │                                  │
│ 🔒 权限管理│                                  │
│          │                                  │
│ ⚙️ 系统状态│                                  │
│          │                                  │
└──────────┴──────────────────────────────────┘
```

### 7.3 卷管理页面功能

- 卷列表（表格：共享名 / 路径 / 状态 / 用户数 / 操作）
- 新建卷（弹窗表单：共享名、路径、描述、是否可浏览、匿名访问）
- 卷详情（侧边抽屉：基本信息 + 该卷的用户权限列表）
- 删除确认弹窗

### 7.4 用户管理页面功能

- 用户列表（表格：用户名 / 显示名 / 状态 / 权限卷数 / 操作）
- 新建用户（弹窗：用户名、显示名、密码）
- 修改密码（独立弹窗）
- 启用/禁用切换开关
- 查看用户权限（侧边面板，显示用户对哪些卷有何权限）
- 删除确认弹窗

### 7.5 权限管理页面功能

核心功能，以矩阵视图展示：

```
         DataFolder   Photos   Backup
user1    读写  ✓      只读  ✓   -
user2    只读  ✓      -         读写  ✓
user3    -              -         -
```

点击矩阵单元格弹出权限选择器：`无权限 / 只读 / 读写`，即时保存并 reload smbd。

同时提供按用户或按卷的列表视图。

### 7.6 系统状态页面功能

- smbd 服务状态指示器（绿色/红色）
- smbd 版本信息
- 当前 smb.conf 内容预览（代码框，只读）
- 手动 Reload / Restart 按钮
- 全局设置表单（Workgroup、Server String、NetBIOS Name）

### 7.7 初始化页面（`/setup`）

```
┌─────────────────────────────────────┐
│                                     │
│         欢迎使用 SMB Controller      │
│                                     │
│    请设置管理员账户以开始使用            │
│                                     │
│    用户名: [___________________]    │
│                                     │
│    密  码: [___________________]    │
│                                     │
│    确认密码: [_________________]    │
│                                     │
│         [ 初始化并开始使用 ]          │
│                                     │
│    ℹ️ 初始化完成后此页面将不可访问      │
│                                     │
└─────────────────────────────────────┘
```

---

## 8. SMB 配置管理

### 8.1 smb.conf 完整生成模板

```go
const smbConfTemplate = `# Generated by SMB Controller - DO NOT EDIT MANUALLY
# Last updated: {{.GeneratedAt}}

[global]
    workgroup = {{.Workgroup}}
    server string = {{.ServerString}}
    {{- if .NetbiosName}}
    netbios name = {{.NetbiosName}}
    {{- end}}
    security = user
    map to guest = bad user
    log level = 1
    max log size = 1000
    passdb backend = tdbsam
    
{{- range .Shares}}

[{{.ShareName}}]
    path = {{.Path}}
    comment = {{.Comment}}
    browseable = {{if .Browseable}}yes{{else}}no{{end}}
    guest ok = {{if .GuestOk}}yes{{else}}no{{end}}
    {{- if .ValidUsers}}
    valid users = {{join .ValidUsers " "}}
    {{- end}}
    {{- if .WriteList}}
    write list = {{join .WriteList " "}}
    {{- end}}
    {{- if .ReadList}}
    read list = {{join .ReadList " "}}
    {{- end}}
    create mask = 0664
    directory mask = 0775
{{- end}}
`
```

### 8.2 权限到 smb.conf 的映射

| DB `access` 值 | smb.conf 效果 |
|----------------|----------------|
| `readwrite` | 加入 `valid users`，加入 `write list` |
| `read` | 加入 `valid users`，加入 `read list` |
| `none` | 不出现在任何列表中（无权访问） |

### 8.3 配置文件路径检测

按优先级检测：

```go
var confSearchPaths = []string{
    "/etc/samba/smb.conf",
    "/usr/local/etc/samba/smb.conf",
    "/etc/smb.conf",
}
```

程序管理的配置写入 `/etc/samba/smb.conf`（默认），路径可通过程序配置文件覆盖。

### 8.4 备份机制

每次写入前自动备份：

```
/etc/samba/smb.conf.bak.{timestamp}
```

保留最近 5 个备份，旧备份自动删除。

---

## 9. 初始化流程

### 9.1 完整启动流程

```
程序启动 (main.go)
  │
  ├─ 1. 加载配置文件 (config.yaml 或 环境变量)
  │
  ├─ 2. 初始化 SQLite 数据库（自动建表/迁移）
  │
  ├─ 3. 检测 smbd 环境（detector.go）
  │     ├─ smbd 是否安装
  │     ├─ smbd 是否运行
  │     └─ 解析现有 smb.conf（记录检测结果，不自动修改）
  │
  ├─ 4. 查询 initialized 标志
  │     ├─ false → 所有 API 请求重定向到 /setup
  │     └─ true  → 正常服务
  │
  └─ 5. 启动 HTTP 服务 (默认 :8080)
```

### 9.2 首次初始化流程

```
用户访问任意页面
  → 后端检测 initialized=false
  → 重定向到 /setup
  
用户在 /setup 填写管理员账密并提交
  → POST /api/v1/setup/init
  → 后端验证（用户名格式、密码强度）
  → bcrypt 加密密码，写入 admins 表
  → 更新 system_settings: initialized=true
  → 返回成功
  
前端跳转到 /login
  → 用户登录后进入主界面
```

### 9.3 密码强度验证规则

```go
func validatePassword(password string) error {
    if len(password) < 8 {
        return errors.New("密码长度不能少于8位")
    }
    // 要求包含：大写字母、小写字母、数字
    // （不强制特殊字符，避免过于严格）
    hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
    hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
    hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
    if !hasUpper || !hasLower || !hasDigit {
        return errors.New("密码必须包含大写字母、小写字母和数字")
    }
    return nil
}
```

---

## 10. 安全设计

### 10.1 认证安全

- 密码使用 `bcrypt`（cost=12）存储，不可逆
- Session ID 使用 `crypto/rand` 生成 32 字节随机值，Base64URL 编码
- Session Cookie 设置 `HttpOnly`、`SameSite=Strict`
- Session 默认有效期 8 小时
- 登录失败不区分"用户名不存在"和"密码错误"（统一返回"凭据无效"）

### 10.2 输入验证

```go
// 用户名：只允许字母、数字、下划线，3-32位
var validUsername = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

// 共享名：字母、数字、下划线、连字符，1-64位
var validShareName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// 路径：必须是绝对路径
func validatePath(path string) error {
    if !filepath.IsAbs(path) {
        return errors.New("路径必须是绝对路径")
    }
    // 禁止路径遍历
    cleaned := filepath.Clean(path)
    if cleaned != path {
        return errors.New("非法路径")
    }
    return nil
}
```

### 10.3 Auth 中间件

```go
func AuthMiddleware(sessionStore *session.SessionStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            cookie, err := r.Cookie("session_id")
            if err != nil {
                writeJSON(w, 401, map[string]any{"error": "Unauthorized"})
                return
            }
            sess, ok := sessionStore.Get(cookie.Value)
            if !ok {
                writeJSON(w, 401, map[string]any{"error": "Session expired"})
                return
            }
            // 将session信息注入context
            ctx := context.WithValue(r.Context(), contextKeySession, sess)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### 10.4 系统安全注意事项

- 仅监听 `127.0.0.1:8080`（默认），需要外网访问时应通过 nginx 反代并配置 HTTPS
- 建议通过防火墙限制对管理端口的访问
- 程序以 root 运行，但操作应最小化：只在需要时调用 `useradd`、`smbpasswd` 等特权命令
- 系统命令执行使用 `exec.Command`（非 shell 调用），避免命令注入

---

## 11. 配置文件

程序自身配置文件（非 smb.conf），默认路径 `/etc/smb-controller/config.yaml`：

```yaml
# SMB Controller 配置文件

server:
  listen: "0.0.0.0:8080"   # 监听地址
  # tls_cert: ""            # 可选，启用HTTPS
  # tls_key: ""

database:
  path: "/var/lib/smb-controller/data.db"

smb:
  conf_path: "/etc/samba/smb.conf"
  backup_dir: "/var/lib/smb-controller/conf-backups"
  backup_max_count: 5
  reload_command: "systemctl reload smbd"
  restart_command: "systemctl restart smbd"

session:
  ttl_hours: 8

log:
  level: "info"    # debug | info | warn | error
  file: "/var/log/smb-controller/app.log"
```

也支持环境变量覆盖（`SMB_CTRL_` 前缀），例如：

```bash
SMB_CTRL_SERVER_LISTEN=":9090"
SMB_CTRL_DATABASE_PATH="/data/smb.db"
```

---

## 12. 部署与运行

### 12.1 前提条件

```bash
# Ubuntu/Debian
apt-get install -y samba

# CentOS/RHEL
yum install -y samba

# 确认 smbd 可用
which smbd
smbd --version
```

### 12.2 编译

```bash
# 编译（包含 web 静态资源 embed）
go build -o smb-controller ./main.go

# 交叉编译 for linux/amd64
GOOS=linux GOARCH=amd64 go build -o smb-controller-linux-amd64 ./main.go
```

### 12.3 systemd 服务文件

```ini
# /etc/systemd/system/smb-controller.service
[Unit]
Description=SMB Controller - Web-based Samba Manager
After=network.target smbd.service
Wants=smbd.service

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/smb-controller --config /etc/smb-controller/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=smb-controller

# 安全加固（在root运行下的最小化限制）
NoNewPrivileges=false
PrivateTmp=false

[Install]
WantedBy=multi-user.target
```

```bash
# 安装并启动
install -m 755 smb-controller /usr/local/bin/
mkdir -p /etc/smb-controller /var/lib/smb-controller /var/log/smb-controller
cp config.yaml /etc/smb-controller/
systemctl daemon-reload
systemctl enable --now smb-controller
```

### 12.4 数据目录初始化

```bash
mkdir -p /var/lib/smb-controller
mkdir -p /var/lib/smb-controller/conf-backups
mkdir -p /var/log/smb-controller
chmod 700 /var/lib/smb-controller
```

---

## 13. 编码规范与依赖

### 13.1 Go 依赖清单

```go
// go.mod
module smb-controller

go 1.22

require (
    // HTTP 路由
    github.com/go-chi/chi/v5 v5.1.0
    
    // SQLite（纯Go，无CGO依赖）
    modernc.org/sqlite v1.31.0
    
    // 密码哈希
    golang.org/x/crypto v0.27.0
    
    // 配置文件解析
    gopkg.in/yaml.v3 v3.0.1
    
    // 日志
    go.uber.org/zap v1.27.0
    
    // UUID（session id生成备用）
    // 实际使用 crypto/rand，此包可选
)
```

**选型说明：**

- `chi`：轻量级路由，兼容 `net/http`，无过多魔法
- `modernc.org/sqlite`：纯 Go 实现的 SQLite，无需安装 gcc/CGO
- `golang.org/x/crypto`：官方 bcrypt 实现
- `gopkg.in/yaml.v3`：YAML 配置解析
- `go.uber.org/zap`：高性能结构化日志

### 13.2 main.go 骨架

```go
package main

import (
    "context"
    "flag"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "smb-controller/internal/config"
    "smb-controller/internal/database"
    "smb-controller/internal/handler"
    "smb-controller/internal/repository"
    "smb-controller/internal/service"
    "smb-controller/internal/session"
    "smb-controller/internal/smb"
    
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func main() {
    // 1. 参数解析
    configPath := flag.String("config", "/etc/smb-controller/config.yaml", "config file path")
    flag.Parse()
    
    // 2. 加载配置
    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatalf("failed to load config: %v", err)
    }
    
    // 3. 初始化数据库
    db, err := database.New(cfg.Database.Path)
    if err != nil {
        log.Fatalf("failed to open database: %v", err)
    }
    defer db.Close()
    
    if err := database.Migrate(db); err != nil {
        log.Fatalf("failed to migrate database: %v", err)
    }
    
    // 4. 检测 SMB 环境
    detector := smb.NewDetector()
    detectResult := detector.Detect()
    log.Printf("SMB detection: smbd_installed=%v, running=%v", 
        detectResult.SmbdInstalled, detectResult.SmbdRunning)
    
    // 5. 初始化各层
    repos := repository.NewRepositories(db)
    executor := smb.NewExecutor(cfg.Smb)
    generator := smb.NewGenerator(cfg.Smb.ConfPath, cfg.Smb.BackupDir)
    sessionStore := session.NewStore(time.Duration(cfg.Session.TTLHours) * time.Hour)
    
    services := service.NewServices(repos, executor, generator)
    
    // 6. 路由设置
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.RealIP)
    
    handler.RegisterRoutes(r, services, sessionStore, detectResult)
    
    // 7. 启动服务
    srv := &http.Server{
        Addr:         cfg.Server.Listen,
        Handler:      r,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    
    go func() {
        log.Printf("SMB Controller listening on %s", cfg.Server.Listen)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("server error: %v", err)
        }
    }()
    
    // 8. 优雅关闭
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    srv.Shutdown(ctx)
}
```

### 13.3 关键实现注意事项

1. **smb.conf 写入必须原子操作**：写临时文件 → `testparm` 验证 → `os.Rename` 原子替换，避免 smbd 读到损坏的配置文件。

2. **系统用户与 SMB 用户分离**：创建 SMB 用户时必须先确保 Linux 系统用户存在（`useradd`），再执行 `smbpasswd -a`；删除时反向执行。

3. **并发安全**：多个管理员同时操作时，smb.conf 的重新生成需要加互斥锁（`sync.Mutex`），避免竞态条件。

4. **错误处理**：所有系统命令调用必须捕获 stderr，并将错误信息包含在返回的 error 中，便于前端展示具体失败原因。

5. **前端静态文件 embed**：

```go
//go:embed web
var webFS embed.FS

// 使用 fs.Sub 提取 web 子目录
webSubFS, _ := fs.Sub(webFS, "web")
http.FileServer(http.FS(webSubFS))
```

6. **`testparm` 验证**：在应用 smb.conf 之前，使用 `testparm -s {tmpfile}` 验证语法正确性，防止错误配置导致 smbd 无法重载。

---

## 附录：开发顺序建议

建议 AI 智能体按以下顺序进行编码，每步完成后验证再继续：

| 阶段 | 任务 |
|------|------|
| Phase 1 | `go.mod`、`config`、`database`（建表+迁移） |
| Phase 2 | `models`、`repository` 层（纯 DB 操作） |
| Phase 3 | `smb/detector`、`smb/executor`（系统命令封装） |
| Phase 4 | `smb/generator`（smb.conf 生成逻辑） |
| Phase 5 | `session`、`service` 层（业务逻辑） |
| Phase 6 | `handler`（HTTP 处理，路由注册） |
| Phase 7 | `web/` 前端页面（HTML/CSS/JS） |
| Phase 8 | `main.go` 组装、集成测试 |
| Phase 9 | systemd 服务文件、README |