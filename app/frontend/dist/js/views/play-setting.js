// 播放设置：格式开关 + 鉴权策略（同一页面两个 Tab）

import { api } from '../api.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'PlaySettingView',
  setup() {
    const tab = ref('formats')

    // ---------- 格式开关 ----------
    const formatsLoading = ref(false)
    const formats = ref([])
    const loadFormats = async () => {
      formatsLoading.value = true
      try {
        formats.value = await api.listFormats() || []
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        formatsLoading.value = false
      }
    }
    const toggleFormat = async (row, val) => {
      try {
        await api.toggleFormat(row.format, val)
        ElementPlus.ElMessage.success(val ? `已开启 ${row.name}` : `已关闭 ${row.name}`)
        row.enabled = val ? 1 : 0
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }

    // ---------- 鉴权策略 ----------
    const policyLoading = ref(false)
    const policySaving = ref(false)
    const policy = reactive({
      enabled: true, token_expire: 3600, ip_bind_enabled: false,
      param_name: 'token', allow_empty: false, allow_anonymous: false,
    })
    const loadPolicy = async () => {
      policyLoading.value = true
      try {
        const d = await api.getPolicy()
        if (d) Object.assign(policy, d)
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        policyLoading.value = false
      }
    }
    const savePolicy = async () => {
      policySaving.value = true
      try {
        await api.setPolicy({ ...policy })
        ElementPlus.ElMessage.success('策略已保存并实时生效')
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        policySaving.value = false
      }
    }

    onMounted(() => {
      loadFormats()
      loadPolicy()
    })

    return {
      store, tab,
      formatsLoading, formats, loadFormats, toggleFormat,
      policyLoading, policySaving, policy, savePolicy,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <el-tabs v-model="tab">
        <el-tab-pane label="格式开关" name="formats">
          <el-alert type="info" :closable="false" style="margin-bottom:16px"
                    title="关闭某个格式后，该格式的播放地址将不再对外分发" />
          <el-table :data="formats" v-loading="formatsLoading" size="small" border style="max-width:900px">
            <el-table-column prop="format" label="格式标识" width="140">
              <template #default="{ row }"><span class="mono">{{ row.format }}</span></template>
            </el-table-column>
            <el-table-column prop="name" label="显示名称" width="160" />
            <el-table-column prop="path_prefix" label="路径前缀" width="140">
              <template #default="{ row }"><span class="mono">{{ row.path_prefix }}</span></template>
            </el-table-column>
            <el-table-column prop="remark" label="备注" min-width="180" show-overflow-tooltip />
            <el-table-column label="对外分发" width="110" align="center">
              <template #default="{ row }">
                <el-switch :model-value="row.enabled === 1" :disabled="!store.has('format:update')"
                           @change="v => toggleFormat(row, v)" />
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="播放鉴权策略" name="policy">
          <el-form v-loading="policyLoading" label-width="170px" style="max-width:640px">
            <el-form-item label="开启播放鉴权">
              <el-switch v-model="policy.enabled" :disabled="!store.has('stream:update')" />
            </el-form-item>
            <el-form-item label="令牌默认有效期(秒)">
              <el-input-number controls-position="right" v-model="policy.token_expire" :min="60" :step="300" style="width:180px"
                               :disabled="!store.has('stream:update')" />
            </el-form-item>
            <el-form-item label="令牌参数名">
              <el-input v-model="policy.param_name" style="width:180px" class="mono"
                        :disabled="!store.has('stream:update')" />
              <div class="text-gray" style="font-size:12px">播放地址中携带令牌的查询参数名</div>
            </el-form-item>
            <el-form-item label="允许 IP 绑定">
              <el-switch v-model="policy.ip_bind_enabled" :disabled="!store.has('stream:update')" />
              <div class="text-gray" style="font-size:12px">开启后生成令牌时可绑定播放端 IP</div>
            </el-form-item>
            <el-form-item label="允许无令牌播放">
              <el-switch v-model="policy.allow_empty" :disabled="!store.has('stream:update')" />
              <div class="text-danger" style="font-size:12px">开启后任何人都可以播放，请谨慎</div>
            </el-form-item>
            <el-form-item label="允许匿名访问">
              <el-switch v-model="policy.allow_anonymous" :disabled="!store.has('stream:update')" />
            </el-form-item>
            <el-form-item v-if="store.has('stream:update')">
              <el-button type="primary" :loading="policySaving" @click="savePolicy">保存策略</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  `,
}
