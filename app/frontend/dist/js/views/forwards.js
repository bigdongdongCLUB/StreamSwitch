// 转推流任务管理

import { api } from '../api.js'
import { fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

const runStatusTag = (s) => ({ running: 'success', failed: 'danger', stopped: 'info', idle: 'info' }[s] || 'info')
const runStatusText = (s) => ({ running: '运行中', failed: '失败', stopped: '已停止', idle: '空闲' }[s] || s)

// 转推平台模板：
//   server 固定地址的平台直接预填（用户只需补 Stream Key）；
//   动态地址的平台提供获取指引（guide）；
//   spec 为该平台的规格建议（编码/分辨率/码率）。
const PLATFORMS = [
  { key: 'custom', name: '自定义', server: '', schema: 'rtmp', guide: '', spec: '' },
  {
    key: 'youtube', name: 'YouTube Live', server: 'rtmp://a.rtmp.youtube.com/live2/', schema: 'rtmp',
    guide: 'Stream Key 获取：YouTube Studio → 直播 → 直播控制面板 →「流密钥」。备用服务器 b.rtmp.youtube.com。',
    spec: 'H264 + AAC；最高 4K60 / 51 Mbps；也支持 RTMPS（443）',
  },
  {
    key: 'twitch', name: 'Twitch', server: 'rtmp://live.twitch.tv/app/', schema: 'rtmp',
    guide: 'Stream Key 获取：Twitch 创作者仪表板 → 设置 → 直播 →「Primary Stream Key」。',
    spec: 'H264 + AAC；建议 1080p60，码率 6000-8000 Kbps',
  },
  {
    key: 'facebook', name: 'Facebook Live', server: 'rtmps://live-api-s.facebook.com:443/rtmp/', schema: 'rtmps',
    guide: 'Stream Key 获取：Facebook 公共主页/小组 → 直播视频 → 流密钥（持久密钥可复用）。强制 RTMPS。',
    spec: '强制 RTMPS；H264 + AAC；建议 1080p，码率 4000 Kbps 内',
  },
  {
    key: 'instagram', name: 'Instagram', server: 'rtmps://live-upload.instagram.com:443/rtmp/', schema: 'rtmps',
    guide: 'Broadcast Key 获取：instagram.com/live/producer（需专业账号）→ 开始直播视频。强制 RTMPS。',
    spec: '强制 RTMPS；竖屏 1080x1920；H264 + AAC',
  },
  {
    key: 'douyin', name: '抖音直播', server: '', schema: 'rtmp',
    guide: '推流地址为动态生成：抖音直播伴侣 → 开播设置 → 复制「服务器地址」与「串流密钥」（或抖音开放平台 live.douyin.com 获取），将完整地址粘贴到下方目标地址。',
    spec: '竖屏 720x1280 / 1080x1920；H264 + AAC；建议码率 2000-4000 Kbps',
  },
  {
    key: 'wechat', name: '视频号直播', server: '', schema: 'rtmp',
    guide: '推流地址为动态生成：视频号助手（channels.weixin.qq.com）→ 直播管理 → 创建直播 → 复制推流地址，粘贴到下方目标地址。',
    spec: '建议 720p~1080p；H264 + AAC；码率 1500-3000 Kbps',
  },
  {
    key: 'bilibili', name: 'B站直播', server: 'rtmp://live-push.bilivideo.com/live-bvc/', schema: 'rtmp',
    guide: 'Stream Key 获取：直播中心 → 我的直播间 → 开播设置 → 复制服务器地址与串流密钥。',
    spec: 'H264 + AAC；1080p 建议码率 6000 Kbps 内',
  },
]

export default {
  name: 'ForwardsView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20, keyword: '', status: '' })

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listForwards({ ...query })
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
      id: 0, name: '', src_app: 'live', src_stream: '', src_vhost: '__defaultVhost__',
      dst_url: '', schema: 'rtmp', status: 1, retry_count: -1, timeout_sec: 10,
    }
    const form = reactive({ ...blank })
    const platform = ref('custom')

    // 选择平台后预填：固定地址平台直接填服务器前缀，动态地址平台仅给指引
    const currentPlatform = () => PLATFORMS.find(p => p.key === platform.value) || PLATFORMS[0]
    const onPlatformChange = () => {
      const p = currentPlatform()
      if (dlgMode.value !== 'create') return
      form.schema = p.schema
      if (p.server) {
        // 已有内容且不属于本平台服务器前缀时，保留用户已填的 key 部分
        const old = form.dst_url || ''
        let keyPart = ''
        for (const tpl of PLATFORMS) {
          if (tpl.server && old.startsWith(tpl.server)) { keyPart = old.slice(tpl.server.length); break }
        }
        form.dst_url = p.server + keyPart
      }
      if (!form.name && p.key !== 'custom') {
        form.name = '转推 ' + p.name
      }
    }
    const rules = {
      name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
      src_stream: [
        { required: true, message: '请输入源流ID', trigger: 'blur' },
        { pattern: /^[A-Za-z0-9_-]+$/, message: '流ID只允许字母、数字、下划线、中划线', trigger: 'blur' },
      ],
      dst_url: [{ required: true, message: '请输入 RTMP 目标地址', trigger: 'blur' }],
    }

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, blank)
      platform.value = 'custom'
      dlgVisible.value = true
    }
    // 根据已有 dst_url 反查平台（编辑时回显）
    const detectPlatform = (dstUrl) => {
      for (const p of PLATFORMS) {
        if (p.server && dstUrl && dstUrl.startsWith(p.server)) return p.key
      }
      return 'custom'
    }
    // 协议切换时联动替换目标地址的 schema 前缀（rtmp:// <-> rtmps://）
    const onSchemaChange = (v) => {
      if (form.dst_url && /^rtmps?:\/\//.test(form.dst_url)) {
        form.dst_url = form.dst_url.replace(/^rtmps?:\/\//, v + '://')
      }
    }
    const openEdit = (row) => {
      dlgMode.value = 'edit'
      Object.assign(form, { ...blank, ...row })
      platform.value = detectPlatform(row.dst_url)
      dlgVisible.value = true
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      dlgLoading.value = true
      const body = {
        name: form.name, src_app: form.src_app, src_stream: form.src_stream, src_vhost: form.src_vhost,
        dst_url: form.dst_url, schema: form.schema, status: form.status,
        retry_count: form.retry_count, timeout_sec: form.timeout_sec,
      }
      try {
        if (dlgMode.value === 'create') {
          await api.createForward(body)
          ElementPlus.ElMessage.success('创建成功')
        } else {
          await api.updateForward(form.id, body)
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

    const toggle = async (row) => {
      try {
        await api.toggleForward(row.id)
        ElementPlus.ElMessage.success(row.status === 1 ? '任务已停止' : '任务已启动')
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除转推任务「${row.name}」吗？`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteForward(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    const errVisible = ref(false)
    const errText = ref('')
    const showError = (row) => { errText.value = row.last_error || '无'; errVisible.value = true }

    return {
      store, loading, list, total, query, load, search, fmtTime, runStatusTag, runStatusText,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm,
      toggle, remove, errVisible, errText, showError,
      PLATFORMS, platform, currentPlatform, onPlatformChange, onSchemaChange,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <el-input v-model="query.keyword" placeholder="名称 / 流ID / 目标地址" clearable style="width:240px"
                  @keyup.enter="search" @clear="search" />
        <el-select v-model="query.status" placeholder="状态" clearable style="width:120px" @change="search">
          <el-option label="启用" :value="1" />
          <el-option label="禁用" :value="0" />
        </el-select>
        <el-button type="primary" icon="Search" @click="search">查询</el-button>
        <div class="spacer"></div>
        <el-button icon="Refresh" @click="load">刷新</el-button>
        <el-button v-if="store.has('forward:create')" type="primary" icon="Plus" @click="openCreate">新增转推</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="任务名称" min-width="110" show-overflow-tooltip />
        <el-table-column label="源流" min-width="120">
          <template #default="{ row }"><span class="mono">{{ row.src_app }}/{{ row.src_stream }}</span></template>
        </el-table-column>
        <el-table-column prop="dst_url" label="目标地址" min-width="220" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono">{{ row.dst_url }}</span></template>
        </el-table-column>
        <el-table-column prop="schema" label="协议" width="80" align="center" />
        <el-table-column label="运行状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="runStatusTag(row.run_status)" size="small">{{ runStatusText(row.run_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="重试" width="80" align="center">
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
            <el-switch :model-value="row.status === 1" :disabled="!store.has('forward:toggle')" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="row.last_error" text type="danger" size="small" @click="showError(row)">错误</el-button>
            <el-button v-if="store.has('forward:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="store.has('forward:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                       v-model:current-page="query.page" v-model:page-size="query.page_size"
                       :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="search" />
      </div>

      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增转推任务' : '编辑转推任务'" width="620px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
          <el-form-item label="目标平台">
            <el-select v-model="platform" style="width:220px" @change="onPlatformChange">
              <el-option v-for="p in PLATFORMS" :key="p.key" :label="p.name" :value="p.key" />
            </el-select>
            <span v-if="currentPlatform().spec" class="text-gray" style="margin-left:10px;font-size:12px">{{ currentPlatform().spec }}</span>
          </el-form-item>
          <el-alert v-if="currentPlatform().guide" type="info" :closable="false" :title="currentPlatform().guide"
                    style="margin-bottom:14px" />
          <el-form-item label="任务名称" prop="name">
            <el-input v-model="form.name" placeholder="如：转推到抖音" />
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="12" :xs="24">
              <el-form-item label="源 App">
                <el-input v-model="form.src_app" />
              </el-form-item>
            </el-col>
            <el-col :span="12" :xs="24">
              <el-form-item label="源流ID" prop="src_stream">
                <el-input v-model="form.src_stream" placeholder="本地已存在的流ID" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-form-item label="目标地址" prop="dst_url">
            <el-input v-model="form.dst_url" class="mono"
                      :placeholder="currentPlatform().server ? currentPlatform().server + '<StreamKey>' : 'rtmp://目标服务器/app/streamKey（从平台后台复制）'" />
            <div v-if="currentPlatform().server" class="text-gray" style="font-size:12px">
              已预填平台服务器地址，只需在后面补上你的 Stream Key
            </div>
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="8" :xs="24">
              <el-form-item label="协议" label-width="60px">
                <el-select v-model="form.schema" style="width:100%" @change="onSchemaChange">
                  <el-option label="rtmp" value="rtmp" />
                  <el-option label="rtmps" value="rtmps" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="8" :xs="24">
              <el-form-item label="重试次数" label-width="80px">
                <el-input-number controls-position="right" v-model="form.retry_count" :min="-1" style="width:100%" />
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
            <span class="text-gray" style="margin-left:8px;font-size:12px">源上线时自动开始转推</span>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">确定</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="errVisible" title="最近一次错误" width="560px">
        <el-alert type="error" :closable="false">
          <pre class="mono" style="white-space:pre-wrap;margin:0">{{ errText }}</pre>
        </el-alert>
      </el-dialog>
    </el-card>
  `,
}
