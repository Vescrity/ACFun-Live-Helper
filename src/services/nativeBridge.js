// WebUI API 辅助：WebUI 模式下通过 HTTP 调后端
const API_BASE = '/api'

async function apiGet(path) {
  try {
    const res = await fetch(API_BASE + path)
    if (!res.ok) return null
    return res.json()
  } catch {
    return null
  }
}

async function apiPost(path, body) {
  try {
    await fetch(API_BASE + path, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: String(body ?? ''),
    })
  } catch { /* ignore */ }
}

function wailsApp() {
  return window.go && window.go.main && window.go.main.App
    ? window.go.main.App
    : null
}

export async function openCoverFile() {
  const app = wailsApp()
  if (app && app.OpenCoverFile) return app.OpenCoverFile()
  // WebUI: 用浏览器原生文件选择
  return triggerFileInput('image/jpeg,image/png,image/webp,image/gif')
}

export async function readCoverFile(filePath) {
  const app = wailsApp()
  if (app && app.ReadCoverFile) return app.ReadCoverFile(filePath)
  const data = await apiGet(`/cover/read?path=${encodeURIComponent(filePath)}`)
  return data?.dataUrl ?? ""
}

export async function saveCoverImage(dataUrl) {
  const app = wailsApp()
  if (app && app.SaveCoverImage) return app.SaveCoverImage(dataUrl)
  const data = await apiPost('/cover/save', dataUrl)
  return dataUrl
}

export async function copyText(text) {
  const app = wailsApp()
  if (app && app.CopyText) return app.CopyText(text)
  if (navigator.clipboard) return navigator.clipboard.writeText(text)
}

export async function openExternalURL(url) {
  const app = wailsApp()
  if (app && app.OpenExternalURL) return app.OpenExternalURL(url)
  window.open(url, "_blank", "noopener,noreferrer")
}

export async function getSystemFonts() {
  const app = wailsApp()
  if (app && app.GetSystemFonts) return app.GetSystemFonts()
  const data = await apiGet('/fonts')
  return Array.isArray(data) ? data : []
}

export async function getOverlayBaseUrl() {
  const app = wailsApp()
  if (app && app.GetOverlayBaseUrl) return app.GetOverlayBaseUrl()
  const data = await apiGet('/overlay-url')
  return data?.error ?? (typeof data === 'string' ? data : "")
}

export async function getBackendPort() {
  const app = wailsApp()
  if (app && app.GetBackendPort) return app.GetBackendPort()
  const data = await apiGet('/backend-port')
  return data?.port ?? 0
}

export async function getLogPath() {
  const app = wailsApp()
  if (app && app.GetLogPath) return app.GetLogPath()
  const data = await apiGet('/log-path')
  return data?.path ?? ""
}

export async function appendLog(message) {
  const app = wailsApp()
  if (app && app.AppendLog) return app.AppendLog(String(message || ""))
  apiPost('/log', String(message || ""))
}

export async function openLogFolder() {
  const app = wailsApp()
  if (app && app.OpenLogFolder) return app.OpenLogFolder()
  apiPost('/log-folder')
}

export async function getSystemStats() {
  const app = wailsApp()
  if (app && app.GetSystemStats) return app.GetSystemStats()
  const data = await apiGet('/stats')
  return data ? { cpu: data.cpu ?? 0, memory: data.memory ?? 0 } : { cpu: 0, memory: 0 }
}

export async function getNetworkDelay() {
  const app = wailsApp()
  if (app && app.GetNetworkDelay) return app.GetNetworkDelay()
  const data = await apiGet('/delay')
  return data?.delay ?? -1
}

export async function setAlwaysOnTop(enabled) {
  const app = wailsApp()
  if (app && app.SetAlwaysOnTop) return app.SetAlwaysOnTop(enabled)
  // WebUI: 浏览器无法强制置顶，忽略
}

export async function setWindowSize(width, height) {
  const app = wailsApp()
  if (app && app.SetWindowSize) return app.SetWindowSize(width, height)
  // WebUI: 由浏览器管理窗口大小
}

export async function isMiniMode() {
  const app = wailsApp()
  if (app && app.IsMiniMode) return app.IsMiniMode()
  return false
}

export async function setMouseClickThrough(enable) {
  const app = wailsApp()
  if (app && app.SetMouseClickThrough) return app.SetMouseClickThrough(Boolean(enable))
  // WebUI: 不支持鼠标穿透
}

export async function setMouseClickThroughHotkey(mods, vk) {
  const app = wailsApp()
  if (app && app.SetMouseClickThroughHotkey) {
    return app.SetMouseClickThroughHotkey(Number(mods) || 0, Number(vk) || 0)
  }
  // WebUI: 不支持全局热键
}

export function onClickThroughToggle(handler) {
  const runtime = window.runtime
  if (runtime && runtime.EventsOn) {
    runtime.EventsOn("mini:click-through-toggle", () => {
      try { handler() } catch {}
    })
    return () => {
      if (runtime.EventsOff) runtime.EventsOff("mini:click-through-toggle")
    }
  }
  return () => {}
}

export async function launchMiniWindow() {
  const app = wailsApp()
  if (app && app.LaunchMiniWindow) return app.LaunchMiniWindow()
  // WebUI: 后端打开新浏览器窗口
  apiPost('/launch-mini')
}

export async function setSharedTheme(theme) {
  const app = wailsApp()
  if (app && app.SetSharedTheme) return app.SetSharedTheme(theme)
  apiPost('/theme', theme)
}

export async function getSharedTheme() {
  const app = wailsApp()
  if (app && app.GetSharedTheme) return app.GetSharedTheme()
  const data = await apiGet('/theme')
  return data?.theme ?? ""
}

export async function setSharedFloatState(payload) {
  const app = wailsApp()
  if (app && app.SetSharedFloatState) return app.SetSharedFloatState(payload)
  apiPost('/float-state', payload)
}

export async function getSharedFloatState() {
  const app = wailsApp()
  return app && app.GetSharedFloatState ? app.GetSharedFloatState() : ""
}

export async function broadcastOverlayStyle(payload) {
  const app = wailsApp()
  if (app && app.BroadcastOverlayStyle) return app.BroadcastOverlayStyle(String(payload || ""))
  apiPost('/overlay-style', String(payload || ""))
}

export async function downloadPlaybackToFile(url, suggestedName) {
  const app = wailsApp()
  if (app && app.DownloadPlaybackToFile) return app.DownloadPlaybackToFile(String(url || ""), String(suggestedName || ""))
  if (url) window.open(url, "_blank", "noopener,noreferrer")
  return ""
}

// 浏览器环境下触发 `<input type=file>` 选择文件
function triggerFileInput(accept) {
  return new Promise((resolve) => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = accept
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) { resolve(""); return }
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result)
      reader.readAsDataURL(file)
    }
    input.click()
  })
}
