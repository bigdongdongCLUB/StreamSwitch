// API 客户端封装：统一令牌、错误处理、401 跳转

const BASE = '/api/v1'

export function getToken() { return localStorage.getItem('livems_token') || '' }
export function setToken(t) { t ? localStorage.setItem('livems_token', t) : localStorage.removeItem('livems_token') }

async function request(method, path, { params, body } = {}) {
  let url = BASE + path
  if (params) {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') qs.append(k, v)
    }
    const s = qs.toString()
    if (s) url += '?' + s
  }
  const headers = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers['Authorization'] = 'Bearer ' + token

  let resp
  try {
    resp = await fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch (e) {
    throw new Error('网络请求失败，请检查网络连接')
  }

  let data = null
  try { data = await resp.json() } catch (e) { /* 非 JSON 响应 */ }

  if (resp.status === 401) {
    setToken('')
    localStorage.removeItem('livems_user')
    if (!location.hash.startsWith('#/login')) {
      ElementPlus.ElMessage.error('登录已失效，请重新登录')
      location.hash = '#/login'
    }
    throw new Error((data && data.message) || '未登录或登录已失效')
  }

  if (!resp.ok || !data) {
    throw new Error((data && data.message) || `请求失败 (HTTP ${resp.status})`)
  }
  if (data.code !== 0) {
    throw new Error(data.message || '操作失败')
  }
  return data.data
}

// 带进度的文件上传（XHR，fetch 不支持上传进度）
export function uploadWithProgress(path, formData, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', BASE + path)
    const token = getToken()
    if (token) xhr.setRequestHeader('Authorization', 'Bearer ' + token)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      let d = null
      try { d = JSON.parse(xhr.responseText) } catch (e) { /* ignore */ }
      if (xhr.status === 401) {
        setToken('')
        localStorage.removeItem('livems_user')
        location.hash = '#/login'
        return reject(new Error('登录已失效，请重新登录'))
      }
      if (xhr.status === 200 && d && d.code === 0) return resolve(d.data)
      reject(new Error((d && d.message) || `上传失败 (HTTP ${xhr.status})`))
    }
    xhr.onerror = () => reject(new Error('网络错误，上传失败'))
    xhr.send(formData)
  })
}

const get = (path, params) => request('GET', path, { params })
const post = (path, body, params) => request('POST', path, { body, params })
const put = (path, body) => request('PUT', path, { body })
const del = (path) => request('DELETE', path)

