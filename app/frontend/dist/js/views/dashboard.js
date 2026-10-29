// 仪表盘：实时统计卡片 + 流量趋势图 + 在线流（含码率）

import { api } from '../api.js'
import { fmtBytes, fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, onMounted, onUnmounted, onActivated } = Vue

// 字节/秒 转 Mbps 文本
const fmtMbps = (bps) => {
  if (!bps || bps <= 0) return '0 Mbps'
  const mbps = (bps * 8) / 1e6
  if (mbps >= 1000) return (mbps / 1000).toFixed(2) + ' Gbps'
  if (mbps >= 1) return mbps.toFixed(2) + ' Mbps'
  return ((bps * 8) / 1e3).toFixed(1) + ' Kbps'
}

export default {
  name: 'DashboardView',
  setup() {
    const loading = ref(false)
    const stats = ref({
      streamTotal: 0, streamOnline: 0, readers: 0, totalBytes: 0,
      recvBps: 0, sendBps: 0,
      proxyTotal: 0, proxyRunning: 0,
      forwardTotal: 0, forwardRunning: 0,
    })
    const onlineStreams = ref([])

    // ---------- 趋势图 ----------
    const chartRef = ref(null)
    const trendRange = ref(60) // 分钟
    let chart = null

    const renderTrend = (points) => {
      if (!chart || !chartRef.value) return
      const times = points.map(p => {
        const d = new Date(p.ts * 1000)
        const hh = String(d.getHours()).padStart(2, '0')
        const mm = String(d.getMinutes()).padStart(2, '0')
        const ss = String(d.getSeconds()).padStart(2, '0')
        return trendRange.value >= 1440 ? `${hh}:${mm}` : `${hh}:${mm}:${ss}`
      })
      chart.setOption({
        grid: { left: 56, right: 20, top: 40, bottom: 30 },
        tooltip: {
          trigger: 'axis',
          valueFormatter: (v) => fmtMbps(v),
        },
        legend: { data: ['入流量（推流）', '出流量（分发）'], top: 6 },
        xAxis: { type: 'category', data: times, boundaryGap: false, axisLabel: { color: '#909399' } },
        yAxis: {
          type: 'value',
          axisLabel: { color: '#909399', formatter: (v) => fmtMbps(v) },
          splitLine: { lineStyle: { color: '#ebeef5' } },
        },
        series: [
          {
            name: '入流量（推流）', type: 'line', smooth: true, showSymbol: false,
            data: points.map(p => p.recv_bps),
            lineStyle: { color: '#409eff' }, itemStyle: { color: '#409eff' },
            areaStyle: { color: 'rgba(64,158,255,0.12)' },
          },
          {
            name: '出流量（分发）', type: 'line', smooth: true, showSymbol: false,
            data: points.map(p => p.send_bps),
            lineStyle: { color: '#67c23a' }, itemStyle: { color: '#67c23a' },
            areaStyle: { color: 'rgba(103,194,58,0.12)' },
          },
        ],
      })
    }

    const loadTrend = async () => {
      try {
        const points = await api.trafficTrend(trendRange.value) || []
        renderTrend(points)
        // 卡片实时速率取最新采样点
        if (points.length > 0) {
          const last = points[points.length - 1]
          stats.value.recvBps = last.recv_bps
          stats.value.sendBps = last.send_bps
        }
      } catch (e) { /* 静默，保留旧数据 */ }
    }

    // ---------- 概览数据 ----------
    const load = async () => {
      loading.value = true
      try {
        const jobs = []
        if (store.has('stream:list')) {
          jobs.push(api.listStreams({ page: 1, page_size: 200 }).then(d => {
            const list = d.list || []
            stats.value.streamTotal = d.total
            const online = list.filter(s => s.online === 1)
            stats.value.streamOnline = online.length
            // 只统计在线流：离线流不可能有人在看（兜底，后端下线时也会清零 reader_count）
            stats.value.readers = online.reduce((n, s) => n + (s.reader_count || 0), 0)
            stats.value.totalBytes = list.reduce((n, s) => n + (s.total_bytes || 0), 0)
            onlineStreams.value = online.slice(0, 8)
          }).catch(() => {}))
        }
        if (store.has('proxy:list')) {
          jobs.push(api.listProxies({ page: 1, page_size: 200 }).then(d => {
            stats.value.proxyTotal = d.total
            stats.value.proxyRunning = (d.list || []).filter(p => p.run_status === 'running').length
          }).catch(() => {}))
        }
        if (store.has('forward:list')) {
          jobs.push(api.listForwards({ page: 1, page_size: 200 }).then(d => {
            stats.value.forwardTotal = d.total
            stats.value.forwardRunning = (d.list || []).filter(f => f.run_status === 'running').length
          }).catch(() => {}))
        }
        await Promise.all(jobs)
        await loadTrend()
      } finally {
        loading.value = false
      }
    }

    const onResize = () => { chart && chart.resize() }
    let timer = null
    onMounted(() => {
      load()
      timer = setInterval(load, 30000)
      if (window.echarts && chartRef.value) {
        chart = echarts.init(chartRef.value)
      }
      window.addEventListener('resize', onResize)
    })
    onActivated(() => { chart && chart.resize() })
    onUnmounted(() => {
      clearInterval(timer)
      window.removeEventListener('resize', onResize)
      chart && chart.dispose()
    })

    const changeRange = (m) => { trendRange.value = m; loadTrend() }

    // 单流出流量估算 = 码率(kbps) × 观看数 → Mbps
    const streamSendMbps = (row) => fmtMbps((row.bitrate || 0) * 1024 / 8 * (row.reader_count || 0))
    const streamBitrateText = (row) => row.bitrate ? (row.bitrate >= 1024 ? (row.bitrate / 1024).toFixed(2) + ' Mbps' : row.bitrate + ' Kbps') : '-'

    return {
      loading, stats, onlineStreams, fmtBytes, fmtTime, load, fmtMbps,
      chartRef, trendRange, changeRange, streamSendMbps, streamBitrateText,
    }
  },
  template: `
    <div v-loading="loading">
      <div class="stat-grid">
        <div class="stat-card">
          <div class="stat-label">流总数 / 在线</div>
          <div class="stat-value"><span class="blue">{{ stats.streamTotal }}</span>
            <span class="text-gray" style="font-size:16px"> / </span>
            <span class="green">{{ stats.streamOnline }}</span></div>
        </div>
        <div class="stat-card">
          <div class="stat-label">当前观看人数</div>
          <div class="stat-value orange">{{ stats.readers }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">入流量速率（推流接收）</div>
          <div class="stat-value blue" style="font-size:24px">{{ fmtMbps(stats.recvBps) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">出流量速率（播放分发）</div>
          <div class="stat-value green" style="font-size:24px">{{ fmtMbps(stats.sendBps) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">累计流量</div>
          <div class="stat-value blue" style="font-size:24px">{{ fmtBytes(stats.totalBytes) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">拉流代理 / 转推 运行</div>
          <div class="stat-value" style="font-size:24px"><span class="green">{{ stats.proxyRunning }}</span><span class="text-gray" style="font-size:15px">/{{ stats.proxyTotal }}</span>
            <span class="text-gray" style="font-size:15px"> · </span>
            <span class="green">{{ stats.forwardRunning }}</span><span class="text-gray" style="font-size:15px">/{{ stats.forwardTotal }}</span></div>
        </div>
      </div>

      <el-card class="page-card" shadow="never">
        <template #header>
          <span>流量趋势</span>
          <el-radio-group :model-value="trendRange" size="small" class="hdr-right" @change="changeRange">
            <el-radio-button :value="60">近 1 小时</el-radio-button>
            <el-radio-button :value="360">近 6 小时</el-radio-button>
            <el-radio-button :value="1440">近 24 小时</el-radio-button>
          </el-radio-group>
        </template>
        <div ref="chartRef" class="chart-box" style="height:300px"></div>
      </el-card>

      <el-card class="page-card" shadow="never">
        <template #header>
          <span>在线流</span>
          <el-button class="hdr-right" text type="primary" size="small" @click="load">刷新</el-button>
        </template>
        <el-table :data="onlineStreams" size="small" v-if="onlineStreams.length">
          <el-table-column prop="name" label="名称" min-width="110" />
          <el-table-column label="流ID" min-width="130">
            <template #default="{ row }"><span class="mono">{{ row.app }}/{{ row.stream }}</span></template>
          </el-table-column>
          <el-table-column label="编码" width="130">
            <template #default="{ row }">{{ row.video_codec || '-' }} / {{ row.audio_codec || '-' }}</template>
          </el-table-column>
          <el-table-column label="分辨率" width="110">
            <template #default="{ row }">{{ row.width ? row.width + 'x' + row.height : '-' }}</template>
          </el-table-column>
          <el-table-column label="码率" width="100">
            <template #default="{ row }">{{ streamBitrateText(row) }}</template>
          </el-table-column>
          <el-table-column label="出流量" width="100">
            <template #default="{ row }">{{ streamSendMbps(row) }}</template>
          </el-table-column>
          <el-table-column prop="reader_count" label="观看" width="70" align="center" />
          <el-table-column label="累计流量" width="100">
            <template #default="{ row }">{{ fmtBytes(row.total_bytes) }}</template>
          </el-table-column>
          <el-table-column label="上线时间" min-width="140">
            <template #default="{ row }">{{ fmtTime(row.publish_time) }}</template>
          </el-table-column>
        </el-table>
        <el-empty v-else description="当前没有在线流" :image-size="80" />
      </el-card>
    </div>
  `,
}
