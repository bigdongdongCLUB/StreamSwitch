// 全局状态：当前登录用户与权限

const { reactive } = Vue

// 窄屏（手机）判定：与 app.css 的 @media (max-width: 768px) 断点保持一致
const mobileQuery = window.matchMedia('(max-width: 768px)')

export const store = reactive({
  user: null,

  // 系统是否尚未初始化（没有任何用户）：null=未检查
  needSetup: null,

  // 是否为移动端窄屏，供组件切换布局（如描述列表列数、对话框宽度）
  isMobile: mobileQuery.matches,

  // 从 localStorage 恢复（页面刷新后保持登录态）
  init() {
    try {
      const raw = localStorage.getItem('livems_user')
      this.user = raw ? JSON.parse(raw) : null
    } catch (e) {
      this.user = null
    }
  },

  setUser(u) {
    this.user = u
    if (u) localStorage.setItem('livems_user', JSON.stringify(u))
    else localStorage.removeItem('livems_user')
  },

  get isSuper() { return this.user && this.user.is_super === 1 },

  // 权限判断：超管全部放行
  has(code) {
    if (!this.user) return false
    if (this.user.is_super === 1) return true
    const perms = this.user.permissions || []
    return perms.includes(code)
  },
})

// 监听窗口尺寸变化，实时更新 isMobile
mobileQuery.addEventListener('change', e => { store.isMobile = e.matches })
