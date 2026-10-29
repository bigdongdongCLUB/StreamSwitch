// HLS 加密管理

import { api } from '../api.js'
import { fmtTime, copyText } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'EncryptsView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const streams = ref([])

    const streamName = (sid) => {
      const s = streams.value.find(x => x.id === sid)
      return s ? `${s.name} (${s.app}/${s.stream})` : `#${sid}`
    }

    const load = async () => {
      loading.value = true
      try {
        const [encs, sts] = await Promise.all([
          api.listEncrypts(),
          api.listStreams({ page: 1, page_size: 200 }).catch(() => ({ list: [] })),
        ])
        list.value = encs || []
        streams.value = sts.list || []
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    onMounted(load)

    // ---------- 新增/编辑 ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const blank = { stream_id: 0, enabled: true, key_expire_sec: 0, iv_strategy: 'seq', rotate_key: false }
    const form = reactive({ ...blank })

    // 可新增的流 = 未配置过加密的流
    const availableStreams = () => streams.value.filter(s => !list.value.some(e => e.stream_id === s.id))

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, blank)
      dlgVisible.value = true
    }
    const openEdit = (row) => {
      dlgMode.value = 'edit'
      Object.assign(form, {
        stream_id: row.stream_id,
        enabled: row.enabled === 1,
        key_expire_sec: row.key_expire_sec,
        iv_strategy: row.iv_strategy,
        rotate_key: false,
      })
      dlgVisible.value = true
    }
    const submitForm = async () => {
      if (!form.stream_id) {
        ElementPlus.ElMessage.warning('请选择流')
        return
      }
      dlgLoading.value = true
      try {
        await api.setEncrypt(form.stream_id, {
          enabled: form.enabled,
          key_expire_sec: form.key_expire_sec,
          iv_strategy: form.iv_strategy,
          rotate_key: form.rotate_key,
        })
        ElementPlus.ElMessage.success('加密配置已保存')
        dlgVisible.value = false
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        dlgLoading.value = false
      }
    }

    const rotate = (row) => {
      ElementPlus.ElMessageBox.confirm('轮换后旧密钥将失效，正在播放的用户可能需要刷新。确定轮换密钥吗？', '提示', { type: 'warning' })
        .then(async () => {
          try {
            await api.rotateEncrypt(row.stream_id)
            ElementPlus.ElMessage.success('密钥已轮换')
            load()
          } catch (e) {
            ElementPlus.ElMessage.error(e.message)
          }
        }).catch(() => {})
    }
    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除「${streamName(row.stream_id)}」的加密配置吗？删除后 HLS 将明文分发。`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteEncrypt(row.stream_id)
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

    // ---------- 密钥查看 ----------
    const keyVisible = ref(false)
    const keyInfo = ref(null)
    const showKey = (row) => { keyInfo.value = row; keyVisible.value = true }

    return {
      store, loading, list, streams, streamName, availableStreams, load, fmtTime,
      dlgVisible, dlgMode, dlgLoading, form, openCreate, openEdit, submitForm,
      rotate, remove, copy, keyVisible, keyInfo, showKey,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <span class="text-gray">为流开启 HLS AES-128 加密，加密后 m3u8 切片通过本系统代理分发并自动解密鉴权</span>
        <div class="spacer"></div>
        <el-button icon="Refresh" @click="load">刷新</el-button>
        <el-button v-if="store.has('encrypt:update')" type="primary" icon="Plus" @click="openCreate">新增加密配置</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="stream_id" label="流ID" width="70" />
        <el-table-column label="流" min-width="180">
          <template #default="{ row }">{{ streamName(row.stream_id) }}</template>
        </el-table-column>
        <el-table-column label="加密状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled === 1 ? 'success' : 'info'" size="small">{{ row.enabled === 1 ? '已开启' : '已关闭' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="IV 策略" width="100" align="center">
          <template #default="{ row }"><span class="mono">{{ row.iv_strategy }}</span></template>
        </el-table-column>
        <el-table-column label="密钥有效期" width="110" align="center">
          <template #default="{ row }">{{ row.key_expire_sec > 0 ? row.key_expire_sec + ' 秒' : '永久' }}</template>
        </el-table-column>
        <el-table-column label="密钥过期时间" min-width="150">
          <template #default="{ row }">
            <span :class="{ 'text-danger': row.expire_at > 0 && row.expire_at * 1000 < Date.now() }">
              {{ row.expire_at ? fmtTime(row.expire_at) : '-' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="上次轮换" min-width="150">
          <template #default="{ row }">{{ fmtTime(row.last_rotate_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="250" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button text type="primary" size="small" @click="showKey(row)">查看密钥</el-button>
            <template v-if="store.has('encrypt:update')">
              <el-button text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
              <el-button text type="warning" size="small" @click="rotate(row)">轮换密钥</el-button>
              <el-button text type="danger" size="small" @click="remove(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>

      <!-- 新增/编辑 -->
      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增加密配置' : '编辑加密配置'" width="480px">
        <el-form :model="form" label-width="120px">
          <el-form-item label="选择流" v-if="dlgMode === 'create'">
            <el-select v-model="form.stream_id" style="width:100%" filterable placeholder="选择未配置加密的流">
              <el-option v-for="s in availableStreams()" :key="s.id"
                         :label="s.name + ' (' + s.app + '/' + s.stream + ')'" :value="s.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="流" v-else>
            <span>{{ streamName(form.stream_id) }}</span>
          </el-form-item>
          <el-form-item label="开启加密">
            <el-switch v-model="form.enabled" />
          </el-form-item>
          <el-form-item label="密钥有效期(秒)">
            <el-input-number controls-position="right" v-model="form.key_expire_sec" :min="0" :step="3600" style="width:180px" />
            <div class="text-gray" style="font-size:12px">0 表示永久有效；到期后需轮换密钥</div>
          </el-form-item>
          <el-form-item label="IV 策略">
            <el-radio-group v-model="form.iv_strategy">
              <el-radio value="seq">seq（媒体序号，符合 RFC8216）</el-radio>
              <el-radio value="random">random（随机 IV）</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="dlgMode === 'edit'" label="重新生成密钥">
            <el-switch v-model="form.rotate_key" />
            <div class="text-danger" style="font-size:12px">开启后保存时将强制轮换密钥</div>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">保存</el-button>
        </template>
      </el-dialog>

      <!-- 密钥查看 -->
      <el-dialog v-model="keyVisible" title="密钥信息" width="560px">
        <template v-if="keyInfo">
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="流">{{ streamName(keyInfo.stream_id) }}</el-descriptions-item>
            <el-descriptions-item label="AES-128 密钥 (HEX)">
              <div style="display:flex;align-items:center;gap:8px">
                <span class="mono">{{ keyInfo.key_hex }}</span>
                <el-button size="small" icon="CopyDocument" @click="copy(keyInfo.key_hex)" />
              </div>
            </el-descriptions-item>
            <el-descriptions-item label="密钥下发路径">
              <span class="mono">{{ keyInfo.key_uri }}</span>
            </el-descriptions-item>
          </el-descriptions>
          <el-alert type="warning" :closable="false" style="margin-top:12px"
                    title="密钥属于敏感信息，请勿泄露给无关人员" />
        </template>
      </el-dialog>
    </el-card>
  `,
}
