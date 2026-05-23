# ACFun Live Helper

[![Release](https://img.shields.io/github/v/release/epstomai/ACFun-Live-Helper?include_prereleases)](https://github.com/epstomai/ACFun-Live-Helper/releases)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Wails](https://img.shields.io/badge/Wails-v2-red)](https://wails.io/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883)](https://vuejs.org/)
[![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8)](https://go.dev/)

面向 AcFun 直播主播的现代化桌面端助手。基于 **Go + Vue 3 + Pinia** 构建，内嵌 `acfunlive-backend`，无需额外启动后端进程。

支持**双构建模式**：

| 模式 | 命令 | 依赖 | 体积 |
|------|------|------|------|
| **WebUI**（默认） | `go build` | 无（纯 Go 静态链接） | ~19MB |
| **Wails 桌面版** | `go build -tags 'wails webkit2_41'` | 需 webkit2gtk-4.1 + GTK3 | ~20MB |

WebUI 模式下以本地 HTTP 服务运行，自动打开系统浏览器，**无需 WebView2 / WebKit**，可在任何有浏览器的平台上运行。

> **当前版本**：`v1.0.0-rc.3` — [前往 Releases 下载](https://github.com/epstomai/ACFun-Live-Helper/releases/latest)

---

## 目录

- [功能特性](#功能特性)
- [技术栈](#技术栈)
- [环境要求](#环境要求)
- [开发](#开发)
- [构建](#构建)
- [双模式对比](#双模式对比)
- [项目结构](#项目结构)
- [下载](#下载)
- [已知问题 / FAQ](#已知问题--faq)
- [开源协议](#开源协议)
- [致谢](#致谢)

## 功能特性

### 开播
- 一键开播 / 关播；标题、封面（支持 GIF）、主分类与子分类编辑
- OBS 推流码自动同步 + 一键复制；推流码未变化不刷屏
- 转码 / 推流通道信息实时显示
- 直播录像剪辑权限开关（`SET_LIVE_CUT_STATUS`），支持打开剪辑页 / 复制原始链接
- 本场计时实时刷新，跨重启 / 切账号不丢失（按 UID 持久化）

### 直播间
- 弹幕 / 礼物 / 红包 / 关注 / 加入守护团 / 进场提示 实时流
- 在线观众列表（含粉丝牌等级、贡献排名）；房管管理（搜索 / 加管 / 取消管理 / 踢出 / 拉黑 / 解除拉黑）
- 弹幕互动：发送弹幕、颜文字快捷输入
- 投喂榜（贡献排行）实时刷新

### 弹幕源 (OBS 浏览器源)
- 后端固定本地端口 (`http://127.0.0.1:15370/danmaku-overlay.html`)，URL 不再每次启动变化，OBS 不需要反复改源
- **SSE 实时推送样式**：在助手里调字号 / 颜色 / 缩放 / 动画 / 透明度 / 圆角等，OBS 浏览器源自动同步，**无需手动刷新**
- 用户名与弹幕内容可分别指定字体；可搜索字体选择器，键盘上下键实时预览，未确认自动回滚
- 气泡 / 文本颜色支持 HSV 选色器 + HEX / RGBA 输入 + Alpha 通道，弹层四象限自适应避免被边缘截断
- 一键预览动画效果（内置示例弹幕池）

### 数据
- 本场直播实时数据：观看 / 点赞 / 钻石 / 礼物 / 香蕉，关播后自动写入"本场总结"
- 直播历史按账号持久化：曲线（弹幕 / 礼物 / 观看）、原始事件时间轴
- **录播下载**：解析 HLS `.m3u8` 索引、流式下载并拼接 `.ts` 分片，弹原生保存对话框选路径，写入本地（用 VLC 直接播放，可用 ffmpeg 转 mp4）
- 录播在线回放（系统浏览器打开签名链接）；历史记录可单条删除
- 自动关播识别：`DANMU_STOP / loadRoom / loadLiveStatus` 任一发现关播都触发收尾，写入历史并刷新本场总结，避免漏记

### 悬浮 & 主题（仅 Wails 桌面版）
- 暗 / 亮主题切换，自适应配色
- 侧栏可折叠为图标，支持手动展开
- **悬浮置顶弹幕窗**（始终在最前），支持以下鼠标穿透与设置特性：
  - **👻 鼠标穿透**：左键点击 👻 按钮进入穿透状态，鼠标事件落到下层窗口
  - **🎚️ 三态设置循环**：Header 置顶设置按钮可循环切换底部状态
  - **⌨️ 全局热键改键**：可自定义鼠标穿透退出热键（默认 `Ctrl+Alt+Shift+G`）

### 系统
- CPU / 内存 / 网络延迟实时监控
- 内置日志面板，支持打开日志文件夹快速排错
- 内嵌 `acfunlive-backend`，无需单独启动 / 配置后端

## 技术栈

- **Go 1.21+** — 主进程、HTTP / SSE / 系统监控 / 文件下载
- **Vue 3 + Pinia** — 前端 UI 与状态管理
- **Vite 8** — 前端构建
- **Lucide Vue** — 图标
- **Wails v2**（可选）— Go ↔ Webview2 桥接（仅 Wails 桌面版）
- **acfunlive-backend / acfundanmu** — A 站弹幕协议与开播 API

## 环境要求

### WebUI 模式（推荐 Linux）

| 需求 | 说明 |
|------|------|
| Go 1.21+ |  |
| Node.js 18+ | 构建前端用 |
| 浏览器 | Chrome / Firefox / Edge 等现代浏览器 |

无需安装 WebView2、WebKit、GTK。

### Wails 桌面版

**Windows**
- Windows 10 / 11
- [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)（Win11 已内置）

**Linux**
- webkit2gtk-4.1
- GTK+ 3.0
- 安装参考（Arch Linux）：
  ```bash
  sudo pacman -S webkit2gtk-4.1 gtk3
  ```

### 开发工具链（通用）

- Go 1.21 或更高
- Node.js 18 或更高
- Wails v2 CLI（仅 Wails 模式需要）：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## 开发

安装依赖：

```bash
npm install
go mod tidy
```

### WebUI 开发模式

浏览器打开 `http://localhost:5173`，Go 后端在 `http://localhost:15369`：

```bash
# 终端 1：前端热重载
npm run dev

# 终端 2：Go 后端
npm run dev:webui
```

### Wails 开发模式（Linux）

前端热重载 + Wails 自动重启 Go 后端：

```bash
npm run wails:dev
```

### 测试

```bash
npm run test:backend-client
```

## 构建

### WebUI 版（推荐 Linux，无 CGO 依赖）

```bash
# 构建前端
npm run build

# 编译纯 Go 二进制（静态链接）
npm run build:webui

# 或直接
CGO_ENABLED=0 go build -o build/bin/aclivehelper-webui .
```

运行：

```bash
./build/bin/aclivehelper-webui
```

自动打开浏览器访问 `http://127.0.0.1:15369`。

### Wails 桌面版（Windows / Linux）

```bash
npm run wails:build
```

指定 Linux/webkit2gtk-4.1：

```bash
npm run wails:build
# 背后执行：wails build -tags 'wails webkit2_41'
```

构建 Windows amd64：

```bash
npm run wails:build:win
```

Windows 一键构建脚本（含环境校验 + 依赖安装）：

```powershell
powershell -ExecutionPolicy Bypass -File .\build-windows.ps1
```

Wails 构建产物默认位于 `build/bin/ACFun Live Helper.exe`。

## 双模式对比

| 特性 | WebUI 模式 | Wails 桌面版 |
|------|-----------|-------------|
| CGO | ❌ 无 | ✅ 有 |
| 系统依赖 | 仅浏览器 | webkit2gtk / WebView2 |
| 二进制体积 | ~19MB | ~20MB |
| 链接方式 | 静态链接 | 动态链接 |
| 悬浮置顶弹幕窗 | ❌ 新浏览器标签页 | ✅ 原生窗口 |
| 鼠标穿透 / 全局热键 | ❌ | ✅（仅 Windows） |
| 窗口置顶 | 浏览器手动管理 | ✅ 原生支持 |
| 系统文件对话框 | 浏览器 `<input type=file>` / `<a download>` | ✅ 原生对话框 |
| 剪贴板 | `navigator.clipboard` | ✅ wails runtime |

> WebUI 模式适合 Linux 用户（避免 webkit2gtk 依赖）和需要轻量运行的用户。Wails 模式保留完整的桌面体验。

## 项目结构

```text
.
├── backend/              # Go 后端命令与业务封装
├── public/               # 静态资源
├── src/                  # Vue 前端源码
│   ├── services/         # 前端服务封装（nativeBridge.js 适配双模式）
│   ├── stores/           # Pinia 状态管理
│   └── utils/            # 通用工具
├── third_party/          # 第三方 Go 依赖源码
├── wailsjs/              # Wails 生成的前后端桥接代码（仅 Wails 模式）
├── app.go                # 共享业务逻辑
├── bridge_wails.go       # Wails 特有实现                    [build tag: wails]
├── bridge_webui.go       # WebUI HTTP API 实现               [build tag: !wails]
├── main.go               # Wails 入口                        [build tag: wails]
├── main_webui.go         # WebUI 入口                        [build tag: !wails]
├── platform_*.go         # 平台窗口配置                       [build tag: *_&&wails]
├── main_*_*.go           # 跨平台 watchParentProcess          [build tag: *_&&wails]
├── mouse_clickthrough_*.go # 鼠标穿透/全局热键                  [build tag: *_&&wails]
├── sysstats_*.go         # 系统统计/字体获取（共享）
├── package.json          # 前端脚本与依赖
└── wails.json            # Wails 项目配置
```

## 下载

前往 Releases 页面下载已发布版本：

- 最新版本：<https://github.com/epstomai/ACFun-Live-Helper/releases/latest>
- 全部版本：<https://github.com/epstomai/ACFun-Live-Helper/releases>

Windows 用户下载 `ACFun Live Helper.exe` 双击即可运行。Linux 用户建议自行编译 WebUI 版本。

首次启动会在 `%AppData%\aclivehelper` 创建账号配置目录。

## 已知问题 / FAQ

- **登录失败 / 滑块验证**：A 站风控会偶发要求滑块验证，本助手暂未集成滑块求解，请到网页版手动通过一次后再尝试。
- **录播下载文件后缀是 `.ts`**：A 站录播以 HLS 切片下发，拼接结果就是 MPEG-TS 流。VLC 可直接播放；如需 mp4：

  ```bash
  ffmpeg -i "xxx.ts" -c copy "xxx.mp4"
  ```

- **OBS 浏览器源不刷新**：本版本已改用 SSE 实时推送，如果仍未生效，请确认 URL 端口是 `:15370` 且 OBS 浏览器源未启用 `Shutdown source when not visible`。
- **Linux 上无法启动 Wails 版**：确认已安装 webkit2gtk-4.1（系统没有 4.0 版本），使用 `npm run wails:build` 自动携带 `webkit2_41` tag。推荐直接使用 WebUI 模式，无需任何系统库。
- **本场总结里时长 00:00:00**：通常是关播时 `GET_SUMMARY` 返回失败，助手会用本场计时器估算 duration 兜底，下次进入数据页会再尝试拉取真实总结。

## 开源协议

本项目采用 **[GNU GPL v3](LICENSE)** 开源协议进行授权分发。

依据 GPL v3 协议规范，任何基于本项目的分发、修改与二次开发行为，均需**保持开源**并沿用 GPL v3 协议。

## 致谢

- **[acfundanmu](https://github.com/orzogc/acfundanmu)** by **@orzogc** — A 站弹幕协议与开播 API 的核心实现，本助手的 `third_party/acfundanmu` 完整内嵌该项目源码。
- **[acfunlive-backend](https://github.com/orzogc/acfunlive-backend)** by **@orzogc** — 直播会话管理 / WebSocket / 命令分发后端，本助手的 `backend/` 在其基础上裁剪集成。
- **[Wails](https://wails.io/)** — Go + Webview2 桌面应用方案。
- **[Lucide](https://lucide.dev/)** — 简洁现代的图标集。

非常感谢上述项目让 ACFun Live Helper 成为可能。
