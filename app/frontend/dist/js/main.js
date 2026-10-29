// 应用入口：注册依赖、路由与全局组件

import { store } from './store.js'
import { api, setToken, getToken } from './api.js'

import LoginView from './views/login.js'
import PlayPageView from './views/play-page.js'
import LayoutView from './views/layout.js'
import DashboardView from './views/dashboard.js'
import StreamsView from './views/streams.js'
import ProxiesView from './views/proxies.js'
import ForwardsView from './views/forwards.js'
import StudioView from './views/studio.js'
import LivePagesView from './views/live-pages.js'
import DomainsView from './views/domains.js'
import PlaySettingView from './views/play-setting.js'
import TokensView from './views/tokens.js'
import EncryptsView from './views/encrypts.js'
import UsersView from './views/users.js'
import RolesView from './views/roles.js'
import LogsView from './views/logs.js'

const { createApp } = Vue

// ---------------- 路由 ----------------
const routes = [
  { path: '/login', name: 'login', component: LoginView, meta: { title: '登录' } },
  // 观众直播播放页（免登录）
  { path: '/p/:code', name: 'play-page', component: PlayPageView, meta: { title: '直播', public: true } },
  {
    path: '/',
    component: LayoutView,
    children: [
      { path: '', name: 'dashboard', component: DashboardView, meta: { title: '仪表盘' } },
      { path: 'streams', name: 'streams', component: StreamsView, meta: { title: '流管理', perm: 'stream:list' } },
      { path: 'live-pages', name: 'live-pages', component: LivePagesView, meta: { title: '直播管理', perm: 'livepage:list' } },
      { path: 'proxies', name: 'proxies', component: ProxiesView, meta: { title: '拉流代理', perm: 'proxy:list' } },
      { path: 'forwards', name: 'forwards', component: ForwardsView, meta: { title: '转推流', perm: 'forward:list' } },
      { path: 'studio', name: 'studio', component: StudioView, meta: { title: '制播管理', perm: 'studio:list' } },
      { path: 'domains', name: 'domains', component: DomainsView, meta: { title: '域名配置', perm: 'domain:list' } },
      { path: 'play-setting', name: 'play-setting', component: PlaySettingView, meta: { title: '播放设置', perm: 'format:list' } },
      { path: 'tokens', name: 'tokens', component: TokensView, meta: { title: '播放令牌', perm: 'token:list' } },
      { path: 'encrypts', name: 'encrypts', component: EncryptsView, meta: { title: 'HLS 加密', perm: 'encrypt:list' } },
      { path: 'users', name: 'users', component: UsersView, meta: { title: '用户管理', perm: 'user:list' } },
      { path: 'roles', name: 'roles', component: RolesView, meta: { title: '角色权限', perm: 'role:list' } },
      { path: 'logs', name: 'logs', component: LogsView, meta: { title: '日志审计', perm: 'log:list' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = VueRouter.createRouter({
  history: VueRouter.createWebHashHistory(),
  routes,
})

router.beforeEach((to) => {
  document.title = (to.meta.title ? to.meta.title + ' - ' : '') + '流媒体管理系统'
  // 公开页面（观众播放页）不校验登录
  if (to.meta.public) return true
  const loggedIn = !!getToken() && !!store.user
  if (to.path === '/login') {
    return loggedIn ? '/' : true
  }
  if (!loggedIn) return '/login'
  if (to.meta.perm && !store.has(to.meta.perm)) {
    ElementPlus.ElMessage.warning('没有访问该页面的权限')
    return '/'
  }
  return true
})

// 登录失效时（api.js 清掉 token 后）兜底跳转
router.afterEach(() => {})

// ---------------- 应用 ----------------
store.init()

const app = createApp({
  setup() { return {} },
  template: '<router-view></router-view>',
})

app.use(router)
app.use(ElementPlus, { locale: ElementPlusLocaleZhCn })

// 注册 Element Plus 图标为全局组件
for (const [name, comp] of Object.entries(ElementPlusIconsVue)) {
  app.component(name, comp)
}

// 全局注入，便于模板内直接使用
app.config.globalProperties.$store = store
app.config.globalProperties.$api = api
app.provide('store', store)

app.mount('#app')

export { router, store, api, setToken }
