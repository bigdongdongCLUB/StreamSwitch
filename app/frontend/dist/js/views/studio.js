// 制播管理：制播任务（滚动字幕/Logo 压制、视频轮播）+ 素材库

import { api } from '../api.js'
import { fmtBytes, fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted, onUnmounted, computed } = Vue

const runStatusTag = (s) => ({ running: 'success', failed: 'danger', stopped: 'info', idle: 'info' }[s] || 'info')
const runStatusText = (s) => ({ running: '运行中', failed: '失败', stopped: '已停止', idle: '空闲' }[s] || s)
const modeText = (m) => ({ loop: '视频轮播', relay: '拉流压制' }[m] || m)
const logoPosText = { tl: '左上', tr: '右上', bl: '左下', br: '右下' }

const fmtDuration = (sec) => {
  if (!sec) return '-'
  const m = Math.floor(sec / 60), s = sec % 60
  return `${m}分${String(s).padStart(2, '0')}秒`
}

export default {
  name: 'StudioView',
  setup() {
    const tab = ref('tasks')

    // ================= 任务列表 =================
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20, keyword: '' })

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listStudioTasks({ ...query })
        list.value = d.list || []
        total.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    const search = () => { query.page = 1; load() }

    let timer = null
    onMounted(() => {
      load()
      loadMaterials()
      loadStreams()
      timer = setInterval(load, 15000)
    })
    onUnmounted(() => clearInterval(timer))

    // ================= 素材库 =================
    const materials = ref([])
    const matType = ref('video')
    const matLoading = ref(false)
    const loadMaterials = async () => {
      matLoading.value = true
      try {
        materials.value = await api.listStudioMaterials('') || []
      } catch (e) { /* ignore */ } finally {
        matLoading.value = false
      }
    }
    const videoMaterials = computed(() => materials.value.filter(m => m.type === 'video'))
    const logoMaterials = computed(() => materials.value.filter(m => m.type === 'logo'))
    const shownMaterials = computed(() => materials.value.filter(m => m.type === matType.value))

    // 本地流列表（relay 模式源流选择）
    const streamList = ref([])
    const loadStreams = async () => {
      try {
        const d = await api.listStreams({ page: 1, page_size: 200 })
        streamList.value = d.list || []
      } catch (e) { /* 无权限时忽略 */ }
    }

    // 上传
    const uploading = ref(false)
    const uploadPct = ref(0)
    const fileInput = ref(null)
    const pickFile = () => fileInput.value && fileInput.value.click()
    const onFilePicked = async (e) => {
      const file = e.target.files[0]
      e.target.value = ''
      if (!file) return
      uploading.value = true
      uploadPct.value = 0
      try {
        const fd = new FormData()
        fd.append('type', matType.value)   // type 放在 file 之前：后端流式解析时先拿到类型再落盘
        fd.append('file', file)
        await api.uploadStudioMaterial(fd, (p) => { uploadPct.value = p })
        ElementPlus.ElMessage.success('上传成功')
        loadMaterials()
      } catch (err) {
        ElementPlus.ElMessage.error(err.message)
      } finally {
        uploading.value = false
      }
    }
    const removeMaterial = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除素材「${row.name}」吗？文件将一并删除。`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteStudioMaterial(row.id)
          ElementPlus.ElMessage.success('删除成功')
          loadMaterials()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    // ================= 任务编辑 =================
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const blank = {
      id: 0, name: '', mode: 'loop', material_id: 0, src_url: '',
      app: 'live', stream: '',
      src_mode: 'local', local_stream: '', // relay 模式：源流选择方式
      caption_enabled: false, caption_text: '', caption_size: 28,
      caption_color: 'white', caption_position: 'bottom', caption_speed: 80,
      logo_material_id: 0, logo_position: 'tr', logo_opacity: 100, logo_width_pct: 0,
      vbitrate: 1500, quality_mode: 'bitrate', crf: 21, status: 1, retry_count: -1,
    }
    const form = reactive({ ...blank })
    const rules = {
      name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
      stream: [
        { required: true, message: '请输入输出流ID', trigger: 'blur' },
        { pattern: /^[A-Za-z0-9_-]+$/, message: '流ID只允许字母、数字、下划线、中划线', trigger: 'blur' },
      ],
      material_id: [{ validator: (r, v, cb) => cb(form.mode === 'loop' && !v ? new Error('请选择轮播视频素材') : undefined), trigger: 'change' }],
      src_url: [{ validator: (r, v, cb) => cb(form.mode === 'relay' && !v ? new Error('请输入源流地址') : undefined), trigger: 'blur' }],
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
        caption_enabled: row.caption_enabled === 1,
      })
      // 回显源流选择方式
      if (row.src_url && row.src_url.startsWith('local://')) {
        form.src_mode = 'local'
        form.local_stream = row.src_url.slice('local://'.length)
      } else {
        form.src_mode = 'custom'
      }
      dlgVisible.value = true
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      dlgLoading.value = true
      // relay 模式：本地流选择转为 local:// 写法
      const body = { ...form }
      if (body.mode === 'relay' && body.src_mode === 'local') {
        if (!body.local_stream) {
          ElementPlus.ElMessage.warning('请选择本地流')
          dlgLoading.value = false
          return
        }
        body.src_url = 'local://' + body.local_stream
      }
      try {
        if (dlgMode.value === 'create') {
          await api.createStudioTask(body)
          ElementPlus.ElMessage.success('创建成功' + (form.status === 1 ? '，已自动启动' : ''))
        } else {
          await api.updateStudioTask(form.id, body)
          ElementPlus.ElMessage.success('更新成功' + (form.run_status === 'running' ? '，任务已按新配置重启' : ''))
        }
        dlgVisible.value = false
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        dlgLoading.value = false
      }
    }

    // ================= 任务操作 =================
    const toggle = async (row) => {
      try {
        const d = await api.toggleStudioTask(row.id)
        ElementPlus.ElMessage.success(d && d.run_status === 'running' ? '任务已启动' : '任务已停止')
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除制播任务「${row.name}」吗？运行中的任务会先停止。`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteStudioTask(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    // 错误详情
    const errVisible = ref(false)
    const errText = ref('')
    const showError = (row) => { errText.value = row.last_error || '无'; errVisible.value = true }

    // 素材名称回显
    const materialName = (id) => {
      const m = materials.value.find(x => x.id === id)
      return m ? m.name : (id ? '#' + id : '-')
    }

    return {
      store, tab, loading, list, total, query, load, search, fmtTime, fmtBytes,
      runStatusTag, runStatusText, modeText, logoPosText, fmtDuration,
      materials, matType, matLoading, videoMaterials, logoMaterials, shownMaterials,
      streamList,
      uploading, uploadPct, fileInput, pickFile, onFilePicked, removeMaterial, loadMaterials,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm,
      toggle, remove, errVisible, errText, showError, materialName,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <el-tabs v-model="tab">
        <!-- ================= 制播任务 ================= -->
        <el-tab-pane label="制播任务" name="tasks">
          <div class="page-toolbar">
            <el-input v-model="query.keyword" placeholder="任务名称 / 流ID" clearable style="width:220px"
                      @keyup.enter="search" @clear="search" />
            <el-button type="primary" icon="Search" @click="search">查询</el-button>
            <div class="spacer"></div>
            <el-button icon="Refresh" @click="load">刷新</el-button>
            <el-button v-if="store.has('studio:create')" type="primary" icon="Plus" @click="openCreate">新建任务</el-button>
          </div>

          <el-table :data="list" v-loading="loading" size="small" border>
            <el-table-column prop="id" label="ID" width="55" />
            <el-table-column prop="name" label="任务名称" min-width="130" show-overflow-tooltip />
            <el-table-column label="模式" width="95" align="center">
              <template #default="{ row }">
                <el-tag :type="row.mode === 'loop' ? 'primary' : 'warning'" size="small">{{ modeText(row.mode) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="源" min-width="150" show-overflow-tooltip>
              <template #default="{ row }">
                <span v-if="row.mode === 'loop'">{{ materialName(row.material_id) }}</span>
                <span v-else class="mono" style="font-size:12px">{{ row.src_url }}</span>
              </template>
            </el-table-column>
            <el-table-column label="输出流" min-width="110">
              <template #default="{ row }"><span class="mono">{{ row.app }}/{{ row.stream }}</span></template>
            </el-table-column>
            <el-table-column label="叠加" min-width="110">
              <template #default="{ row }">
                <el-tag v-if="row.caption_enabled === 1" size="small" type="info" style="margin-right:4px">字幕</el-tag>
                <el-tag v-if="row.logo_material_id > 0" size="small" type="info">Logo</el-tag>
                <span v-if="!row.caption_enabled && !row.logo_material_id" class="text-gray">-</span>
              </template>
            </el-table-column>
            <el-table-column label="运行状态" width="90" align="center">
              <template #default="{ row }">
                <el-tag :type="runStatusTag(row.run_status)" size="small">{{ runStatusText(row.run_status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="重试" width="75" align="center">
              <template #default="{ row }">
                <span :class="{ 'text-danger': row.retry_times > 0 }">{{ row.retry_times }}</span>
                <span class="text-gray">/{{ row.retry_count < 0 ? '∞' : row.retry_count }}</span>
              </template>
            </el-table-column>
            <el-table-column label="最近活跃" min-width="145">
              <template #default="{ row }">{{ fmtTime(row.last_active_at) }}</template>
            </el-table-column>
            <el-table-column label="启用" width="65" align="center">
              <template #default="{ row }">
                <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="230" fixed="right" class-name="col-actions">
              <template #default="{ row }">
                <template v-if="store.has('studio:toggle')">
                  <el-button v-if="row.run_status !== 'running'" text type="success" size="small" @click="toggle(row)">启动</el-button>
                  <el-button v-else text type="warning" size="small" @click="toggle(row)">停止</el-button>
                </template>
                <el-button v-if="row.last_error" text type="danger" size="small" @click="showError(row)">错误</el-button>
                <el-button v-if="store.has('studio:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
                <el-button v-if="store.has('studio:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                           v-model:current-page="query.page" v-model:page-size="query.page_size"
                           :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="search" />
          </div>
        </el-tab-pane>

        <!-- ================= 素材库 ================= -->
        <el-tab-pane label="素材库" name="materials">
          <div class="page-toolbar">
            <el-radio-group v-model="matType">
              <el-radio-button value="video">视频素材</el-radio-button>
              <el-radio-button value="logo">Logo 图片</el-radio-button>
            </el-radio-group>
            <div class="spacer"></div>
            <template v-if="store.has('studio:create')">
              <el-button type="primary" icon="Upload" :disabled="uploading" @click="pickFile">
                {{ uploading ? '上传中' : (matType === 'video' ? '上传视频' : '上传 Logo') }}
              </el-button>
              <input ref="fileInput" type="file" style="display:none"
                     :accept="matType === 'video' ? '.mp4,.flv,.mkv,.mov,.ts,.avi,.webm' : '.png,.jpg,.jpeg,.webp,.gif'"
                     @change="onFilePicked" />
            </template>
          </div>
          <div v-if="uploading" style="margin-bottom:12px">
            <el-progress :percentage="uploadPct" :status="uploadPct >= 100 ? 'success' : ''" />
            <div class="text-gray" style="font-size:12px;margin-top:4px">
              {{ uploadPct >= 100 ? '文件已传完，服务器正在落盘并解析视频信息，请稍候…' : '正在上传，请勿关闭页面' }}
            </div>
          </div>

          <el-table :data="shownMaterials" v-loading="matLoading" size="small" border>
            <el-table-column prop="id" label="ID" width="55" />
            <el-table-column prop="name" label="素材名称" min-width="180" show-overflow-tooltip />
            <el-table-column label="大小" width="100">
              <template #default="{ row }">{{ fmtBytes(row.size) }}</template>
            </el-table-column>
            <el-table-column label="时长" width="100">
              <template #default="{ row }">{{ row.type === 'video' ? fmtDuration(row.duration) : '-' }}</template>
            </el-table-column>
            <el-table-column label="分辨率" width="110">
              <template #default="{ row }">{{ row.width ? row.width + 'x' + row.height : '-' }}</template>
            </el-table-column>
            <el-table-column label="上传时间" min-width="150">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="90" fixed="right" class-name="col-actions">
              <template #default="{ row }">
                <el-button v-if="store.has('studio:delete')" text type="danger" size="small" @click="removeMaterial(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-alert type="info" :closable="false" style="margin-top:12px"
                    title="视频素材用于「视频轮播」任务循环播出；Logo 图片建议 PNG 透明底，用于压制叠加" />
        </el-tab-pane>
      </el-tabs>

      <!-- 新建/编辑任务 -->
      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新建制播任务' : '编辑制播任务'" width="720px" top="4vh">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
          <el-form-item label="任务名称" prop="name">
            <el-input v-model="form.name" placeholder="如：7x24 轮播频道 / 会议室压制播出" />
          </el-form-item>
          <el-form-item label="任务模式">
            <el-radio-group v-model="form.mode" :disabled="dlgMode === 'edit'">
              <el-radio-button value="loop">视频轮播</el-radio-button>
              <el-radio-button value="relay">拉流压制</el-radio-button>
            </el-radio-group>
            <div class="text-gray" style="font-size:12px">轮播：本地素材循环播出；压制：对一路已有直播流叠加字幕/Logo 后另发一路</div>
          </el-form-item>
          <el-form-item v-if="form.mode === 'loop'" label="轮播素材" prop="material_id">
            <el-select v-model="form.material_id" style="width:100%" filterable placeholder="选择视频素材（先到素材库上传）">
              <el-option v-for="m in videoMaterials" :key="m.id"
                         :label="m.name + '（' + fmtDuration(m.duration) + ' / ' + (m.width ? m.width + 'x' + m.height : '?') + '）'"
                         :value="m.id" />
            </el-select>
          </el-form-item>
          <template v-else>
            <el-form-item label="源流来源">
              <el-radio-group v-model="form.src_mode">
                <el-radio-button value="local">选择本地流</el-radio-button>
                <el-radio-button value="custom">自定义地址</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item v-if="form.src_mode === 'local'" label="本地流" prop="local_stream">
              <el-select v-model="form.local_stream" style="width:100%" filterable placeholder="选择本机已登记的流">
                <el-option v-for="s in streamList" :key="s.id"
                           :label="s.name + ' (' + s.app + '/' + s.stream + ')' + (s.online === 1 ? ' ·在线' : '')"
                           :value="s.app + '/' + s.stream" />
              </el-select>
              <div class="text-gray" style="font-size:12px">压制时自动使用本机地址并携带内部令牌，无需关心播放鉴权</div>
            </el-form-item>
            <el-form-item v-else label="源流地址" prop="src_url">
              <el-input v-model="form.src_url" class="mono" placeholder="http(s)://...flv / rtmp://... / ...m3u8（外部源地址）" />
            </el-form-item>
          </template>
          <el-row :gutter="12">
            <el-col :span="10" :xs="24">
              <el-form-item label="输出 App">
                <el-input v-model="form.app" />
              </el-form-item>
            </el-col>
            <el-col :span="14" :xs="24">
              <el-form-item label="输出流ID" prop="stream">
                <el-input v-model="form.stream" placeholder="播出流ID，不存在时将自动创建" />
              </el-form-item>
            </el-col>
          </el-row>

          <el-divider content-position="left">滚动字幕</el-divider>
          <el-form-item label="开启字幕">
            <el-switch v-model="form.caption_enabled" />
          </el-form-item>
          <template v-if="form.caption_enabled">
            <el-form-item label="字幕内容">
              <el-input v-model="form.caption_text" maxlength="200" show-word-limit
                        placeholder="如：本直播由流媒体平台提供技术支持" />
            </el-form-item>
            <el-row :gutter="12">
              <el-col :span="7" :xs="24">
                <el-form-item label="字号">
                  <el-input-number controls-position="right" v-model="form.caption_size" :min="16" :max="120" style="width:100%" />
                </el-form-item>
              </el-col>
              <el-col :span="8" :xs="24">
                <el-form-item label="颜色">
                  <el-select v-model="form.caption_color" style="width:100%">
                    <el-option label="白色" value="white" />
                    <el-option label="黄色" value="yellow" />
                    <el-option label="红色" value="red" />
                    <el-option label="绿色" value="#00ff00" />
                    <el-option label="蓝色" value="#409eff" />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :span="9" :xs="24">
                <el-form-item label="位置" label-width="60px">
                  <el-radio-group v-model="form.caption_position">
                    <el-radio-button value="bottom">底部</el-radio-button>
                    <el-radio-button value="top">顶部</el-radio-button>
                  </el-radio-group>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="滚动速度">
              <el-slider v-model="form.caption_speed" :min="20" :max="300" :format-tooltip="v => v + ' px/s'" />
            </el-form-item>
          </template>

          <el-divider content-position="left">Logo 水印</el-divider>
          <el-form-item label="Logo 图片">
            <el-select v-model="form.logo_material_id" style="width:100%" clearable placeholder="不使用 Logo（先到素材库上传图片）">
              <el-option :value="0" label="不使用" />
              <el-option v-for="m in logoMaterials" :key="m.id"
                         :label="m.name + '（' + m.width + 'x' + m.height + '）'" :value="m.id" />
            </el-select>
          </el-form-item>
          <template v-if="form.logo_material_id > 0">
            <el-row :gutter="12">
              <el-col :span="7" :xs="24">
                <el-form-item label="位置" label-width="60px">
                  <el-select v-model="form.logo_position" style="width:100%">
                    <el-option v-for="(t, p) in logoPosText" :key="p" :label="t" :value="p" />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :span="8" :xs="24">
                <el-form-item label="不透明度%" label-width="80px">
                  <el-input-number controls-position="right" v-model="form.logo_opacity" :min="10" :max="100" style="width:100%" />
                </el-form-item>
              </el-col>
              <el-col :span="9" :xs="24">
                <el-form-item label="宽度占比%" label-width="80px">
                  <el-input-number controls-position="right" v-model="form.logo_width_pct" :min="0" :max="50" style="width:100%" />
                </el-form-item>
              </el-col>
            </el-row>
          </template>

          <el-divider content-position="left">输出参数</el-divider>
          <template v-if="form.caption_enabled || form.logo_material_id > 0">
            <el-form-item label="编码模式">
              <el-radio-group v-model="form.quality_mode">
                <el-radio-button value="bitrate">固定码率</el-radio-button>
                <el-radio-button value="crf">画质优先 (CRF)</el-radio-button>
              </el-radio-group>
              <span class="text-gray" style="margin-left:10px;font-size:12px">
                {{ form.quality_mode === 'crf' ? '画质稳定，码率随画面复杂度自适应' : '带宽恒定，适合严格控制出口带宽' }}
              </span>
            </el-form-item>
          </template>
          <el-row :gutter="12">
            <el-col :span="8" :xs="24" v-if="form.quality_mode === 'crf' && (form.caption_enabled || form.logo_material_id > 0)">
              <el-form-item label="CRF 质量" label-width="80px">
                <el-input-number controls-position="right" v-model="form.crf" :min="18" :max="28" style="width:100%" />
                <div class="text-gray" style="font-size:12px">18~23 越小越清晰（体积越大）</div>
              </el-form-item>
            </el-col>
            <el-col :span="8" :xs="24">
              <el-form-item :label="form.quality_mode === 'crf' ? '码率上限k' : '视频码率k'" label-width="90px">
                <el-input-number controls-position="right" v-model="form.vbitrate" :min="400" :max="8000" :step="100" style="width:100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8" :xs="24">
              <el-form-item label="重试次数" label-width="80px">
                <el-input-number controls-position="right" v-model="form.retry_count" :min="-1" style="width:100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12">
            <el-col :span="8" :xs="24">
              <el-form-item label="启用任务" label-width="80px">
                <el-switch :model-value="form.status === 1" @change="v => form.status = v ? 1 : 0" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-alert v-if="form.caption_enabled || form.logo_material_id > 0" type="warning" :closable="false"
                    title="叠加字幕/Logo 需要视频重编码（约占用 1 核 CPU / 1080p）；纯轮播无叠加时为转封装模式，几乎不占 CPU" />
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
