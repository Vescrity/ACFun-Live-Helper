// TTS 弹幕语音播报服务
// 基于浏览器内置 Web Speech API (SpeechSynthesis)，零依赖。
// 通过 useTTS() 获取单例，调 init() 初始化语音，speak() 播报。

import { ref } from "vue"

let instance = null

export function useTTS() {
  if (instance) return instance

  const available = ref(typeof window !== "undefined" && "speechSynthesis" in window)
  const initialized = ref(false)
  const voices = ref([])
  let voice = null

  // 可调的参数，外部可改写
  const config = {
    rate: 1.1,
    volume: 0.8,
    pitch: 1,
  }

  function init() {
    if (!available.value || initialized.value) return
    const synth = window.speechSynthesis
    const loadVoices = () => {
      voices.value = synth.getVoices()
      voice = pickVoice(voices.value)
      if (voice) initialized.value = true
    }
    loadVoices()
    // Chrome 异步加载 voices
    synth.onvoiceschanged = () => {
      loadVoices()
    }
  }

  // 优先中文女声，其次任何中文，否则默认
  function pickVoice(list) {
    // 二线
    const chinese = list.filter((v) => v.lang.startsWith("zh"))
    if (!chinese.length) return list[0] || null

    // 偏女声
    const female = chinese.find((v) => /female|xiaoxiao|yaoyao|kangkang/i.test(v.name + v.voiceURI))
    if (female) return female

    return chinese[0]
  }

  function speak(text) {
    if (!available.value || !initialized.value || !voice) return
    const synth = window.speechSynthesis
    // 如果正在播放，不打断（弹幕排队自然消耗）
    if (synth.speaking) return
    const u = new SpeechSynthesisUtterance(text)
    u.voice = voice
    u.lang = voice.lang
    u.rate = config.rate
    u.volume = config.volume
    u.pitch = config.pitch
    synth.speak(u)
  }

  instance = { available, initialized, voices, config, init, speak }
  return instance
}

// 将弹幕 item 格式化为适合朗读的文本
export function danmakuToText(item) {
  const name = item.nickname || ""
  switch (item.type) {
    case "ADD_GIFT":
      return `${name} 送出 ${item.num} 个${item.content}`
    case "JOIN_ROOM":
      return `${name} 进入直播间`
    case "ADD_FOLLOW":
      return `${name} 关注了主播`
    case "ADD_JOIN_GROUP":
      return `${name} ${item.content || "加入守护团"}`
    default:
      return `${name}：${item.content || ""}`
  }
}

// 判断该类型的弹幕是否应该播报（由外部 filter 控制）
export function shouldSpeakDanmaku(item, filter) {
  if (!item) return false
  switch (item.type) {
    case "ADD_GIFT":
      return filter.filterGift !== false
    case "JOIN_ROOM":
      return filter.filterJoin !== false
    case "ADD_FOLLOW":
      return filter.filterFollow !== false
    default:
      return true // 普通弹幕始终播报
  }
}
