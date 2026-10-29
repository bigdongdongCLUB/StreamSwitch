// 角色权限管理

import { api } from '../api.js'
import { fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'RolesView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const permTree = ref([])

    const load = async () => {
      loading.value = true
      try {
        list.value = await api.listRoles() || []
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    const loadPerms = async () => {
      try {
        const perms = await api.listPermissions() || []
        // 按 parent_id 组装为两级树
        const roots = perms.filter(p => p.parent_id === 0).sort((a, b) => a.sort - b.sort)
        permTree.value = roots.map(r => ({
          ...r,
          children: perms.filter(p => p.parent_id === r.id).sort((a, b) => a.sort - b.sort),
        }))
      } catch (e) { /* ignore */ }
    }
    onMounted(() => { load(); loadPerms() })

    // ---------- 新增/编辑 ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const treeRef = ref(null)
    const blank = { id: 0, name: '', code: '', remark: '', status: 1 }
    const form = reactive({ ...blank })
    const rules = {
      name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
      code: [{ required: true, message: '请输入角色标识', trigger: 'blur' }],
    }

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, blank)
      dlgVisible.value = true
      setTimeout(() => treeRef.value && treeRef.value.setCheckedKeys([]), 0)
    }
    const openEdit = async (row) => {
      dlgMode.value = 'edit'
      Object.assign(form, { ...blank, ...row })
      dlgVisible.value = true
      try {
        const ids = await api.rolePermissions(row.id) || []
        // 仅勾选叶子节点，父节点由半选状态自动表现
        const leafIds = ids.filter(id => {
          const node = permTree.value.flatMap(r => [r, ...r.children]).find(n => n.id === id)
          return node && (!node.children || node.children.length === 0)
        })
        setTimeout(() => treeRef.value && treeRef.value.setCheckedKeys(leafIds), 0)
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      const checked = treeRef.value ? treeRef.value.getCheckedKeys() : []
      const half = treeRef.value ? treeRef.value.getHalfCheckedKeys() : []
      const perm_ids = [...checked, ...half]
      dlgLoading.value = true
      const body = { name: form.name, code: form.code, remark: form.remark, status: form.status, perm_ids }
      try {
        if (dlgMode.value === 'create') {
          await api.createRole(body)
          ElementPlus.ElMessage.success('创建成功')
        } else {
          await api.updateRole(form.id, body)
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

    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除角色「${row.name}」吗？已分配该角色的用户将失去对应权限。`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteRole(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    return {
      store, loading, list, load, fmtTime, permTree,
      dlgVisible, dlgMode, dlgLoading, formRef, treeRef, form, rules,
      openCreate, openEdit, submitForm, remove,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <span class="text-gray">角色绑定权限后分配给用户；超级管理员无需分配即拥有全部权限</span>
        <div class="spacer"></div>
        <el-button v-if="store.has('role:create')" type="primary" icon="Plus" @click="openCreate">新增角色</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="角色名称" min-width="140" />
        <el-table-column prop="code" label="标识" min-width="120">
          <template #default="{ row }"><span class="mono">{{ row.code }}</span></template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="200" show-overflow-tooltip />
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="150">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="store.has('role:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="store.has('role:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增角色' : '编辑角色'" width="560px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
          <el-form-item label="角色名称" prop="name">
            <el-input v-model="form.name" placeholder="如：运营人员" />
          </el-form-item>
          <el-form-item label="角色标识" prop="code">
            <el-input v-model="form.code" :disabled="dlgMode === 'edit' && form.id === 1" placeholder="如：operator" class="mono" />
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
          <el-form-item label="权限分配">
            <div style="width:100%;max-height:300px;overflow:auto;border:1px solid #dcdfe6;border-radius:6px;padding:8px">
              <el-tree ref="treeRef" :data="permTree" show-checkbox node-key="id"
                       :props="{ label: 'name', children: 'children' }" />
            </div>
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
