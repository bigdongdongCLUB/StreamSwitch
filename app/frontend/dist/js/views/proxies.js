// 拉流代理任务管理

import { api } from '../api.js'
import { fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

const runStatusTag = (s) => ({ running: 'success', failed: 'danger', stopped: 'info', idle: 'info' }[s] || 'info')
const runStatusText = (s) => ({ running: '运行中', failed: '失败', stopped: '已停止', idle: '空闲' }[s] || s)

export default {
  name: 'ProxiesView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20, keyword: '', status: '' })

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listProxies({ ...query })
        list.value = d.list || []
        total.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    const search = () => { query.page = 1; load() }
    onMounted(load)

    // ---------- 新增/编辑 ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const blank = {
      id: 0, name: '', src_url: '', app: 'live', stream: '', vhost: '__defaultVhost__',
      status: 1, retry_count: -1, retry_interval: 10, timeout_sec: 10,
      enable_rtsp: 1, enable_rtmp: 1, enable_ts: 0, enable_fmp4: 0, enable_hls: 1,
    }
    const form = reactive({ ...blank })
    const rules = {
      name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
      src_url: [{ required: true, message: '请输入远端源地址', trigger: 'blur' }],
      stream: [
        { required: true, message: '请输入本地流ID', trigger: 'blur' },
        { pattern: /^[A-Za-z0-9_-]+$/, message: '流ID只允许字母、数字、下划线、中划线', trigger: 'blur' },
      ],
    }

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, blank)
      dlgVisible.value = true
    }
    const openEdit = (row) => {
      dlgMode.value = 'edit'
      Object.assign(form, { ...blank, ...row })
      dlgVisible.value = true
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      dlgLoading.value = true
      const body = {
        name: form.name, src_url: form.src_url, app: form.app, stream: form.stream, vhost: form.vhost,
        status: form.status, retry_count: form.retry_count, retry_interval: form.retry_interval,
        timeout_sec: form.timeout_sec, enable_rtsp: form.enable_rtsp, enable_rtmp: form.enable_rtmp,
        enable_ts: form.enable_ts, enable_fmp4: form.enable_fmp4, enable_hls: form.enable_hls,
      }
      try {
        if (dlgMode.value === 'create') {
          await api.createProxy(body)
          ElementPlus.ElMessage.success('创建成功')
        } else {
          await api.updateProxy(form.id, body)
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

    // ---------- 操作 ----------
    const act = async (fn, row, okMsg) => {
      try {
        await fn(row.id)
        ElementPlus.ElMessage.success(okMsg)
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const toggle = (row) => act(api.toggleProxy, row, row.status === 1 ? '任务已停止' : '任务已启动')
    const start = (row) => act(api.startProxy, row, '任务已启动')
    const stop = (row) => act(api.stopProxy, row, '任务已停止')
    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除拉流任务「${row.name}」吗？`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(() => act(api.deleteProxy, row, '删除成功')).catch(() => {})
    }

    // ---------- 错误详情 ----------
    const errVisible = ref(false)
    const errText = ref('')
    const showError = (row) => { errText.value = row.last_error || '无'; errVisible.value = true }

    return {
      store, loading, list, total, query, load, search, fmtTime, runStatusTag, runStatusText,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm,
      toggle, start, stop, remove, errVisible, errText, showError,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <el-input v-model="query.keyword" placeholder="名称 / 流ID / 源地址" clearable style="width:240px"
                  @keyup.enter="search" @clear="search" />
        <el-select v-model="query.status" placeholder="状态" clearable style="width:120px" @change="search">
          <el-option label="启用" :value="1" />
          <el-option label="禁用" :value="0" />
        </el-select>
        <el-button type="primary" icon="Search" @click="search">查询</el-button>
        <div class="spacer"></div>
        <el-button icon="Refresh" @click="load">刷新</el-button>
        <el-button v-if="store.has('proxy:create')" type="primary" icon="Plus" @click="openCreate">新增代理</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="任务名称" min-width="110" show-overflow-tooltip />
        <el-table-column prop="src_url" label="远端源地址" min-width="220" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono">{{ row.src_url }}</span></template>
        </el-table-column>
        <el-table-column label="本地流" min-width="130">
          <template #default="{ row }"><span class="mono">{{ row.app }}/{{ row.stream }}</span></template>
        </el-table-column>
        <el-table-column label="运行状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="runStatusTag(row.run_status)" size="small">{{ runStatusText(row.run_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="重试" width="90" align="center">
          <template #default="{ row }">
            <span :class="{ 'text-danger': row.retry_times > 0 }">{{ row.retry_times }}</span>
            <span class="text-gray">/{{ row.retry_count < 0 ? '∞' : row.retry_count }}</span>
          </template>
        </el-table-column>
        <el-table-column label="最后活跃" min-width="150">
          <template #default="{ row }">{{ fmtTime(row.last_active_at) }}</template>
        </el-table-column>
        <el-table-column label="启用" width="70" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.status === 1" :disabled="!store.has('proxy:toggle')" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <template v-if="store.has('proxy:toggle')">
              <el-button v-if="row.run_status !== 'running'" text type="success" size="small" @click="start(row)">启动</el-button>
              <el-button v-else text type="warning" size="small" @click="stop(row)">停止</el-button>
            </template>
            <el-button v-if="row.last_error" text type="danger" size="small" @click="showError(row)">错误</el-button>
            <el-button v-if="store.has('proxy:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="store.has('proxy:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                       v-model:current-page="query.page" v-model:page-size="query.page_size"
                       :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="search" />
      </div>

      <!-- 新增/编辑 -->
      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增拉流代理' : '编辑拉流代理'" width="620px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
          <el-form-item label="任务名称" prop="name">
            <el-input v-model="form.name" placeholder="如：CCTV1 转播" />
          </el-form-item>
          <el-form-item label="远端源地址" prop="src_url">
            <el-input v-model="form.src_url" placeholder="rtmp:// / http(s)://...flv / ...m3u8" class="mono" />
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="12" :xs="24">
              <el-form-item label="本地 App">
                <el-input v-model="form.app" :disabled="dlgMode === 'edit'" />
              </el-form-item>
            </el-col>
            <el-col :span="12" :xs="24">
              <el-form-item label="本地流ID" prop="stream">
                <el-input v-model="form.stream" :disabled="dlgMode === 'edit'" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-form-item label="协议转换">
            <el-checkbox :model-value="form.enable_rtsp === 1" @change="v => form.enable_rtsp = v ? 1 : 0">RTSP</el-checkbox>
            <el-checkbox :model-value="form.enable_rtmp === 1" @change="v => form.enable_rtmp = v ? 1 : 0">RTMP/FLV</el-checkbox>
            <el-checkbox :model-value="form.enable_ts === 1" @change="v => form.enable_ts = v ? 1 : 0">HTTP-TS</el-checkbox>
            <el-checkbox :model-value="form.enable_fmp4 === 1" @change="v => form.enable_fmp4 = v ? 1 : 0">FMP4</el-checkbox>
            <el-checkbox :model-value="form.enable_hls === 1" @change="v => form.enable_hls = v ? 1 : 0">HLS</el-checkbox>
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="8" :xs="24">
              <el-form-item label="重试次数" label-width="70px">
                <el-input-number controls-position="right" v-model="form.retry_count" :min="-1" style="width:100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8" :xs="24">
              <el-form-item label="重试间隔(秒)" label-width="100px">
                <el-input-number controls-position="right" v-model="form.retry_interval" :min="1" style="width:100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8" :xs="24">
              <el-form-item label="超时(秒)" label-width="80px">
                <el-input-number controls-position="right" v-model="form.timeout_sec" :min="1" style="width:100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-form-item label="启用任务">
            <el-switch :model-value="form.status === 1" @change="v => form.status = v ? 1 : 0" />
            <span class="text-gray" style="margin-left:8px;font-size:12px">-1 表示无限重试</span>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">确定</el-button>
        </template>
      </el-dialog>

      <!-- 错误详情 -->
      <el-dialog v-model="errVisible" title="最近一次错误" width="560px">
        <el-alert type="error" :closable="false">
          <pre class="mono" style="white-space:pre-wrap;margin:0">{{ errText }}</pre>
        </el-alert>
      </el-dialog>
    </el-card>
  `,
}
