// 日志审计：操作日志 + 任务错误日志

import { api } from '../api.js'
import { store } from '../store.js'
import { fmtTime } from '../utils.js'

const { ref, reactive, onMounted } = Vue

export default {
  name: 'LogsView',
  setup() {
    const tab = ref('op')

    // ---------- 操作日志 ----------
    const opLoading = ref(false)
    const opList = ref([])
    const opTotal = ref(0)
    const opQuery = reactive({ page: 1, page_size: 20, module: '', username: '' })
    const loadOp = async () => {
      opLoading.value = true
      try {
        const d = await api.listLogs({ ...opQuery })
        opList.value = d.list || []
        opTotal.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        opLoading.value = false
      }
    }
    const searchOp = () => { opQuery.page = 1; loadOp() }

    // ---------- 任务错误日志 ----------
    const errLoading = ref(false)
    const errList = ref([])
    const errTotal = ref(0)
    const errQuery = reactive({ page: 1, page_size: 20, task_type: '', task_id: '' })
    const loadErr = async () => {
      errLoading.value = true
      try {
        const d = await api.listTaskErrors({ ...errQuery })
        errList.value = d.list || []
        errTotal.value = d.total
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        errLoading.value = false
      }
    }
    const searchErr = () => { errQuery.page = 1; loadErr() }

    // ---------- 详情 ----------
    const detailVisible = ref(false)
    const detail = ref(null)
    const showDetail = (row) => { detail.value = row; detailVisible.value = true }

    // 与后端 parsePathAction 的路径段（复数）保持一致
    const moduleNames = {
      auth: '认证', streams: '流管理', proxies: '拉流代理', forwards: '转推流',
      domains: '播放域名', formats: '格式开关', 'auth-policy': '鉴权策略', tokens: '播放令牌', encrypts: 'HLS加密',
      users: '用户管理', roles: '角色权限', logs: '日志审计', 'task-errors': '任务错误',
    }

    onMounted(() => { loadOp(); loadErr() })

    return {
      store, tab, fmtTime, moduleNames,
      opLoading, opList, opTotal, opQuery, loadOp, searchOp,
      errLoading, errList, errTotal, errQuery, loadErr, searchErr,
      detailVisible, detail, showDetail,
    }
  },
  template: `
    <el-card class="page-card" shadow="never">
      <el-tabs v-model="tab">
        <el-tab-pane label="操作日志" name="op">
          <div class="page-toolbar">
            <el-select v-model="opQuery.module" placeholder="模块" clearable style="width:140px" @change="searchOp">
              <el-option v-for="(n, c) in moduleNames" :key="c" :label="n" :value="c" />
            </el-select>
            <el-input v-model="opQuery.username" placeholder="操作人" clearable style="width:160px"
                      @keyup.enter="searchOp" @clear="searchOp" />
            <el-button type="primary" icon="Search" @click="searchOp">查询</el-button>
            <div class="spacer"></div>
            <el-button icon="Refresh" @click="loadOp">刷新</el-button>
          </div>

          <el-table :data="opList" v-loading="opLoading" size="small" border>
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column prop="username" label="操作人" width="110" />
            <el-table-column label="模块" width="100">
              <template #default="{ row }">{{ moduleNames[row.module] || row.module }}</template>
            </el-table-column>
            <el-table-column prop="action" label="动作" width="110" />
            <el-table-column label="请求" min-width="220" show-overflow-tooltip>
              <template #default="{ row }"><span class="mono">{{ row.method }} {{ row.path }}</span></template>
            </el-table-column>
            <el-table-column prop="ip" label="IP" width="130">
              <template #default="{ row }"><span class="mono">{{ row.ip }}</span></template>
            </el-table-column>
            <el-table-column label="结果" width="80" align="center">
              <template #default="{ row }">
                <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? '成功' : '失败' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="耗时" width="90" align="center">
              <template #default="{ row }">{{ row.duration }} ms</template>
            </el-table-column>
            <el-table-column label="时间" min-width="150">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="80" fixed="right" class-name="col-actions">
              <template #default="{ row }">
                <el-button text type="primary" size="small" @click="showDetail(row)">详情</el-button>
              </template>
            </el-table-column>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next, sizes" :total="opTotal"
                           v-model:current-page="opQuery.page" v-model:page-size="opQuery.page_size"
                           :page-sizes="[10, 20, 50, 100]" @current-change="loadOp" @size-change="searchOp" />
          </div>
        </el-tab-pane>

        <el-tab-pane label="任务错误日志" name="err">
          <div class="page-toolbar">
            <el-select v-model="errQuery.task_type" placeholder="任务类型" clearable style="width:140px" @change="searchErr">
              <el-option label="拉流代理" value="proxy" />
              <el-option label="转推流" value="forward" />
            </el-select>
            <el-input v-model="errQuery.task_id" placeholder="任务ID" clearable style="width:140px"
                      @keyup.enter="searchErr" @clear="searchErr" />
            <el-button type="primary" icon="Search" @click="searchErr">查询</el-button>
            <div class="spacer"></div>
            <el-button icon="Refresh" @click="loadErr">刷新</el-button>
          </div>

          <el-table :data="errList" v-loading="errLoading" size="small" border>
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column label="任务类型" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="row.task_type === 'proxy' ? 'primary' : 'warning'">
                  {{ row.task_type === 'proxy' ? '拉流代理' : '转推流' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="task_id" label="任务ID" width="80" align="center" />
            <el-table-column prop="task_name" label="任务名称" width="140" show-overflow-tooltip />
            <el-table-column label="级别" width="80" align="center">
              <template #default="{ row }">
                <el-tag size="small" :type="{ error: 'danger', warn: 'warning', info: 'info' }[row.level] || 'info'">
                  {{ row.level }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="message" label="错误信息" min-width="300" show-overflow-tooltip>
              <template #default="{ row }"><span class="mono">{{ row.message }}</span></template>
            </el-table-column>
            <el-table-column label="时间" min-width="150">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next, sizes" :total="errTotal"
                           v-model:current-page="errQuery.page" v-model:page-size="errQuery.page_size"
                           :page-sizes="[10, 20, 50, 100]" @current-change="loadErr" @size-change="searchErr" />
          </div>
        </el-tab-pane>
      </el-tabs>

      <!-- 日志详情 -->
      <el-dialog v-model="detailVisible" title="日志详情" width="640px">
        <template v-if="detail">
          <el-descriptions :column="store.isMobile ? 1 : 2" border size="small">
            <el-descriptions-item label="操作人">{{ detail.username }}</el-descriptions-item>
            <el-descriptions-item label="模块">{{ moduleNames[detail.module] || detail.module }}</el-descriptions-item>
            <el-descriptions-item label="动作">{{ detail.action }}</el-descriptions-item>
            <el-descriptions-item label="结果">
              <el-tag :type="detail.status === 1 ? 'success' : 'danger'" size="small">{{ detail.status === 1 ? '成功' : '失败' }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="请求"><span class="mono">{{ detail.method }} {{ detail.path }}</span></el-descriptions-item>
            <el-descriptions-item label="IP"><span class="mono">{{ detail.ip }}</span></el-descriptions-item>
            <el-descriptions-item label="耗时">{{ detail.duration }} ms</el-descriptions-item>
            <el-descriptions-item label="时间">{{ fmtTime(detail.created_at) }}</el-descriptions-item>
          </el-descriptions>
          <el-alert v-if="detail.error_msg" type="error" :closable="false" style="margin-top:12px" :title="detail.error_msg" />
          <div style="margin-top:12px">
            <div class="text-gray" style="font-size:12px;margin-bottom:4px">请求参数</div>
            <pre class="mono" style="background:#f5f7fa;padding:10px;border-radius:6px;white-space:pre-wrap;margin:0">{{ detail.params || '无' }}</pre>
          </div>
        </template>
      </el-dialog>
    </el-card>
  `,
}
