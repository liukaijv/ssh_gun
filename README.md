# ssh_gun

Windows 桌面 SSH 开发者工具（Go + Wails + Vue3/Naive UI）。

## 功能

- **服务器管理**：密码 / 私钥认证，连接测试；凭据用 Windows DPAPI 加密后写入 TOML
- **目录同步**：本地 → 远程，SFTP 后端（远端只需 sshd）；支持排除规则、手动全量同步、fsnotify 自动同步
- **端口转发**：例如本地 `3316` → 远程 `3306`
- **日志**：`slog` 写入 `%LOCALAPPDATA%\ssh_gun\logs\`（10MB 轮转，保留 7 天）+ 界面实时环形缓冲

配置文件：`%APPDATA%\ssh_gun\config.toml`（启动时读取，不做热加载；手改后需重启）。

## 开发

```bash
# 依赖
go version   # 1.24+
node -v
wails version

# 前端
cd frontend && npm install && npm run test && npm run build

# Go 测试（默认不含 Docker 集成）
go test ./...

# Docker 集成（需 Docker Desktop）
go test -tags=integration ./test/integration/...

# 开发运行
wails dev

# 打包
wails build
```

## 同步说明

默认后端为 `sftp`：

- 增量：监听本地变更后推送
- 全量：本地扫描 + 远端 `find -printf`（GNU）；BusyBox 等不支持时回退到 SFTP 遍历
- 批量：文件较多时可走 tar 流（后端内部阈值）

可选 `rsync` 后端（内置 cwrsync）见 `internal/syncengine/rsyncbackend`，映射里将 `backend` 设为 `rsync`。

## 删除服务器

若仍有目录映射或端口转发引用该服务器，删除会被拒绝，并提示依赖名称。

## 目录结构

见仓库 `internal/`：`config`、`sshclient`、`forward`、`syncengine`、`applog`、`testsshd`。
