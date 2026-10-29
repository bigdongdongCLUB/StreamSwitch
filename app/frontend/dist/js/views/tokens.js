// 播放令牌管理

import { api } from '../api.js'
import { fmtTime, copyText } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'TokensView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20 })

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listTokens({ ...query })
        list.value = d.list || []
        total.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    onMounted(load)

    // ---------- 生成令牌 ----------
    const dlgVisible = ref(false)
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const streamOptions = ref([])
    const form = reactive({ stream_id: 0, expire_sec: 3600, ip_bind: '', format: '' })

    const openCreate = async () => {
      Object.assign(form, { stream_id: 0, expire_sec: 3600, ip_bind: '', format: '' })
      dlgVisible.value = true
      try {
        const d = await api.listStreams({ page: 1, page_size: 200 })
        streamOptions.value = d.list || []
      } catch (e) { /* 无权限时忽略 */ }
    }
    const submitForm = async () => {
      dlgLoading.value = true
      try {
        const t = await api.createToken({ ...form })
        dlgVisible.value = false
        load()
        const ok = await copyText(t.token)
        ElementPlus.ElMessageBox.alert(
          `<div class="mono" style="word-break:break-all;font-size:13px">${t.token}</div>`,
          '令牌已生成' + (ok ? '（已复制到剪贴板）' : ''),
          { dangerouslyUseHTMLString: true, confirmButtonText: '知道了' }
        )
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        dlgLoading.value = false
      }
    }

    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm('确定删除该令牌吗？删除后使用该令牌的播放将立即失效。', '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteToken(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    const copy = async (text) => {
      const ok = await copyText(text)
      ElementPlus.ElMessage[ok ? 'success' : 'error'](ok ? '已复制' : '复制失败')
    }

    const formatNames = { rtmp: 'RTMP', 'http-flv': 'HTTP-FLV', 'ws-flv': 'WS-FLV', hls: 'HLS' }
    const isExpired = (row) => row.expire_at > 0 && row.expire_at * 1000 < Date.now()

    return {
      store, loading, list, total, query, load, fmtTime,
      dlgVisible, dlgLoading, formRef, form, streamOptions, openCreate, submitForm,
      remove, copy, formatNames, isExpired,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <span class="text-gray">令牌用于播放鉴权，可限定流、格式、有效期与绑定 IP</span>
        <div class="spacer"></div>
        <el-button icon="Refresh" @click="load">刷新</el-button>
        <el-button v-if="store.has('token:create')" type="primary" icon="Plus" @click="openCreate">生成令牌</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column label="令牌" min-width="220">
          <template #default="{ row }">
            <span class="mono">{{ row.token.slice(0, 20) }}…</span>
            <el-button text type="primary" size="small" icon="CopyDocument" @click="copy(row.token)" />
          </template>
        </el-table-column>
        <el-table-column label="限定流" min-width="120">
          <template #default="{ row }">
            <span v-if="row.stream_id > 0" class="mono">{{ row.app }}/{{ row.stream }}</span>
            <el-tag v-else type="warning" size="small">不限流</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="限定格式" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.format" size="small">{{ formatNames[row.format] || row.format }}</el-tag>
            <span v-else class="text-gray">全部</span>
          </template>
        </el-table-column>
        <el-table-column label="绑定IP" min-width="120" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono">{{ row.ip_bind || '-' }}</span></template>
        </el-table-column>
        <el-table-column label="过期时间" min-width="150">
          <template #default="{ row }">
            <span :class="{ 'text-danger': isExpired(row) }">{{ row.expire_at ? fmtTime(row.expire_at) : '永久' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="isExpired(row) ? 'danger' : 'success'" size="small">{{ isExpired(row) ? '已过期' : '有效' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="150">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="store.has('token:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                       v-model:current-page="query.page" v-model:page-size="query.page_size"
                       :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="() => { query.page = 1; load() }" />
      </div>

      <el-dialog v-model="dlgVisible" title="生成播放令牌" width="480px">
        <el-form ref="formRef" :model="form" label-width="110px">
          <el-form-item label="限定流">
            <el-select v-model="form.stream_id" style="width:100%" filterable>
              <el-option label="不限流（可播放所有流）" :value="0" />
              <el-option v-for="s in streamOptions" :key="s.id"
                         :label="s.name + ' (' + s.app + '/' + s.stream + ')'" :value="s.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="有效期(秒)">
            <el-input-number controls-position="right" v-model="form.expire_sec" :min="60" :step="300" style="width:180px" />
            <div class="text-gray" style="font-size:12px">默认 3600 秒（1 小时）</div>
          </el-form-item>
          <el-form-item label="限定格式">
            <el-select v-model="form.format" clearable placeholder="全部格式" style="width:180px">
              <el-option v-for="(n, f) in formatNames" :key="f" :label="n" :value="f" />
            </el-select>
          </el-form-item>
          <el-form-item label="绑定 IP">
            <el-input v-model="form.ip_bind" placeholder="多个用逗号分隔，留空不限制" class="mono" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">生成</el-button>
        </template>
      </el-dialog>
    </el-card>
  `,
}
