// 用户管理

import { api } from '../api.js'
import { fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'UsersView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20, keyword: '' })
    const roleOptions = ref([])

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listUsers({ ...query })
        list.value = d.list || []
        total.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    const search = () => { query.page = 1; load() }

    const loadRoles = async () => {
      try { roleOptions.value = await api.listRoles() || [] } catch (e) { /* ignore */ }
    }
    onMounted(() => { load(); loadRoles() })

    // ---------- 新增/编辑 ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const blank = {
      id: 0, username: '', password: '', nickname: '', email: '', phone: '',
      status: 1, is_super: 0, role_ids: [], remark: '',
    }
    const form = reactive({ ...blank })
    const rules = {
      username: [
        { required: true, message: '请输入用户名', trigger: 'blur' },
        { min: 3, max: 32, message: '用户名长度需在 3-32 个字符之间', trigger: 'blur' },
      ],
      password: [
        { required: true, message: '请输入密码', trigger: 'blur' },
        { min: 6, message: '密码长度不能少于 6 位', trigger: 'blur' },
      ],
    }

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, blank)
      dlgVisible.value = true
    }
    const openEdit = (row) => {
      dlgMode.value = 'edit'
      Object.assign(form, {
        ...blank, ...row,
        password: '',
        role_ids: (row.roles || []).map(r => r.id),
      })
      dlgVisible.value = true
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      dlgLoading.value = true
      try {
        if (dlgMode.value === 'create') {
          await api.createUser({
            username: form.username, password: form.password, nickname: form.nickname,
            email: form.email, phone: form.phone, status: form.status,
            is_super: form.is_super, role_ids: form.role_ids, remark: form.remark,
          })
          ElementPlus.ElMessage.success('创建成功')
        } else {
          await api.updateUser(form.id, {
            nickname: form.nickname, email: form.email, phone: form.phone,
            status: form.status, is_super: form.is_super, role_ids: form.role_ids, remark: form.remark,
          })
          ElementPlus.ElMessage.success('更新成功')
        }
        dlgVisible.value = false
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        dlgLoading.value = false
      }
    }

    // ---------- 重置密码 ----------
    const resetPwd = (row) => {
      ElementPlus.ElMessageBox.prompt(`为用户「${row.username}」设置新密码`, '重置密码', {
        inputPattern: /^.{6,}$/, inputErrorMessage: '密码长度不能少于 6 位',
        inputType: 'password', confirmButtonText: '重置', cancelButtonText: '取消',
      }).then(async ({ value }) => {
        try {
          await api.resetUserPassword(row.id, value)
          ElementPlus.ElMessage.success('密码已重置')
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除用户「${row.username}」吗？`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteUser(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    return {
      store, loading, list, total, query, load, search, fmtTime, roleOptions,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm,
      resetPwd, remove,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <el-input v-model="query.keyword" placeholder="用户名 / 昵称" clearable style="width:220px"
                  @keyup.enter="search" @clear="search" />
        <el-button type="primary" icon="Search" @click="search">查询</el-button>
        <div class="spacer"></div>
        <el-button v-if="store.has('user:create')" type="primary" icon="Plus" @click="openCreate">新增用户</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="username" label="用户名" min-width="110" />
        <el-table-column prop="nickname" label="昵称" min-width="110" />
        <el-table-column label="角色" min-width="150">
          <template #default="{ row }">
            <el-tag v-if="row.is_super === 1" type="danger" size="small" style="margin-right:4px">超管</el-tag>
            <el-tag v-for="r in (row.roles || [])" :key="r.id" size="small" style="margin-right:4px">{{ r.name }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最后登录" min-width="150">
          <template #default="{ row }">{{ fmtTime(row.last_login_at) }}</template>
        </el-table-column>
        <el-table-column prop="last_login_ip" label="登录IP" width="130">
          <template #default="{ row }"><span class="mono">{{ row.last_login_ip || '-' }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="store.has('user:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="store.has('user:resetpwd')" text type="warning" size="small" @click="resetPwd(row)">重置密码</el-button>
            <el-button v-if="store.has('user:delete')" text type="danger" size="small"
                       :disabled="row.id === store.user?.id" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                       v-model:current-page="query.page" v-model:page-size="query.page_size"
                       :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="search" />
      </div>

      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增用户' : '编辑用户'" width="520px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
          <el-form-item label="用户名" prop="username">
            <el-input v-model="form.username" :disabled="dlgMode === 'edit'" autocomplete="off" />
          </el-form-item>
          <el-form-item v-if="dlgMode === 'create'" label="密码" prop="password">
            <el-input v-model="form.password" type="password" show-password autocomplete="new-password" />
          </el-form-item>
          <el-form-item label="昵称">
            <el-input v-model="form.nickname" />
          </el-form-item>
          <el-form-item label="邮箱">
            <el-input v-model="form.email" />
          </el-form-item>
          <el-form-item label="手机号">
            <el-input v-model="form.phone" />
          </el-form-item>
          <el-form-item label="角色">
            <el-select v-model="form.role_ids" multiple style="width:100%" placeholder="选择角色">
              <el-option v-for="r in roleOptions" :key="r.id" :label="r.name" :value="r.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="超级管理员">
            <el-switch :model-value="form.is_super === 1" @change="v => form.is_super = v ? 1 : 0" />
            <div class="text-danger" style="font-size:12px">超管拥有全部权限，无需分配角色</div>
          </el-form-item>
          <el-form-item label="状态">
            <el-radio-group v-model="form.status">
              <el-radio :value="1">启用</el-radio>
              <el-radio :value="0">禁用</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="form.remark" type="textarea" :rows="2" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">确定</el-button>
        </template>
      </el-dialog>
    </el-card>
  `,
}
