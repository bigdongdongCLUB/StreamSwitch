// 主布局：侧边菜单 + 顶栏 + 内容区

import { api, setToken } from '../api.js'
import { store } from '../store.js'

const { ref, computed, watch } = Vue
const { useRouter, useRoute } = VueRouter

export default {
  name: 'LayoutView',
  setup() {
    const router = useRouter()
    const route = useRoute()
    const collapsed = ref(false)

    // 移动端：侧边栏改为抽屉式（默认隐藏，点击顶栏按钮滑出，选中菜单/点遮罩后收起）
    const mobileMenuOpen = ref(false)
    const toggleMenu = () => {
      if (store.isMobile) mobileMenuOpen.value = !mobileMenuOpen.value
      else collapsed.value = !collapsed.value
    }
    watch(() => route.path, () => { mobileMenuOpen.value = false })
    watch(() => store.isMobile, (m) => { if (!m) mobileMenuOpen.value = false })

    // 菜单定义：perm 为所需权限码，无 perm 表示登录即可见
    const menus = computed(() => [
      { path: '/', title: '仪表盘', icon: 'Odometer' },
      { path: '/streams', title: '流管理', icon: 'VideoCamera', perm: 'stream:list' },
      { path: '/live-pages', title: '直播管理', icon: 'Monitor', perm: 'livepage:list' },
      { path: '/proxies', title: '拉流代理', icon: 'Download', perm: 'proxy:list' },
      { path: '/forwards', title: '转推流', icon: 'Upload', perm: 'forward:list' },
      { path: '/studio', title: '制播管理', icon: 'VideoPlay', perm: 'studio:list' },
      { path: '/domains', title: '域名配置', icon: 'Link', perm: 'domain:list' },
      { path: '/play-setting', title: '播放设置', icon: 'Setting', perm: 'format:list' },
      { path: '/tokens', title: '播放令牌', icon: 'Key', perm: 'token:list' },
      { path: '/encrypts', title: 'HLS 加密', icon: 'Lock', perm: 'encrypt:list' },
      { path: '/users', title: '用户管理', icon: 'User', perm: 'user:list' },
      { path: '/roles', title: '角色权限', icon: 'UserFilled', perm: 'role:list' },
      { path: '/logs', title: '日志审计', icon: 'Document', perm: 'log:list' },
    ].filter(m => !m.perm || store.has(m.perm)))

    const activeMenu = computed(() => route.path)
    const pageTitle = computed(() => route.meta.title || '')

    // ---- 退出登录 ----
    const logout = async () => {
      try { await api.logout() } catch (e) { /* 忽略 */ }
      setToken('')
      store.setUser(null)
      router.push('/login')
    }

    // ---- 修改密码 ----
    const pwdVisible = ref(false)
    const pwdForm = ref({ old_password: '', new_password: '', confirm: '' })
    const pwdLoading = ref(false)
    const pwdFormRef = ref(null)
    const pwdRules = {
      old_password: [{ required: true, message: '请输入旧密码', trigger: 'blur' }],
      new_password: [
        { required: true, message: '请输入新密码', trigger: 'blur' },
        { min: 6, message: '新密码长度不能少于 6 位', trigger: 'blur' },
      ],
      confirm: [
        { required: true, message: '请再次输入新密码', trigger: 'blur' },
        {
          validator: (r, v, cb) => cb(v === pwdForm.value.new_password ? undefined : new Error('两次输入的密码不一致')),
          trigger: 'blur',
        },
      ],
    }
    const submitPwd = async () => {
      await pwdFormRef.value.validate().catch(() => Promise.reject())
      pwdLoading.value = true
      try {
        await api.changePassword(pwdForm.value.old_password, pwdForm.value.new_password)
        ElementPlus.ElMessage.success('密码修改成功，请重新登录')
        pwdVisible.value = false
        setToken('')
        store.setUser(null)
        router.push('/login')
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        pwdLoading.value = false
      }
    }

    const onUserCommand = (cmd) => {
      if (cmd === 'pwd') {
        pwdForm.value = { old_password: '', new_password: '', confirm: '' }
        pwdVisible.value = true
      } else if (cmd === 'logout') {
        ElementPlus.ElMessageBox.confirm('确定退出登录吗？', '提示', { type: 'warning' })
          .then(logout)
          .catch(() => {})
      }
    }

    return {
      store, collapsed, menus, activeMenu, pageTitle,
      mobileMenuOpen, toggleMenu,
      logout, onUserCommand,
      pwdVisible, pwdForm, pwdLoading, pwdFormRef, pwdRules, submitPwd,
    }
  },
  template: `
    <el-container class="layout" :class="{ 'is-mobile': store.isMobile }">
      <!-- 移动端抽屉遮罩 -->
      <div v-if="store.isMobile && mobileMenuOpen" class="layout-mask" @click="mobileMenuOpen = false"></div>
      <el-aside :width="store.isMobile ? '220px' : (collapsed ? '64px' : '220px')"
                class="layout-aside" :class="{ 'is-open': mobileMenuOpen }">
        <div class="layout-logo">
          <span class="logo-dot"></span>
          <span v-show="store.isMobile || !collapsed">流媒体管理系统</span>
        </div>
        <el-menu :default-active="activeMenu" :collapse="!store.isMobile && collapsed" :collapse-transition="false"
                 background-color="#001529" text-color="#a6adb4" active-text-color="#ffffff" router>
          <el-menu-item v-for="m in menus" :key="m.path" :index="m.path">
            <el-icon><component :is="m.icon" /></el-icon>
            <template #title>{{ m.title }}</template>
          </el-menu-item>
        </el-menu>
      </el-aside>

      <el-container>
        <el-header class="layout-header">
          <el-icon class="fold-btn" @click="toggleMenu">
            <Menu v-if="store.isMobile" /><Fold v-else-if="!collapsed" /><Expand v-else />
          </el-icon>
          <el-breadcrumb class="breadcrumb" separator="/">
            <el-breadcrumb-item class="hide-xs">首页</el-breadcrumb-item>
            <el-breadcrumb-item v-if="pageTitle">{{ pageTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
          <el-dropdown @command="onUserCommand">
            <span class="user-name">
              <el-icon style="vertical-align:-2px"><User /></el-icon>
              <span class="hide-xs">{{ store.user?.nickname || store.user?.username }}</span>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="pwd">修改密码</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </el-header>

        <el-main class="layout-main">
          <router-view />
        </el-main>
      </el-container>

      <el-dialog v-model="pwdVisible" title="修改密码" width="420px" append-to-body>
        <el-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-width="90px">
          <el-form-item label="旧密码" prop="old_password">
            <el-input v-model="pwdForm.old_password" type="password" show-password />
          </el-form-item>
          <el-form-item label="新密码" prop="new_password">
            <el-input v-model="pwdForm.new_password" type="password" show-password />
          </el-form-item>
          <el-form-item label="确认密码" prop="confirm">
            <el-input v-model="pwdForm.confirm" type="password" show-password />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="pwdVisible = false">取消</el-button>
          <el-button type="primary" :loading="pwdLoading" @click="submitPwd">确定</el-button>
        </template>
      </el-dialog>
    </el-container>
  `,
}
