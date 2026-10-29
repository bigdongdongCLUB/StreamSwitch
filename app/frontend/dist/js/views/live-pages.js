// 直播管理：直播分享页的增/改/删（观众免登录通过 /p/<code> 观看）

import { api } from '../api.js'
import { fmtTime, copyText } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'LivePagesView',
  setup() {
    const loading = ref(false)
    const list = ref([])
    const total = ref(0)
    const query = reactive({ page: 1, page_size: 20, keyword: '' })
    const streams = ref([])

    const load = async () => {
      loading.value = true
      try {
        const d = await api.listLivePages({ ...query })
        list.value = d.list || []
        total.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    const search = () => { query.page = 1; load() }
    const loadStreams = async () => {
      try {
        const d = await api.listStreams({ page: 1, page_size: 200 })
        streams.value = d.list || []
      } catch (e) { /* ignore */ }
    }
    onMounted(() => { load(); loadStreams() })

    // 观众访问链接（当前页面源 + hash 路由）
    const pageURL = (code) => location.origin + '/#/p/' + code
    const copyLink = async (row) => {
      const ok = await copyText(pageURL(row.code))
      ElementPlus.ElMessage[ok ? 'success' : 'error'](ok ? '链接已复制' : '复制失败')
    }

    const streamText = (sid) => {
      const s = streams.value.find(x => x.id === sid)
      return s ? `${s.name} (${s.app}/${s.stream})` : '#' + sid
    }
    const streamOnline = (sid) => {
      const s = streams.value.find(x => x.id === sid)
      return s && s.online === 1
    }

    // ---------- 新建/编辑 ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const blank = { id: 0, name: '', stream_id: 0, password: '', change_pwd: false, announcement: '', status: 1 }
    const form = reactive({ ...blank })
    const rules = {
      name: [{ required: true, message: '请输入页面名称', trigger: 'blur' }],
      stream_id: [{ required: true, message: '请选择关联的流', trigger: 'change' }],
    }

    const openCreate = () => {
      dlgMode.value = 'create'
      Object.assign(form, { ...blank, password: '' })
      dlgVisible.value = true
    }
    const openEdit = (row) => {
      dlgMode.value = 'edit'
      // 密码明文回显，可直接查看/修改/清空（清空=公开访问）
      Object.assign(form, { ...blank, ...row })
      dlgVisible.value = true
    }
    const submitForm = async () => {
      await formRef.value.validate().catch(() => Promise.reject())
      dlgLoading.value = true
      const body = {
        name: form.name, stream_id: form.stream_id,
        password: form.password,
        announcement: form.announcement, status: form.status,
      }
      try {
        if (dlgMode.value === 'create') {
          const d = await api.createLivePage(body)
          ElementPlus.ElMessage.success('创建成功')
          dlgVisible.value = false
          load()
          // 创建成功直接展示链接
          ElementPlus.ElMessageBox.alert(
            `<div class="mono" style="word-break:break-all">${pageURL(d.code)}</div>`,
            '直播页已创建', { dangerouslyUseHTMLString: true, confirmButtonText: '复制链接', showClose: true }
          ).then(() => copyText(pageURL(d.code))).catch(() => {})
        } else {
          await api.updateLivePage(form.id, body)
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

    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除直播页「${row.name}」吗？观众链接将立即失效。`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteLivePage(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    return {
      store, loading, list, total, query, load, search, fmtTime,
      streams, streamText, streamOnline, pageURL, copyLink,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm, remove,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <div class="page-toolbar">
        <el-input v-model="query.keyword" placeholder="页面名称 / 访问码" clearable style="width:220px"
                  @keyup.enter="search" @clear="search" />
        <el-button type="primary" icon="Search" @click="search">查询</el-button>
        <div class="spacer"></div>
        <el-button icon="Refresh" @click="load">刷新</el-button>
        <el-button v-if="store.has('livepage:create')" type="primary" icon="Plus" @click="openCreate">新建直播页</el-button>
      </div>

      <el-alert type="info" :closable="false" style="margin-bottom:12px"
                title="直播页供观众免登录观看：分享链接给对方即可，可设置访问密码（也可不设，公开访问）" />

      <el-table :data="list" v-loading="loading" size="small" border>
        <el-table-column prop="id" label="ID" width="55" />
        <el-table-column prop="name" label="页面名称" min-width="130" show-overflow-tooltip />
        <el-table-column label="访问链接" min-width="200">
          <template #default="{ row }">
            <span class="mono" style="font-size:12px">{{ pageURL(row.code) }}</span>
            <el-button text type="primary" size="small" icon="CopyDocument" @click="copyLink(row)" />
          </template>
        </el-table-column>
        <el-table-column label="关联流" min-width="150">
          <template #default="{ row }">
            <el-tag :type="streamOnline(row.stream_id) ? 'success' : 'info'" size="small" style="margin-right:4px">
              {{ streamOnline(row.stream_id) ? '直播中' : '离线' }}
            </el-tag>
            {{ streamText(row.stream_id) }}
          </template>
        </el-table-column>
        <el-table-column label="访问密码" width="150" align="center">
          <template #default="{ row }">
            <template v-if="row.password">
              <span class="mono">{{ row.password }}</span>
              <el-button text type="primary" size="small" icon="CopyDocument"
                         @click="copyText(row.password).then(ok => ElementPlus.ElMessage[ok?'success':'error'](ok?'密码已复制':'复制失败'))" />
            </template>
            <el-tag v-else type="info" size="small">公开</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="view_count" label="访问量" width="80" align="center" />
        <el-table-column label="状态" width="75" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="145">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button text type="success" size="small" @click="copyLink(row)">复制链接</el-button>
            <el-button v-if="store.has('livepage:update')" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="store.has('livepage:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination background layout="total, prev, pager, next, sizes" :total="total"
                       v-model:current-page="query.page" v-model:page-size="query.page_size"
                       :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="search" />
      </div>

      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新建直播页' : '编辑直播页'" width="520px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
          <el-form-item label="页面名称" prop="name">
            <el-input v-model="form.name" placeholder="如：项目评审直播 / 公开课" />
          </el-form-item>
          <el-form-item label="关联流" prop="stream_id">
            <el-select v-model="form.stream_id" style="width:100%" filterable placeholder="选择要播放的流">
              <el-option v-for="s in streams" :key="s.id"
                         :label="s.name + ' (' + s.app + '/' + s.stream + ')' + (s.online === 1 ? ' ·直播中' : '')"
                         :value="s.id" />
            </el-select>
            <div class="text-gray" style="font-size:12px">制播任务的输出流也在此列表中（标记 [制播]）</div>
          </el-form-item>
          <el-form-item label="访问密码">
            <el-input v-model="form.password" placeholder="留空则公开访问（无需密码）" autocomplete="off" />
            <div class="text-gray" style="font-size:12px">观众打开链接后需输入该密码才能观看；留空=公开。密码在后台明文可见，便于告知观众</div>
          </el-form-item>
          <el-form-item label="公告">
            <el-input v-model="form.announcement" type="textarea" :rows="2" maxlength="200"
                      placeholder="显示在播放页顶部的公告（可选）" />
          </el-form-item>
          <el-form-item label="状态">
            <el-radio-group v-model="form.status">
              <el-radio :value="1">启用</el-radio>
              <el-radio :value="0">停用</el-radio>
            </el-radio-group>
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
