// 登录页

import { api, setToken } from '../api.js'
import { store } from '../store.js'

const { ref } = Vue
const { useRouter } = VueRouter

export default {
  name: 'LoginView',
  setup() {
    const router = useRouter()
    const form = ref({ username: '', password: '' })
    const loading = ref(false)
    const formRef = ref(null)

    const rules = {
      username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
      password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
    }

    const submit = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      loading.value = true
      try {
        const data = await api.login(form.value.username.trim(), form.value.password)
        setToken(data.token)
        store.setUser(data.user)
        ElementPlus.ElMessage.success('登录成功')
        router.push('/')
      } catch (e) {
        ElementPlus.ElMessage.error(e.message || '登录失败')
      } finally {
        loading.value = false
      }
    }

    return { form, loading, formRef, rules, submit }
  },
  template: `
    <div class="login-page">
      <div class="login-card">
        <h1 class="login-title">流媒体管理系统</h1>
        <p class="login-subtitle">Stream Management System</p>
        <el-form ref="formRef" :model="form" :rules="rules" size="large" @submit.prevent="submit">
          <el-form-item prop="username">
            <el-input v-model="form.username" placeholder="用户名" prefix-icon="User" autocomplete="username" />
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="form.password" type="password" placeholder="密码" prefix-icon="Lock"
                      show-password autocomplete="current-password" @keyup.enter="submit" />
          </el-form-item>
          <el-form-item style="margin-bottom:0">
            <el-button type="primary" style="width:100%" :loading="loading" @click="submit">登 录</el-button>
          </el-form-item>
        </el-form>
      </div>
    </div>
  `,
}
