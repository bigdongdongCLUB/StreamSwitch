// 域名设置：推流域名 + 播放域名统一管理

import { api } from '../api.js'
import { fmtTime } from '../utils.js'
import { store } from '../store.js'

const { ref, reactive, computed, onMounted } = Vue

export default {
  name: 'DomainsView',
  setup() {
    const canUpdate = computed(() => store.has('domain:update'))

    // ================= 推流域名设置 =================
    const ingressLoading = ref(false)
    const ingressSaving = ref(false)
    const ingress = reactive({
      push_schema: 'rtmp',
      push_host: '',
      push_port: 1935,
      rtsp_port: 0,
      follow_play_domain: false,
    })
    const loadIngress = async () => {
      ingressLoading.value = true
      try {
        const d = await api.getIngress()
        if (d) Object.assign(ingress, d)
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        ingressLoading.value = false
      }
    }
    const saveIngress = async () => {
      ingressSaving.value = true
      try {
        await api.saveIngress({ ...ingress })
        ElementPlus.ElMessage.success('推流域名设置已保存并实时生效')
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        ingressSaving.value = false
      }
    }

    // ================= 播放域名 =================
    const loading = ref(false)
    const list = ref([])

    const load = async () => {
      loading.value = true
      try {
        list.value = await api.listDomains() || []
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        loading.value = false
      }
    }
    onMounted(() => { load(); loadIngress(); loadCerts() })

    // ================= SSL 证书 =================
    const certsLoading = ref(false)
    const certs = ref([])
    const loadCerts = async () => {
      certsLoading.value = true
      try {
        certs.value = await api.listCerts() || []
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        certsLoading.value = false
      }
    }

    const certDaysType = (d) => d < 0 ? 'danger' : (d < 7 ? 'danger' : (d < 30 ? 'warning' : 'success'))
    const certDaysText = (d) => d < 0 ? `已过期 ${-d} 天` : `剩余 ${d} 天`

    // 上传/更新证书
    const certDlgVisible = ref(false)
    const certSaving = ref(false)
    const certForm = reactive({ name: '', fullchain: '', privkey: '', apply: true })
    const openCertDlg = (row) => {
      certForm.name = row ? row.name : ''
      certForm.fullchain = ''
      certForm.privkey = ''
      certForm.apply = true
      certDlgVisible.value = true
    }
    // 从本地文件读取证书/私钥：支持一次选多个文件（如 fullchain.pem + privkey.pem），
    // 按 PEM 块类型自动归位：CERTIFICATE → 证书框，*PRIVATE KEY → 私钥框；
    // 若文件内同时含证书与私钥（常见的合并 .pem），也会拆开分别填入。
    const certFileInput = ref(null)
    const pickCertFiles = () => certFileInput.value && certFileInput.value.click()
    const PEM_BLOCK_RE = /-----BEGIN ([A-Z0-9 ]+)-----[\s\S]*?-----END \1-----/g
    const onCertFilesPicked = async (e) => {
      const files = Array.from(e.target.files || [])
      e.target.value = ''
      if (!files.length) return
      let certs = [], keys = [], unknown = []
      for (const f of files) {
        if (f.size > 1024 * 1024) { unknown.push(`${f.name}（超过 1MB，不像 PEM 文件）`); continue }
        const text = await f.text()
        const blocks = text.match(PEM_BLOCK_RE) || []
        if (!blocks.length) { unknown.push(`${f.name}（未找到 PEM 块，若是 .pfx/.der 等二进制格式请先转换为 PEM）`); continue }
        for (const b of blocks) {
          const type = b.match(/-----BEGIN ([A-Z0-9 ]+)-----/)[1]
          if (type === 'CERTIFICATE') certs.push(b)
          else if (type.endsWith('PRIVATE KEY')) keys.push(b)
        }
        // 用第一个文件名猜证书名称（去掉扩展名与常见前缀）
        if (!certForm.name) {
          const base = f.name.replace(/\.(pem|crt|cer|key|txt)$/i, '').replace(/^(fullchain|privkey|cert|certificate|chain|key)[._-]?/i, '')
          if (/^[A-Za-z0-9][A-Za-z0-9._*-]{0,127}$/.test(base) && base.includes('.')) certForm.name = base
        }
      }
      if (certs.length) certForm.fullchain = certs.join('\n') + '\n'
      if (keys.length) certForm.privkey = keys[0] + '\n'
      const parts = []
      if (certs.length) parts.push(`证书 ${certs.length} 段`)
      if (keys.length) parts.push('私钥 1 段')
      if (parts.length) ElementPlus.ElMessage.success('已读取：' + parts.join('，') + (certs.length && !keys.length ? '；请再选择私钥文件' : ''))
      if (unknown.length) ElementPlus.ElMessage.warning('无法识别：' + unknown.join('；'))
      if (!parts.length && !unknown.length) ElementPlus.ElMessage.warning('所选文件中没有证书或私钥')
    }

    const submitCert = async () => {
      if (!certForm.name || !certForm.fullchain || !certForm.privkey) {
        ElementPlus.ElMessage.warning('请完整填写证书名称、证书内容与私钥')
        return
      }
      certSaving.value = true
      try {
        await api.saveCert({ ...certForm })
        ElementPlus.ElMessage.success(certForm.apply ? '证书已保存并应用到站点' : '证书已保存')
        certDlgVisible.value = false
        loadCerts()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        certSaving.value = false
      }
    }
    const applyCert = (row) => {
      ElementPlus.ElMessageBox.confirm(
        `将把站点证书切换为「${row.name}」（覆盖域名：${(row.domains || []).join('、') || '未知'}），Nginx 会自动校验，失败自动回滚。确定应用吗？`,
        '应用证书', { type: 'warning' }
      ).then(async () => {
        try {
          await api.applyCert(row.name)
          ElementPlus.ElMessage.success('证书已应用并重载 Nginx')
          loadCerts()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }
    const removeCert = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除证书「${row.name}」吗？`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteCert(row.name)
          ElementPlus.ElMessage.success('删除成功')
          loadCerts()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    const defaultDomain = computed(() => list.value.find(d => d.is_default === 1) || null)

    // 推流地址预览
    const effectivePushHost = computed(() => {
      if (ingress.follow_play_domain && defaultDomain.value) return defaultDomain.value.host
      if (ingress.push_host) return ingress.push_host
      if (defaultDomain.value) return defaultDomain.value.host
      return '127.0.0.1'
    })
    const previewURL = computed(() => {
      const defPort = ingress.push_schema === 'rtmps' ? 443 : 1935
      const portPart = (ingress.push_port > 0 && ingress.push_port !== defPort) ? `:${ingress.push_port}` : ''
      return `${ingress.push_schema}://${effectivePushHost.value}${portPart}/live/<流ID>?key=<推流Key>`
    })

    // ---------- 播放域名 CRUD ----------
    const dlgVisible = ref(false)
    const dlgMode = ref('create')
    const dlgLoading = ref(false)
    const formRef = ref(null)
    const blank = { id: 0, name: '', schema: 'http', host: '', port: 80, is_default: 0, status: 1, sort: 0, remark: '' }
    const form = reactive({ ...blank })
    const rules = {
      name: [{ required: true, message: '请输入域名名称', trigger: 'blur' }],
      host: [{ required: true, message: '请输入主机名', trigger: 'blur' }],
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
        name: form.name, schema: form.schema, host: form.host, port: form.port,
        is_default: form.is_default, status: form.status, sort: form.sort, remark: form.remark,
      }
      try {
        if (dlgMode.value === 'create') await api.createDomain(body)
        else await api.updateDomain(form.id, body)
        ElementPlus.ElMessage.success(dlgMode.value === 'create' ? '创建成功' : '更新成功')
        dlgVisible.value = false
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        dlgLoading.value = false
      }
    }

    const setDefault = async (row) => {
      try {
        await api.setDefaultDomain(row.id)
        ElementPlus.ElMessage.success('已设为默认域名')
        load()
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      }
    }
    const remove = (row) => {
      ElementPlus.ElMessageBox.confirm(`确定删除域名「${row.name}」(${row.host}) 吗？`, '警告', {
        type: 'warning', confirmButtonText: '删除', confirmButtonClass: 'el-button--danger',
      }).then(async () => {
        try {
          await api.deleteDomain(row.id)
          ElementPlus.ElMessage.success('删除成功')
          load()
        } catch (e) {
          ElementPlus.ElMessage.error(e.message)
        }
      }).catch(() => {})
    }

    return {
      store, canUpdate, fmtTime,
      ingressLoading, ingressSaving, ingress, saveIngress, effectivePushHost, previewURL,
      loading, list, load, defaultDomain,
      dlgVisible, dlgMode, dlgLoading, formRef, form, rules, openCreate, openEdit, submitForm,
      setDefault, remove,
      certsLoading, certs, loadCerts, certDaysType, certDaysText,
      certDlgVisible, certSaving, certForm, openCertDlg, submitCert, applyCert, removeCert,
      certFileInput, pickCertFiles, onCertFilesPicked,
    }
  },
  template: `
    <div>
      <!-- 推流域名设置 -->
      <el-card class="page-card" shadow="never" v-loading="ingressLoading">
        <template #header>
          <span>推流域名设置</span>
          <span class="text-gray" style="font-size:12px;margin-left:10px">推流走 RTMP 直连（1935 端口），不经过 Nginx；保存后实时生效</span>
        </template>

        <el-form label-width="170px" style="max-width:720px">
          <el-form-item label="推流协议">
            <el-radio-group v-model="ingress.push_schema" :disabled="!canUpdate">
              <el-radio value="rtmp">RTMP</el-radio>
              <el-radio value="rtmps">RTMPS</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="跟随默认播放域名">
            <el-switch v-model="ingress.follow_play_domain" :disabled="!canUpdate" />
            <div class="text-gray" style="font-size:12px">
              开启后推流地址使用下方默认播放域名
              <template v-if="defaultDomain">（当前：<span class="mono">{{ defaultDomain.host }}</span>）</template>
              <template v-else>（尚未配置默认播放域名）</template>
            </div>
          </el-form-item>
          <el-form-item label="推流域名" v-if="!ingress.follow_play_domain">
            <el-input v-model="ingress.push_host" placeholder="如 live.example.com" class="mono"
                      style="width:300px" :disabled="!canUpdate" />
            <div class="text-gray" style="font-size:12px">只填域名或 IP，不要带协议、端口或路径；留空则使用默认播放域名</div>
          </el-form-item>
          <el-form-item :label="ingress.push_schema === 'rtmps' ? '对外 RTMPS 端口' : '对外 RTMP 端口'">
            <el-input-number controls-position="right" v-model="ingress.push_port" :min="0" :max="65535" style="width:180px" :disabled="!canUpdate" />
            <div class="text-gray" style="font-size:12px">
              公网映射后的端口；{{ ingress.push_schema === 'rtmps' ? '443' : '1935' }} 为协议默认端口（地址中省略），0 表示使用默认
            </div>
          </el-form-item>
          <el-form-item label="对外 RTSP 端口">
            <el-input-number controls-position="right" v-model="ingress.rtsp_port" :min="0" :max="65535" style="width:180px" :disabled="!canUpdate" />
            <div class="text-gray" style="font-size:12px">0 表示不对外提供 RTSP 播放</div>
          </el-form-item>
          <el-form-item label="推流地址预览">
            <el-input :model-value="previewURL" readonly class="mono" />
          </el-form-item>
          <el-form-item v-if="canUpdate">
            <el-button type="primary" :loading="ingressSaving" @click="saveIngress">保存推流设置</el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <!-- 播放域名 -->
      <el-card class="page-card" shadow="never">
        <template #header>
          <span>播放域名</span>
          <span class="text-gray" style="font-size:12px;margin-left:10px">播放走 HTTP/HTTPS（经 Nginx 反代 ZLM），可配置多个并指定默认域名</span>
        </template>
        <div class="page-toolbar">
          <span class="text-gray">HTTPS 非 443、HTTP 非 80 时务必填写公网映射后的实际端口</span>
          <div class="spacer"></div>
          <el-button icon="Refresh" @click="load">刷新</el-button>
          <el-button v-if="store.has('domain:create')" type="primary" icon="Plus" @click="openCreate">新增域名</el-button>
        </div>

        <el-table :data="list" v-loading="loading" size="small" border>
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="name" label="名称" min-width="120" />
          <el-table-column label="地址" min-width="220">
            <template #default="{ row }">
              <span class="mono">{{ row.schema }}://{{ row.host }}<span v-if="![80,443].includes(row.port)">:{{ row.port }}</span></span>
            </template>
          </el-table-column>
          <el-table-column label="默认" width="90" align="center">
            <template #default="{ row }">
              <el-tag v-if="row.is_default === 1" type="success" size="small">默认</el-tag>
              <el-button v-else-if="canUpdate" text type="primary" size="small" @click="setDefault(row)">设为默认</el-button>
              <span v-else class="text-gray">-</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80" align="center">
            <template #default="{ row }">
              <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="sort" label="排序" width="70" align="center" />
          <el-table-column label="创建时间" min-width="150">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right" class-name="col-actions">
            <template #default="{ row }">
              <el-button v-if="canUpdate" text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
              <el-button v-if="store.has('domain:delete')" text type="danger" size="small" @click="remove(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <!-- SSL 证书 -->
      <el-card class="page-card" shadow="never">
        <template #header>
          <span>SSL 证书</span>
          <span class="text-gray" style="font-size:12px;margin-left:10px">部署到本站点 Nginx，应用前自动校验，失败自动回滚</span>
        </template>
        <div class="page-toolbar">
          <span class="text-gray">证书需包含完整证书链（fullchain）与私钥（privkey）；当前站点使用证书请在此处上传并应用</span>
          <div class="spacer"></div>
          <el-button icon="Refresh" @click="loadCerts">刷新</el-button>
          <el-button v-if="canUpdate" type="primary" icon="Upload" @click="openCertDlg(null)">上传证书</el-button>
        </div>

        <el-table :data="certs" v-loading="certsLoading" size="small" border>
          <el-table-column prop="name" label="证书名称" min-width="150">
            <template #default="{ row }"><span class="mono">{{ row.name }}</span></template>
          </el-table-column>
          <el-table-column label="覆盖域名" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ (row.domains || []).join('、') || '-' }}</template>
          </el-table-column>
          <el-table-column prop="issuer" label="颁发者" min-width="160" show-overflow-tooltip />
          <el-table-column label="到期时间" min-width="150">
            <template #default="{ row }">{{ row.not_after ? fmtTime(row.not_after) : '-' }}</template>
          </el-table-column>
          <el-table-column label="剩余" width="110" align="center">
            <template #default="{ row }">
              <el-tag v-if="row.not_after" :type="certDaysType(row.days_left)" size="small">{{ certDaysText(row.days_left) }}</el-tag>
              <span v-else class="text-gray">-</span>
            </template>
          </el-table-column>
          <el-table-column label="使用中" width="80" align="center">
            <template #default="{ row }">
              <el-tag v-if="row.in_use" type="success" size="small">使用中</el-tag>
              <span v-else class="text-gray">-</span>
            </template>
          </el-table-column>
          <el-table-column label="私钥" width="70" align="center">
            <template #default="{ row }">
              <el-icon v-if="row.has_key" class="text-success"><CircleCheck /></el-icon>
              <el-icon v-else class="text-danger"><CircleClose /></el-icon>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="170" fixed="right" class-name="col-actions">
            <template #default="{ row }">
              <template v-if="canUpdate">
                <el-button v-if="!row.in_use" text type="success" size="small" @click="applyCert(row)">应用</el-button>
                <el-button text type="primary" size="small" @click="openCertDlg(row)">更新</el-button>
                <!-- 使用中的证书不可删除（删除会导致 HTTPS 中断），置灰并提示原因 -->
                <el-tooltip :disabled="!row.in_use" content="该证书正在被站点使用，请先应用其它证书后再删除" placement="top">
                  <span>
                    <el-button text type="danger" size="small" :disabled="row.in_use" @click="removeCert(row)">删除</el-button>
                  </span>
                </el-tooltip>
              </template>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <!-- 端口映射参考 -->
      <el-card class="page-card" shadow="never">
        <template #header><span>端口映射参考</span></template>
        <el-table :data="[
          { port: '51443 TCP', usage: 'HTTPS 主入口（后台 + API + FLV/HLS/WS-FLV 播放），内外网同端口', required: '必须' },
          { port: '51880 TCP', usage: 'HTTP 入口（内外网同端口）', required: '建议' },
          { port: '1935 TCP', usage: 'RTMP 推流 / RTMP 播放', required: '需要公网推流时必须' },
          { port: '554 TCP', usage: 'RTSP 播放', required: '按需' },
        ]" size="small" border>
          <el-table-column prop="port" label="内网端口" width="120"><template #default="{ row }"><span class="mono">{{ row.port }}</span></template></el-table-column>
          <el-table-column prop="usage" label="用途" min-width="280" />
          <el-table-column prop="required" label="映射建议" width="170" />
        </el-table>
        <el-alert type="warning" :closable="false" style="margin-top:12px"
                  title="8080（ZLM HTTP）、8090（livems API）、3306（MySQL）、6379（Redis）仅供内网，严禁映射到公网" />
      </el-card>

      <!-- 证书上传对话框 -->
      <el-dialog v-model="certDlgVisible" title="上传 / 更新 SSL 证书" width="640px">
        <el-form label-width="110px">
          <el-form-item label="证书名称" required>
            <el-input v-model="certForm.name" placeholder="通常为域名，如 live.example.com" class="mono" style="width:300px" />
            <div class="text-gray" style="font-size:12px">同名证书会被覆盖更新</div>
          </el-form-item>
          <el-form-item label="证书文件">
            <el-button icon="FolderOpened" @click="pickCertFiles">选择文件</el-button>
            <input ref="certFileInput" type="file" multiple style="display:none"
                   accept=".pem,.crt,.cer,.key,.txt" @change="onCertFilesPicked" />
            <div class="text-gray" style="font-size:12px;line-height:1.6">
              可一次选中 fullchain.pem 与 privkey.pem（或 .crt + .key），内容会自动填入下方对应文本框；
              也可继续手动粘贴。仅支持 PEM 文本格式，.pfx/.p12/.der 请先转换。
            </div>
          </el-form-item>
          <el-form-item label="证书 (PEM)" required>
            <el-input v-model="certForm.fullchain" type="textarea" :rows="7" class="mono"
                      placeholder="-----BEGIN CERTIFICATE-----&#10;完整证书链（fullchain.pem 内容）&#10;-----END CERTIFICATE-----" />
          </el-form-item>
          <el-form-item label="私钥 (PEM)" required>
            <el-input v-model="certForm.privkey" type="textarea" :rows="6" class="mono"
                      placeholder="-----BEGIN PRIVATE KEY-----&#10;私钥（privkey.pem 内容）&#10;-----END PRIVATE KEY-----" />
          </el-form-item>
          <el-form-item label="立即应用">
            <el-switch v-model="certForm.apply" />
            <div class="text-gray" style="font-size:12px">开启后保存即切换站点证书（自动 nginx -t 校验，失败自动回滚）</div>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="certDlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="certSaving" @click="submitCert">保存</el-button>
        </template>
      </el-dialog>

      <!-- 域名编辑对话框 -->
      <el-dialog v-model="dlgVisible" :title="dlgMode === 'create' ? '新增域名' : '编辑域名'" width="500px">
        <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
          <el-form-item label="名称" prop="name">
            <el-input v-model="form.name" placeholder="如：主站播放域名" />
          </el-form-item>
          <el-form-item label="协议">
            <el-radio-group v-model="form.schema" @change="form.port = form.schema === 'https' ? 443 : 80">
              <el-radio value="http">HTTP</el-radio>
              <el-radio value="https">HTTPS</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="主机名" prop="host">
            <el-input v-model="form.host" placeholder="live.example.com" class="mono" />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number controls-position="right" v-model="form.port" :min="1" :max="65535" style="width:160px" />
          </el-form-item>
          <el-form-item label="设为默认">
            <el-switch :model-value="form.is_default === 1" @change="v => form.is_default = v ? 1 : 0" />
          </el-form-item>
          <el-form-item label="状态">
            <el-radio-group v-model="form.status">
              <el-radio :value="1">启用</el-radio>
              <el-radio :value="0">禁用</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="排序">
            <el-input-number controls-position="right" v-model="form.sort" :min="0" style="width:160px" />
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="form.remark" type="textarea" :rows="2" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlgVisible = false">取消</el-button>
          <el-button type="primary" :loading="dlgLoading" @click="submitForm">确定</el-button>
        </template>
      </el-dialog>
    </div>
  `,
}
