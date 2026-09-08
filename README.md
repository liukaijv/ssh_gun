# 飞梭（Feisuo）

Windows 桌面 SSH 开发者工具（Go + Wails + Vue3/Naive UI）。

## 功能

- **服务器管理**：密码 / 私钥认证，连接测试；凭据用 Windows DPAPI 加密后写入 TOML
- **目录同步**：本地 → 远程；设置中可切换 **SFTP**（远端只需 sshd）或 **rsync**（内置 cwrsync）；排除规则、`.gitignore`、手动全量同步、fsnotify 自动同步
- **端口转发**：local / remote / dynamic（SOCKS5）
- **托管进程**：本地命令启停、工作目录、进程级环境变量、开机自启；与 SSH 类自启并行，互不阻塞
- **系统托盘**：关闭窗口隐藏到托盘；托盘菜单「退出」才真正退出
- **单实例**：再次启动会唤起已有窗口，不启第二主界面（rsync 桥接子进程除外）
- **配置导入/导出**：可加密导出，支持合并或整表替换
- **日志**：`slog` 写入 `%LOCALAPPDATA%\ssh_gun\logs\`（10MB 轮转，保留 7 天）+ 界面实时环形缓冲

配置文件：`%APPDATA%\ssh_gun\config.toml`（启动时读取，不做热加载；手改后需重启）。

## 开发

```bash
# 依赖
go version   # 1.25+
node -v
wails version

# 前端
cd frontend && npm install && npm run test && npm run build

# 开发运行
wails dev

# 打包 → build/bin/Feisuo.exe
wails build
```

## 同步说明

默认后端为 `sftp`：

- 增量：监听本地变更后推送
- 全量：本地扫描 + 远端 `find -printf`（GNU）；BusyBox 等不支持时回退到 SFTP 遍历
- 批量：文件较多时可走 tar 流（后端内部策略）

`rsync` 后端在设置中切换；通过 `--rsh-bridge` 子进程复用本机凭据，无需远端另装专用 agent。

## 目录结构

`internal/` 主要包：

| 包 | 作用 |
|----|------|
| `config` | TOML 配置、DPAPI、导入导出 |
| `sshclient` | SSH 连接池 |
| `forward` | 端口转发 / SOCKS5 |
| `syncengine` | 同步引擎（`sftpbackend` / `rsyncbackend`） |
| `rsyncbin` | 内置 cwrsync 解压 |
| `rshbridge` | rsync `--rsh` 桥接子进程 |
| `procman` | 托管进程 |
| `singleinstance` | 单实例 + 二次启动唤起 |
| `applog` | 日志轮转与环形缓冲 |
| `testsshd` | 测试用 SSH 服务 |
