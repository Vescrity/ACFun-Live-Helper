# ACFun Live Helper

ACFun Live Helper 是一个面向 AcFun 直播场景的桌面端辅助工具，基于 Wails v2、Go、Vue 3、Vite 和 Pinia 构建，内置 `acfunlive-backend` 相关能力。

## 功能特性

- 直播开播与关播
- 开播标题与封面修改
- GIF 封面支持
- 房间观众管理
- 拉黑观众
- 房管管理
- 弹幕发送
- 弹幕透明置顶显示
- 弹幕发送置顶窗
- 弹幕播报
- 弹幕互动
- OBS 推流码修改
- 投喂列表查看
- 下播后的直播详情查看

## 技术栈

- Wails v2
- Go 1.21+
- Vue 3
- Vite
- Pinia

## 环境要求

- Go 1.21 或更高版本
- Node.js 18 或更高版本
- Wails v2 CLI
- Windows 构建环境推荐使用 PowerShell

## 安装依赖

```powershell
npm install
go mod tidy
```

## 开发

```powershell
npm run wails:dev
```

也可以直接运行：

```powershell
wails dev
```

## 构建

构建当前平台：

```powershell
npm run wails:build
```

构建 Windows amd64：

```powershell
npm run wails:build:win
```

也可以使用 Windows 一键构建脚本：

```powershell
powershell -ExecutionPolicy Bypass -File .\build-windows.ps1
```

构建产物默认位于：

```text
build\bin\ACFun Live Helper.exe
```

## 后端客户端测试

```powershell
npm run test:backend-client
```

## 项目结构

```text
.
├── backend/              # Go 后端命令与业务封装
├── public/               # 静态资源
├── src/                  # Vue 前端源码
│   ├── services/         # 前端服务封装
│   ├── stores/           # Pinia 状态管理
│   └── utils/            # 通用工具
├── third_party/          # 第三方 Go 依赖源码
├── wailsjs/              # Wails 生成的前后端桥接代码
├── app.go                # Wails 应用入口
├── main.go               # Go 主入口
├── package.json          # 前端脚本与依赖
└── wails.json            # Wails 项目配置
```

## 下载

可以前往当前仓库的 Releases 页面下载已发布版本：

```text
https://github.com/epstomai/ACFun-Live-Helper/releases
```
