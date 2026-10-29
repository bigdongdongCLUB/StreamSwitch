// 流管理：列表 / 新增 / 编辑 / 启停 / 删除 / 详情 / 播放地址 / 同步

import { api } from '../api.js'
import { fmtTime, fmtBytes, fmtDuration, copyText } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'StreamsView',
  setup() {
    // ---------- 列表 ----------
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20, keyword: '', status: '' })

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listStreams({ ...query })
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

    // ---------- 新增 / 编辑 ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create') // create | edit
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const form = reactive({ id: 0, name: '', app: 'live', stream: '', push_key: '', remark: '', status: 1 })
    const rules = {
      name: [{ required: true, message: '请输入流名称', trigger: 'blur' }],
      stream: [
        { required: true, message: '请输入流ID', trigger: 'blur' },
        { pattern: /^[A-Za-z0-9_-]+$/, message: '流ID只允许字母、数字、下划线、中划线', trigger: 'blur' },
      ],
    }

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, { id: 0, name: '', app: 'live', stream: '', push_key: '', remark: '', status: 1 })
      dlgVisible.value = true
    }
    const openEdit = (row) => {
      dlgMode.value = 'edit'
      Object.assign(form, {
        id: row.id, name: row.name, app: row.app, stream: row.stream,
        push_key: row.push_key, remark: row.remark, status: row.status,
      })
      dlgVisible.value = true
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      dlgLoading.value = true
      try {
        if (dlgMode.value === 'create') {
          const d = await api.createStream({
            name: form.name, app: form.app, stream: form.stream,
            push_key: form.push_key, remark: form.remark,
          })
          ElementPlus.ElMessage.success('流创建成功')
          dlgVisible.value = false
          load()
          if (d && d.publish_url) showUrlDialogById(d.stream.id)
        } else {
          await api.updateStream(form.id, {
            name: form.name, push_key: form.push_key, remark: form.remark, status: form.status,
          })
          ElementPlus.ElMessage.success('更新成功')
          dlgVisible.value = false
          load()
        }
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        dlgLoading.value = false
      }
    }

    // ---------- 启停 / 删除 / 同步 ----------
    const toggle = async (row) => {
      // 禁用会立即断开正在进行的推流，并拒绝其重连；在线流需二次确认
      if (row.status === 1 && row.online === 1) {
        try {
          await ElementPlus.ElMessageBox.confirm(
            `流「${row.name}」(${row.app}/${row.stream}) 正在直播中，禁用将立即断开推流并拒绝重连，观众会中断观看。确定禁用吗？`,
            '禁用在线流', { type: 'warning', confirmButtonText: '禁用', confirmButtonClass: 'el-button--danger' })
        } catch (e) { return }
      }
      try {
        const d = await api.toggleStream(row.id)
        ElementPlus.ElMessage.success(d && d.status === 1 ? '流已启用，可以开始推流' : '流已禁用，推流已断开')
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除流「${row.name}」(${row.app}/${row.stream}) 吗？`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteStream(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }
    const sync = async () => {
      try {
        await api.syncStreams()
        ElementPlus.ElMessage.success('已与 ZLMediaKit 同步')
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }

    // ---------- 详情 ----------
    const infoVisible = ref(false)
    const infoLoading = ref(false)
    const info = ref(null)
    const showInfo = async (row) => {
      infoVisible.value = true
      infoLoading.value = true
      try {
        info.value = await api.getStreamInfo(row.id)
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
        infoVisible.value = false
      } finally {
        infoLoading.value = false
      }
    }

    // ---------- 播放地址 ----------
    const urlVisible = ref(false)
    const urlLoading = ref(false)
    const urlStream = ref(null)
    const urlResult = ref(null)
    const urlForm = reactive({ domain_id: 0, need_token: true, expire_sec: 3600, ip_bind: '', format: '' })
    const domainOptions = ref([])

    const showUrlDialogById = async (id) => {
      const row = list.value.find(s => s.id === id)
      if (row) showUrls(row)
    }
    const showUrls = async (row) => {
      urlStream.value = row
      urlResult.value = null
      Object.assign(urlForm, { domain_id: 0, need_token: true, expire_sec: 3600, ip_bind: '', format: '' })
      urlVisible.value = true
      try {
        const [domains, urls] = await Promise.all([
          store.has('domain:list') ? api.listDomains().catch(() => []) : Promise.resolve([]),
          api.getStreamURLs(row.id, 0),
        ])
        domainOptions.value = domains || []
        urlResult.value = urls
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const genUrls = async () => {
      urlLoading.value = true
      try {
        urlResult.value = await api.genStreamURL({
          stream_id: urlStream.value.id,
          domain_id: urlForm.domain_id,
          expire_sec: urlForm.expire_sec,
          ip_bind: urlForm.ip_bind,
          format: urlForm.format,
          need_token: urlForm.need_token,
        })
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        urlLoading.value = false
      }
    }
    const copy = async (text) => {
      const ok = await copyText(text)
      ElementPlus.ElMessage[ok ? 'success' : 'error'](ok ? '已复制' : '复制失败，请手动复制')
    }

    const formatNames = { rtmp: 'RTMP', 'http-flv': 'HTTP-FLV', 'ws-flv': 'WS-FLV', hls: 'HLS' }

    // 拆分完整推流地址为 OBS 两段式（服务器 + 串流密钥）
    // rtmp://host[:port]/app/stream?key=xxx => { server: rtmp://host[:port]/app, streamKey: stream?key=xxx }
    const splitPublishURL = (url) => {
      if (!url) return null
      const m = url.match(/^(rtmps?:\/\/[^/]+\/[^/]+)\/([^?]+)(?:\?(.*))?$/)
      if (!m) return null
      return { server: m[1], streamKey: m[2] + (m[3] ? '?' + m[3] : '') }
    }
    const infoObsParts = () => info.value ? splitPublishURL(info.value.publish_url) : null
    // 播放地址弹窗中的推流配置（从 rtmp 播放地址反推服务器段）
    const urlObsParts = () => {
      if (!urlResult.value || !urlStream.value) return null
      const rtmp = urlResult.value.urls && urlResult.value.urls.rtmp
      if (!rtmp) return null
      const server = rtmp.slice(0, rtmp.lastIndexOf('/'))
      const key = urlStream.value.push_key
      return { server, streamKey: urlStream.value.stream + (key ? '?key=' + key : '') }
    }

    // ---------- 视频预览 ----------
    const playVisible = ref(false)
    const playLoading = ref(false)
    const playStream = ref(null)
    const playFormat = ref('')
    const playFormats = ref([])
    const playError = ref('')
    const videoRef = ref(null)
    let flvPlayer = null
    let hlsInst = null

    const destroyPlayer = () => {
      try { flvPlayer && flvPlayer.pause && flvPlayer.unload && flvPlayer.destroy() } catch (e) { /* ignore */ }
      try { hlsInst && hlsInst.destroy() } catch (e) { /* ignore */ }
      flvPlayer = null
      hlsInst = null
    }

    // 预览地址统一使用当前页面源（同源播放，内网/外网访问均可用，与播放域名配置解耦）
    const toSameOrigin = (u) => {
      try {
        const x = new URL(u)
        return location.origin + x.pathname + x.search
      } catch (e) { return u }
    }

    const playWith = (f) => {
      destroyPlayer()
      playError.value = ''
      playFormat.value = f.key
      const video = videoRef.value
      if (!video) {
        playError.value = '播放器初始化失败，请关闭弹窗重试'
        return
      }
      video.muted = true
      if (f.key === 'http-flv') {
        if (!window.flvjs || !flvjs.isSupported()) {
          playError.value = '当前浏览器不支持 FLV 播放，请切换到 HLS'
          return
        }
        flvPlayer = flvjs.createPlayer({ type: 'flv', isLive: true, url: f.url })
        flvPlayer.on(flvjs.Events.ERROR, (type, detail, err) => {
          playError.value = 'FLV 播放失败: ' + (detail || type || '网络错误') + '（' + f.url + '）'
        })
        flvPlayer.attachMediaElement(video)
        flvPlayer.load()
        flvPlayer.play().catch(() => { playError.value = '播放被浏览器拦截，请点击视频播放按钮' })
      } else {
        if (window.Hls && Hls.isSupported()) {
          hlsInst = new Hls({ enableWorker: true })
          hlsInst.loadSource(f.url)
          hlsInst.attachMedia(video)
          hlsInst.on(Hls.Events.MANIFEST_PARSED, () => video.play().catch(() => {}))
          hlsInst.on(Hls.Events.ERROR, (ev, data) => {
            if (data && data.fatal) playError.value = 'HLS 播放失败: ' + (data.details || '')
          })
        } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
          // Safari 原生 HLS
          video.src = f.url
          video.play().catch(() => {})
        } else {
          playError.value = '当前浏览器不支持 HLS 播放'
        }
      }
    }

    const openPreview = async (row) => {
      if (row.online !== 1) {
        ElementPlus.ElMessage.warning('流当前离线，请先推流后再预览')
        return
      }
      playStream.value = row
      playFormats.value = []
      playFormat.value = ''
      playError.value = ''
      playVisible.value = true
      playLoading.value = true
      try {
        // 生成带令牌的播放地址（预览用，1 小时有效）
        const d = await api.genStreamURL({ stream_id: row.id, need_token: true, expire_sec: 3600 })
        const fmts = []
        // 编码提示：H265 浏览器普遍不支持
        if (row.video_codec && /265|hevc/i.test(row.video_codec)) {
          playError.value = '提示：视频编码为 ' + row.video_codec + '，浏览器可能无法解码，建议改用 H264'
        }
        if (!d.encrypted && d.urls['http-flv']) {
          fmts.push({ key: 'http-flv', label: 'HTTP-FLV（低延迟）', url: toSameOrigin(d.urls['http-flv']) })
        }
        if (d.urls.hls) {
          fmts.push({ key: 'hls', label: d.encrypted ? 'HLS（加密）' : 'HLS', url: toSameOrigin(d.urls.hls) })
        }
        playFormats.value = fmts
        await Vue.nextTick()
        if (fmts.length > 0) {
          playWith(fmts[0])
        } else {
          playError.value = '没有可用的播放格式（可能相关格式已在「播放设置」中关闭）'
        }
      } catch (e) {
        playError.value = e.message
      } finally {
        playLoading.value = false
      }
    }

    const closePreview = () => {
      destroyPlayer()
      playVisible.value = false
      playStream.value = null
    }

    return {
      store, loading, list, total, query, load, search, fmtTime, fmtBytes, fmtDuration,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm,
      toggle, remove, sync,
      infoVisible, infoLoading, info, showInfo, splitPublishURL, infoObsParts, urlObsParts,
      urlVisible, urlLoading, urlStream, urlResult, urlForm, domainOptions, showUrls, genUrls, copy, formatNames,
      playVisible, playLoading, playStream, playFormat, playFormats, playError, videoRef,
      openPreview, playWith, closePreview,
    }
  },
  template: `
    <div>
      <el-card class="page-card" shadow="never">
        <div class="page-toolbar">
          <el-input v-model="query.keyword" placeholder="名称 / 流ID" clearable style="width:220px"
                    @keyup.enter="search" @clear="search" />
          <el-select v-model="query.status" placeholder="状态" clearable style="width:120px" @change="search">
            <el-option label="启用" :value="1" />
            <el-option label="禁用" :value="0" />
          </el-select>
          <el-button type="primary" icon="Search" @click="search">查询</el-button>
          <div class="spacer"></div>
          <el-button v-if="store.has('stream:update')" icon="Refresh" @click="sync">同步状态</el-button>
          <el-button v-if="store.has('stream:create')" type="primary" icon="Plus" @click="openCreate">新增流</el-button>
        </div>

        <el-table :data="list" v-loading="loading" size="small" border>
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="name" label="名称" min-width="110" show-overflow-tooltip />
          <el-table-column label="流ID" min-width="130">
            <template #default="{ row }"><span class="mono">{{ row.app }}/{{ row.stream }}</span></template>
          </el-table-column>
          <el-table-column label="在线" width="70" align="center">
            <template #default="{ row }">
              <el-tag :type="row.online === 1 ? 'success' : 'info'" size="small">
                {{ row.online === 1 ? '在线' : '离线' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="编码/分辨率" width="140">
            <template #default="{ row }">
              <span v-if="row.online === 1">{{ row.video_codec || '-' }}<span v-if="row.width"> {{ row.width }}x{{ row.height }}</span></span>
              <span v-else class="text-gray">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="reader_count" label="观看" width="70" align="center" />
          <el-table-column label="累计流量" width="100">
            <template #default="{ row }">{{ fmtBytes(row.total_bytes) }}</template>
          </el-table-column>
          <el-table-column label="上线时间" min-width="150">
            <template #default="{ row }">{{ fmtTime(row.publish_time) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="80" align="center">
            <template #default="{ row }">
              <el-switch :model-value="row.status === 1" :disabled="!store.has('stream:toggle')"
                         @change="toggle(row)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="280" fixed="right" class-name="col-actions">
            <template #default="{ row }">
              <el-button text type="success" size="small" :disabled="row.online !== 1" @click="openPreview(row)">预览</el-button>
              <el-button text type="primary" size="small" @click="showInfo(row)">详情</el-button>
              <el-button text type="primary" size="small" @click="showUrls(row)">播放地址</el-button>
              <el-button v-if="store.has('stream:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
              <el-button v-if="store.has('stream:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="pager">
          <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                         v-model:current-page="query.page" v-model:page-size="query.page_size"
                         :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="search" />
        </div>
      </el-card>

      <!-- 新增/编辑 -->
      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增流' : '编辑流'" width="520px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
          <el-form-item label="流名称" prop="name">
            <el-input v-model="form.name" placeholder="展示用名称，如：会议室直播" />
          </el-form-item>
          <el-form-item label="App" prop="app">
            <el-input v-model="form.app" :disabled="dlgMode === 'edit'" placeholder="live" />
          </el-form-item>
          <el-form-item label="流ID" prop="stream">
            <el-input v-model="form.stream" :disabled="dlgMode === 'edit'" placeholder="自定义流Key，如 room001" />
          </el-form-item>
          <el-form-item label="推流Key">
            <el-input v-model="form.push_key" placeholder="推流鉴权Key，留空则不校验" />
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="form.remark" type="textarea" :rows="2" />
          </el-form-item>
          <el-form-item v-if="dlgMode === 'edit'" label="状态">
            <el-radio-group v-model="form.status">
              <el-radio :value="1">启用</el-radio>
              <el-radio :value="0">禁用</el-radio>
            </el-radio-group>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">确定</el-button>
        </template>
      </el-dialog>

      <!-- 详情 -->
      <el-dialog v-model="infoVisible" title="流详情" width="640px">
        <div v-loading="infoLoading">
          <template v-if="info">
            <el-descriptions :column="store.isMobile ? 1 : 2" border size="small">
              <el-descriptions-item label="名称">{{ info.name }}</el-descriptions-item>
              <el-descriptions-item label="流ID"><span class="mono">{{ info.app }}/{{ info.stream }}</span></el-descriptions-item>
              <el-descriptions-item label="在线状态">
                <el-tag :type="info.online === 1 ? 'success' : 'info'" size="small">{{ info.online_text }}</el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="推流地址" :span="2">
                <div style="display:flex;align-items:center;gap:8px">
                  <span class="mono">{{ info.publish_url }}</span>
                  <el-button size="small" icon="CopyDocument" @click="copy(info.publish_url)" />
                </div>
              </el-descriptions-item>
              <template v-if="infoObsParts()">
                <el-descriptions-item label="服务器" :span="2">
                  <div style="display:flex;align-items:center;gap:8px">
                    <span class="mono">{{ infoObsParts().server }}</span>
                    <el-button size="small" icon="CopyDocument" @click="copy(infoObsParts().server)" />
                  </div>
                </el-descriptions-item>
                <el-descriptions-item label="串流密钥" :span="2">
                  <div style="display:flex;align-items:center;gap:8px">
                    <span class="mono">{{ infoObsParts().streamKey }}</span>
                    <el-button size="small" icon="CopyDocument" @click="copy(infoObsParts().streamKey)" />
                  </div>
                </el-descriptions-item>
              </template>
              <el-descriptions-item label="视频编码">{{ info.video_codec || '-' }}</el-descriptions-item>
              <el-descriptions-item label="音频编码">{{ info.audio_codec || '-' }}</el-descriptions-item>
              <el-descriptions-item label="分辨率">{{ info.width ? info.width + 'x' + info.height : '-' }}</el-descriptions-item>
              <el-descriptions-item label="帧率">{{ info.fps || '-' }}</el-descriptions-item>
              <el-descriptions-item label="总码率">{{ info.bitrate ? info.bitrate + ' kbps' : '-' }}</el-descriptions-item>
              <el-descriptions-item label="采样率">{{ info.sample_rate || '-' }}</el-descriptions-item>
              <el-descriptions-item label="推流IP">{{ info.publish_ip || '-' }}</el-descriptions-item>
              <el-descriptions-item label="上线时间">{{ info.publish_time_text }}</el-descriptions-item>
              <el-descriptions-item label="当前观看">{{ info.reader_count }}</el-descriptions-item>
              <el-descriptions-item label="累计观看">{{ info.total_reader }}</el-descriptions-item>
              <el-descriptions-item label="累计流量">{{ fmtBytes(info.total_bytes) }}</el-descriptions-item>
              <el-descriptions-item label="创建时间">{{ fmtTime(info.created_at) }}</el-descriptions-item>
            </el-descriptions>
          </template>
        </div>
      </el-dialog>

      <!-- 视频预览 -->
      <el-dialog v-model="playVisible" :title="'视频预览 - ' + (playStream ? playStream.name : '')"
                 width="720px" :close-on-click-modal="false" @close="closePreview">
        <div v-loading="playLoading" style="min-height:200px">
          <video ref="videoRef" controls autoplay muted playsinline
                 style="width:100%;max-height:420px;background:#000;border-radius:6px"></video>
          <el-alert v-if="playError" :title="playError" type="warning" :closable="false" style="margin-top:10px" />
          <div style="display:flex;align-items:center;gap:10px;margin-top:12px">
            <span class="text-gray" style="font-size:13px">播放格式：</span>
            <el-radio-group :model-value="playFormat" size="small"
                            @change="k => { const f = playFormats.find(x => x.key === k); if (f) playWith(f) }">
              <el-radio-button v-for="f in playFormats" :key="f.key" :value="f.key">{{ f.label }}</el-radio-button>
            </el-radio-group>
            <div class="spacer" style="flex:1"></div>
            <span v-if="playStream" class="mono text-gray" style="font-size:12px">{{ playStream.app }}/{{ playStream.stream }}</span>
          </div>
          <div class="text-gray" style="font-size:12px;margin-top:8px">
            预览地址已附带 1 小时有效的播放令牌；预览为静音自动播放，可手动取消静音。
          </div>
        </div>
      </el-dialog>

      <!-- 播放地址 -->
      <el-dialog v-model="urlVisible" title="播放地址" width="680px">
        <template v-if="urlStream">
          <!-- OBS 推流配置 -->
          <template v-if="urlObsParts()">
            <el-divider content-position="left" style="margin-top:0">推流配置</el-divider>
            <div class="url-row">
              <span class="url-label">服务器</span>
              <el-input :model-value="urlObsParts().server" readonly size="small" />
              <el-button size="small" icon="CopyDocument" @click="copy(urlObsParts().server)" />
            </div>
            <div class="url-row">
              <span class="url-label">串流密钥</span>
              <el-input :model-value="urlObsParts().streamKey" readonly size="small" />
              <el-button size="small" icon="CopyDocument" @click="copy(urlObsParts().streamKey)" />
            </div>
            <el-divider content-position="left">播放地址</el-divider>
          </template>

          <el-form inline label-width="90px">
            <el-form-item label="播放域名">
              <el-select v-model="urlForm.domain_id" style="width:200px">
                <el-option label="默认域名" :value="0" />
                <el-option v-for="d in domainOptions" :key="d.id" :label="d.name + ' (' + d.host + ')'" :value="d.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="附带令牌">
              <el-switch v-model="urlForm.need_token" />
            </el-form-item>
            <el-form-item v-if="urlForm.need_token" label="有效期(秒)">
              <el-input-number controls-position="right" v-model="urlForm.expire_sec" :min="60" :step="300" style="width:140px" />
            </el-form-item>
            <el-form-item v-if="urlForm.need_token" label="限定格式">
              <el-select v-model="urlForm.format" clearable style="width:130px">
                <el-option v-for="(n, f) in formatNames" :key="f" :label="n" :value="f" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="urlForm.need_token" label="绑定IP">
              <el-input v-model="urlForm.ip_bind" placeholder="多个用逗号分隔" style="width:180px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="urlLoading" @click="genUrls">生成地址</el-button>
            </el-form-item>
          </el-form>

          <el-alert v-if="urlResult && urlResult.encrypted" type="warning" :closable="false"
                    title="该流已开启 HLS 加密，请使用加密播放地址" style="margin-bottom:12px" />

          <div v-if="urlResult">
            <div v-for="(u, f) in urlResult.urls" :key="f" class="url-row">
              <span class="url-label">{{ formatNames[f] || f }}</span>
              <el-input :model-value="u" readonly size="small" />
              <el-button size="small" icon="CopyDocument" @click="copy(u)" />
            </div>
            <div v-if="urlResult.disabled && urlResult.disabled.length" class="text-gray" style="margin-top:8px;font-size:12px">
              已关闭的格式：{{ urlResult.disabled.map(f => formatNames[f] || f).join('、') }}
            </div>
          </div>
        </template>
      </el-dialog>
    </div>
  `,
}
