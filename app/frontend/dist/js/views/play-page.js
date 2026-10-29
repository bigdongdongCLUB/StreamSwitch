// 观众直播播放页（免登录，/p/:code）
// 流程：打开 → 公开信息 →（有密码则校验）→ 下发播放地址 → 播放器

const { ref, onMounted, onUnmounted, nextTick } = Vue
const { useRoute } = VueRouter

export default {
  name: 'PlayPageView',
  setup() {
    const route = useRoute()
    const code = route.params.code

    const pageState = ref('loading') // loading / password / play / error / offline
    const pageInfo = ref(null)
    const errorMsg = ref('')
    const password = ref('')
    const authLoading = ref(false)
    const authError = ref('')
    const videoRef = ref(null)
    const playErr = ref('')
    const streamName = ref('')
    let flvPlayer = null
    let hlsInst = null

    // 播放地址转同源（观众从任意入口访问都可用）
    const toSameOrigin = (u) => {
      try {
        const x = new URL(u)
        return location.origin + x.pathname + x.search
      } catch (e) { return u }
    }

    const destroyPlayer = () => {
      try { flvPlayer && flvPlayer.unload && flvPlayer.destroy() } catch (e) { /* ignore */ }
      try { hlsInst && hlsInst.destroy() } catch (e) { /* ignore */ }
      flvPlayer = null
      hlsInst = null
    }

    const startPlay = (urls) => {
      const video = videoRef.value
      if (!video) return
      video.muted = false
      // 优先 HTTP-FLV（低延迟），兜底 HLS
      if (urls['http-flv'] && window.flvjs && flvjs.isSupported()) {
        flvPlayer = flvjs.createPlayer({ type: 'flv', isLive: true, url: toSameOrigin(urls['http-flv']) })
        flvPlayer.on(flvjs.Events.ERROR, (t, d) => {
          playErr.value = '播放中断: ' + (d || t)
        })
        flvPlayer.attachMediaElement(video)
        flvPlayer.load()
        flvPlayer.play().catch(() => {})
        return
      }
      if (urls.hls) {
        const src = toSameOrigin(urls.hls)
        if (window.Hls && Hls.isSupported()) {
          hlsInst = new Hls({ enableWorker: true })
          hlsInst.loadSource(src)
          hlsInst.attachMedia(video)
          hlsInst.on(Hls.Events.MANIFEST_PARSED, () => video.play().catch(() => {}))
          hlsInst.on(Hls.Events.ERROR, (ev, data) => {
            if (data && data.fatal) playErr.value = 'HLS 播放失败: ' + (data.details || '')
          })
        } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
          video.src = src
          video.play().catch(() => {})
        } else {
          playErr.value = '当前浏览器不支持播放，请使用 Chrome/Edge/Safari 最新版'
        }
        return
      }
      playErr.value = '没有可用的播放格式'
    }

    // 拉取播放数据
    const loadPlay = async () => {
      const ticket = sessionStorage.getItem('livepage_ticket_' + code) || ''
      try {
        const resp = await fetch(`/api/v1/p/${code}/play?ticket=` + encodeURIComponent(ticket))
        const d = await resp.json()
        if (d.code !== 0) {
          // 凭证失效回到密码输入
          if (pageInfo.value && pageInfo.value.need_password) {
            sessionStorage.removeItem('livepage_ticket_' + code)
            pageState.value = 'password'
            authError.value = d.message || '请重新输入访问密码'
            return
          }
          errorMsg.value = d.message || '加载失败'
          pageState.value = 'error'
          return
        }
        streamName.value = d.data.stream_name || d.data.name
        if (!d.data.online) {
          pageState.value = 'offline'
          return
        }
        pageState.value = 'play'
        await nextTick()
        startPlay(d.data.urls || {})
      } catch (e) {
        errorMsg.value = '网络错误，请稍后重试'
        pageState.value = 'error'
      }
    }

    // 密码校验
    const submitPassword = async () => {
      if (!password.value) return
      authLoading.value = true
      authError.value = ''
      try {
        const resp = await fetch(`/api/v1/p/${code}/auth`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ password: password.value }),
        })
        const d = await resp.json()
        if (d.code !== 0) {
          authError.value = d.message || '密码错误'
          return
        }
        sessionStorage.setItem('livepage_ticket_' + code, d.data.ticket)
        await loadPlay()
      } catch (e) {
        authError.value = '网络错误，请重试'
      } finally {
        authLoading.value = false
      }
    }

    onMounted(async () => {
      try {
        const resp = await fetch(`/api/v1/p/${code}`)
        const d = await resp.json()
        if (d.code !== 0) {
          errorMsg.value = d.message || '页面不存在或已停用'
          pageState.value = 'error'
          return
        }
        pageInfo.value = d.data
        document.title = d.data.name + ' - 直播'
        if (d.data.need_password) {
          pageState.value = 'password'
        } else {
          await loadPlay()
        }
      } catch (e) {
        errorMsg.value = '网络错误，请检查链接'
        pageState.value = 'error'
      }
    })
    onUnmounted(destroyPlayer)

    return {
      pageState, pageInfo, errorMsg, password, authLoading, authError,
      videoRef, playErr, streamName, submitPassword,
    }
  },
  template: `
    <div class="play-page">
      <div class="play-container">
        <!-- 加载中 -->
        <div v-if="pageState === 'loading'" class="play-status">
          <el-icon class="is-loading" :size="36" color="#409eff"><Loading /></el-icon>
          <p>加载中…</p>
        </div>

        <!-- 错误 -->
        <div v-else-if="pageState === 'error'" class="play-status">
          <el-icon :size="36" color="#f56c6c"><CircleClose /></el-icon>
          <p>{{ errorMsg }}</p>
        </div>

        <!-- 密码门 -->
        <div v-else-if="pageState === 'password'" class="play-password-card">
          <div class="play-title">{{ pageInfo?.name }}</div>
          <div class="play-sub">该直播设有访问密码，请输入后观看</div>
          <el-input v-model="password" placeholder="访问密码" size="large" style="margin:18px 0 12px"
                    @keyup.enter="submitPassword" />
          <el-alert v-if="authError" :title="authError" type="error" :closable="false" style="margin-bottom:12px" />
          <el-button type="primary" size="large" style="width:100%" :loading="authLoading" @click="submitPassword">
            进入直播
          </el-button>
        </div>

        <!-- 离线提示 -->
        <div v-else-if="pageState === 'offline'" class="play-status">
          <el-icon :size="36" color="#909399"><VideoPause /></el-icon>
          <p>直播暂未开始或已结束</p>
          <el-button text type="primary" @click="loadPlay">刷新试试</el-button>
        </div>

        <!-- 播放器 -->
        <template v-else-if="pageState === 'play'">
          <div class="play-header">
            <span class="play-live-dot"></span>
            <span class="play-title-sm">{{ pageInfo?.name }}</span>
            <span class="play-live-badge">LIVE</span>
          </div>
          <el-alert v-if="pageInfo?.announcement" type="info" :closable="false"
                    :title="pageInfo.announcement" style="margin-bottom:10px" />
          <video ref="videoRef" controls autoplay playsinline class="play-video"></video>
          <el-alert v-if="playErr" :title="playErr" type="warning" :closable="false" style="margin-top:10px" />
        </template>
      </div>
    </div>
  `,
}
