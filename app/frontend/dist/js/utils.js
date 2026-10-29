// 工具函数

// 格式化 Unix 时间戳（秒）为本地时间字符串
export function fmtTime(ts) {
  if (!ts || ts <= 0) return '-'
  const d = new Date(ts * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// 格式化字节数
export function fmtBytes(n) {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 2)} ${units[i]}`
}

// 复制文本到剪贴板
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch (e) {
    // 回退方案
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    let ok = false
    try { ok = document.execCommand('copy') } catch (_) { /* ignore */ }
    document.body.removeChild(ta)
    return ok
  }
}

// 秒数转可读时长
export function fmtDuration(sec) {
  if (!sec || sec <= 0) return '永久'
  if (sec < 60) return `${sec} 秒`
  if (sec < 3600) return `${Math.floor(sec / 60)} 分钟`
  if (sec < 86400) return `${Math.floor(sec / 3600)} 小时`
  return `${Math.floor(sec / 86400)} 天`
}
