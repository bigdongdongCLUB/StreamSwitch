// 系统初始化：首次打开后台时注册首个用户，该用户即为超级管理员

import { api, setToken } from '../api.js'
import { store } from '../store.js'

const { ref } = Vue
const { useRouter } = VueRouter

export default {
  name: 'SetupView',
  setup() {
    const router = useRouter()
    const form = ref({ username: 'admin', nickname: '', password: '', confirm: '' })
    const loading = ref(false)
    const formRef = ref(null)

    const rules = {
      username: [
        { required: true, message: '请输入用户名', trigger: 'blur' },
        { pattern: /^[A-Za-z0-9_.@-]{3,32}$/, message: '3-32 位，只能包含字母、数字及 _ - . @', trigger: 'blur' },
      ],
      password: [
        { required: true, message: '请输入密码', trigger: 'blur' },
        { min: 8, message: '密码长度不能少于 8 位', trigger: 'blur' },
      ],
      confirm: [
        { required: true, message: '请再次输入密码', trigger: 'blur' },
        {
          validator: (r, v, cb) => cb(v === form.value.password ? undefined : new Error('两次输入的密码不一致')),
          trigger: 'blur',
        },
      ],
    }

    const submit = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      loading.value = true
      try {
        const data = await api.setup(form.value.username.trim(), form.value.password, form.value.nickname.trim())
        setToken(data.token)
        store.setUser(data.user)
        store.needSetup = false
        ElementPlus.ElMessage.success('初始化完成，欢迎使用')
        router.replace('/')
      } catch (e) {
        ElementPlus.ElMessage.error(e.message || '注册失败')
        // 可能已被他人抢先初始化：重新检查状态
        try {
          const s = await api.setupStatus()
          if (!s.need_setup) { store.needSetup = false; router.replace('/login') }
        } catch (_) { /* 忽略 */ }
      } finally {
        loading.value = false
      }
    }

    return { form, loading, formRef, rules, submit }
  },
  template: `
    <div class="login-page">
      <div class="login-card">
        <h1 class="login-title">初始化系统</h1>
        <p class="login-subtitle">首次使用，请注册超级管理员账号</p>
        <el-alert type="info" :closable="false" show-icon style="margin-bottom:18px"
                  title="第一个注册的账号即为超级管理员，拥有全部权限；之后的用户请在「用户管理」中创建。" />
        <el-form ref="formRef" :model="form" :rules="rules" size="large" @submit.prevent="submit">
          <el-form-item prop="username">
            <el-input v-model="form.username" placeholder="用户名" prefix-icon="User" autocomplete="username" />
          </el-form-item>
          <el-form-item prop="nickname">
            <el-input v-model="form.nickname" placeholder="昵称（可选，默认「超级管理员」）" prefix-icon="Postcard" />
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="form.password" type="password" placeholder="密码（至少 8 位）" prefix-icon="Lock"
                      show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item prop="confirm">
            <el-input v-model="form.confirm" type="password" placeholder="确认密码" prefix-icon="Lock"
                      show-password autocomplete="new-password" @keyup.enter="submit" />
          </el-form-item>
          <el-form-item style="margin-bottom:0">
            <el-button type="primary" style="width:100%" :loading="loading" @click="submit">注册并进入后台</el-button>
          </el-form-item>
        </el-form>
      </div>
    </div>
  `,
}