export const api = {
  // ---- 认证 ----
  login: (username, password) => post('/auth/login', { username, password }),
  logout: () => post('/auth/logout'),
  profile: () => get('/auth/profile'),
  changePassword: (old_password, new_password) => post('/auth/password', { old_password, new_password }),

  // ---- 流管理 ----
  listStreams: (params) => get('/streams', params),
  getStream: (id) => get(`/streams/${id}`),
  getStreamInfo: (id) => get(`/streams/${id}/info`),
  getStreamURLs: (id, domain_id) => get(`/streams/${id}/urls`, { domain_id }),
  createStream: (body) => post('/streams', body),
  updateStream: (id, body) => put(`/streams/${id}`, body),
  toggleStream: (id) => post(`/streams/${id}/toggle`),
  deleteStream: (id) => del(`/streams/${id}`),
  syncStreams: () => post('/streams/sync'),
  genStreamURL: (body) => post('/streams/urls', body),

  // ---- 拉流代理 ----
  listProxies: (params) => get('/proxies', params),
  getProxy: (id) => get(`/proxies/${id}`),
  createProxy: (body) => post('/proxies', body),
  updateProxy: (id, body) => put(`/proxies/${id}`, body),
  toggleProxy: (id) => post(`/proxies/${id}/toggle`),
  startProxy: (id) => post(`/proxies/${id}/start`),
  stopProxy: (id) => post(`/proxies/${id}/stop`),
  deleteProxy: (id) => del(`/proxies/${id}`),

  // ---- 转推流 ----
  listForwards: (params) => get('/forwards', params),
  getForward: (id) => get(`/forwards/${id}`),
  createForward: (body) => post('/forwards', body),
  updateForward: (id, body) => put(`/forwards/${id}`, body),
  toggleForward: (id) => post(`/forwards/${id}/toggle`),
  deleteForward: (id) => del(`/forwards/${id}`),

  // ---- 播放域名 ----
  listDomains: () => get('/domains'),
  createDomain: (body) => post('/domains', body),
  updateDomain: (id, body) => put(`/domains/${id}`, body),
  setDefaultDomain: (id) => post(`/domains/${id}/default`),
  deleteDomain: (id) => del(`/domains/${id}`),

  // ---- 播放格式 ----
  listFormats: () => get('/formats'),
  toggleFormat: (format, enabled) => post(`/formats/${format}/toggle`, { enabled }),

  // ---- 鉴权策略 ----
  getPolicy: () => get('/auth-policy'),
  setPolicy: (body) => put('/auth-policy', body),

  // ---- 播放令牌 ----
  listTokens: (params) => get('/tokens', params),
  createToken: (body) => post('/tokens', body),
  deleteToken: (id) => del(`/tokens/${id}`),

  // ---- HLS 加密 ----
  listEncrypts: () => get('/encrypts'),
  getEncrypt: (streamId) => get(`/encrypts/${streamId}`),
  setEncrypt: (streamId, body) => put(`/encrypts/${streamId}`, body),
  rotateEncrypt: (streamId) => post(`/encrypts/${streamId}/rotate`),
  deleteEncrypt: (streamId) => del(`/encrypts/${streamId}`),

  // ---- 用户/角色/权限 ----
  listUsers: (params) => get('/users', params),
  getUser: (id) => get(`/users/${id}`),
  createUser: (body) => post('/users', body),
  updateUser: (id, body) => put(`/users/${id}`, body),
  resetUserPassword: (id, new_password) => post(`/users/${id}/password`, { new_password }),
  deleteUser: (id) => del(`/users/${id}`),
  listRoles: () => get('/roles'),
  createRole: (body) => post('/roles', body),
  updateRole: (id, body) => put(`/roles/${id}`, body),
  deleteRole: (id) => del(`/roles/${id}`),
  rolePermissions: (id) => get(`/roles/${id}/permissions`),
  listPermissions: () => get('/permissions'),

  // ---- 直播管理（分享页）----
  listLivePages: (params) => get('/live-pages', params),
  createLivePage: (body) => post('/live-pages', body),
  updateLivePage: (id, body) => put(`/live-pages/${id}`, body),
  deleteLivePage: (id) => del(`/live-pages/${id}`),

  // ---- 制播管理 ----
  listStudioMaterials: (type) => get('/studio/materials', { type }),
  uploadStudioMaterial: (formData, onProgress) => uploadWithProgress('/studio/materials', formData, onProgress),
  deleteStudioMaterial: (id) => del(`/studio/materials/${id}`),
  listStudioTasks: (params) => get('/studio/tasks', params),
  createStudioTask: (body) => post('/studio/tasks', body),
  updateStudioTask: (id, body) => put(`/studio/tasks/${id}`, body),
  startStudioTask: (id) => post(`/studio/tasks/${id}/start`),
  stopStudioTask: (id) => post(`/studio/tasks/${id}/stop`),
  toggleStudioTask: (id) => post(`/studio/tasks/${id}/toggle`),
  deleteStudioTask: (id) => del(`/studio/tasks/${id}`),

  // ---- 日志审计 ----
  listLogs: (params) => get('/logs', params),
  listTaskErrors: (params) => get('/task-errors', params),

  // ---- 系统设置 ----
  getIngress: () => get('/settings/ingress'),
  saveIngress: (body) => put('/settings/ingress', body),

  // ---- 仪表盘统计 ----
  trafficTrend: (minutes) => get('/stats/traffic', { minutes }),

  // ---- SSL 证书 ----
  listCerts: () => get('/certs'),
  saveCert: (body) => post('/certs', body),
  applyCert: (name) => post(`/certs/${encodeURIComponent(name)}/apply`),
  deleteCert: (name) => del(`/certs/${encodeURIComponent(name)}`),
}
