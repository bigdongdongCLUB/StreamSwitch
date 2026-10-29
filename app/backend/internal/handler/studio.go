package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"livems/internal/middleware"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// uploadMaxDuration 单次素材上传请求（接收 + 落盘 + 探测）允许的最长时间。
const uploadMaxDuration = 2 * time.Hour

// StudioHandler 制播管理接口。
type StudioHandler struct {
	studioSvc *service.StudioService
}

// NewStudioHandler 创建制播处理器。
func NewStudioHandler(studioSvc *service.StudioService) *StudioHandler {
	return &StudioHandler{studioSvc: studioSvc}
}

// ---------------------------------------------------------------------------
// 素材库
// ---------------------------------------------------------------------------

// ListMaterials 处理素材列表查询。
//
// GET /api/v1/studio/materials?type=video|logo
func (h *StudioHandler) ListMaterials(c *gin.Context) {
	list, err := h.studioSvc.ListMaterials(c.Query("type"))
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// UploadMaterial 处理素材上传（multipart/form-data，字段：file + type）。
//
// POST /api/v1/studio/materials
func (h *StudioHandler) UploadMaterial(c *gin.Context) {
	// 大文件上传：解除 http.Server 的 WriteTimeout 对本请求的限制（接收 + 落盘 + ffprobe 可能远超 30s），
	// 改为本请求独立的宽松期限。
	rc := http.NewResponseController(c.Writer)
	deadline := time.Now().Add(uploadMaxDuration)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)

	// 不使用 c.FormFile / ParseMultipartForm：标准库会先把文件 part 吞进内存缓冲区（反复扩容+GC），
	// 再落临时文件、再由业务层拷贝一次，数百 MB 的视频会把 CPU 烧满而吞吐只有几 MB/s。
	// 这里直接迭代 multipart part，把文件流式写入最终位置，零内存缓冲、单次磁盘写入。
	mr, err := c.Request.MultipartReader()
	if err != nil {
		response.BadRequest(c, "请求必须为 multipart/form-data（字段：file + type）")
		return
	}
	mtype := c.Query("type") // 允许通过查询参数预先指定类型（file 字段先于 type 字段到达时仍可判定）
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			response.BadRequest(c, "缺少上传文件（字段名 file）")
			return
		}
		if err != nil {
			response.BadRequest(c, "解析上传内容失败: "+err.Error())
			return
		}
		switch part.FormName() {
		case "type":
			v, _ := io.ReadAll(io.LimitReader(part, 64))
			mtype = strings.TrimSpace(string(v))
		case "file":
			if mtype == "" {
				mtype = service.InferMaterialType(part.FileName())
			}
			m, err := h.studioSvc.SaveMaterial(part.FileName(), part, mtype, middleware.CurrentUserID(c))
			if err != nil {
				response.BadRequest(c, err.Error())
				return
			}
			response.OKMsg(c, "上传成功", m)
			return
		}
		_ = part.Close()
	}
}

// DeleteMaterial 处理素材删除。
//
// DELETE /api/v1/studio/materials/:id
func (h *StudioHandler) DeleteMaterial(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的素材ID")
		return
	}
	if err := h.studioSvc.DeleteMaterial(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// ---------------------------------------------------------------------------
// 制播任务
// ---------------------------------------------------------------------------

// ListTasks 处理任务列表查询。
//
// GET /api/v1/studio/tasks
func (h *StudioHandler) ListTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	list, total, err := h.studioSvc.ListTasks(page, pageSize, keyword)
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// CreateTask 处理任务创建。
//
// POST /api/v1/studio/tasks
func (h *StudioHandler) CreateTask(c *gin.Context) {
	var req service.StudioTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	t, err := h.studioSvc.CreateTask(&req, middleware.CurrentUserID(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 创建后若启用则自动启动
	if t.Status == 1 {
		if err := h.studioSvc.StartTask(t.ID); err != nil {
			response.OKMsg(c, "创建成功，但启动失败: "+err.Error(), t)
			return
		}
	}
	response.OKMsg(c, "创建成功", t)
}

// UpdateTask 处理任务更新。
//
// PUT /api/v1/studio/tasks/:id
func (h *StudioHandler) UpdateTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的任务ID")
		return
	}
	var req service.StudioTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	t, err := h.studioSvc.UpdateTask(id, &req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 配置变更后若任务在运行，重启使新配置生效
	if t.RunStatus == "running" {
		h.studioSvc.StopTask(id)
		if err := h.studioSvc.StartTask(id); err != nil {
			response.OKMsg(c, "更新成功，但重启失败: "+err.Error(), t)
			return
		}
	}
	response.OKMsg(c, "更新成功", t)
}

// StartTask 处理任务启动。
//
// POST /api/v1/studio/tasks/:id/start
func (h *StudioHandler) StartTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的任务ID")
		return
	}
	if err := h.studioSvc.StartTask(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "任务已启动", nil)
}

// StopTask 处理任务停止。
//
// POST /api/v1/studio/tasks/:id/stop
func (h *StudioHandler) StopTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的任务ID")
		return
	}
	h.studioSvc.StopTask(id)
	response.OKMsg(c, "任务已停止", nil)
}

// ToggleTask 处理任务启停切换。
//
// POST /api/v1/studio/tasks/:id/toggle
func (h *StudioHandler) ToggleTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的任务ID")
		return
	}
	status, err := h.studioSvc.ToggleTask(id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg := "任务已启动"
	if status == "stopped" {
		msg = "任务已停止"
	}
	response.OKMsg(c, msg, gin.H{"run_status": status})
}

// DeleteTask 处理任务删除。
//
// DELETE /api/v1/studio/tasks/:id
func (h *StudioHandler) DeleteTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的任务ID")
		return
	}
	if err := h.studioSvc.DeleteTask(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}
