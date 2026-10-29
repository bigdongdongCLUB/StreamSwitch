package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/models"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
)

// StudioService 制播管理服务：素材库管理 + 制播任务 CRUD + ffmpeg 进程编排。
type StudioService struct {
	cfg         *config.Config
	materialRepo *repository.StudioMaterialRepo
	taskRepo    *repository.StudioTaskRepo
	streamRepo  *repository.StreamRepo
}

// NewStudioService 创建制播服务。
func NewStudioService(cfg *config.Config) *StudioService {
	return &StudioService{
		cfg:          cfg,
		materialRepo: repository.NewStudioMaterialRepo(),
		taskRepo:     repository.NewStudioTaskRepo(),
		streamRepo:   repository.NewStreamRepo(),
	}
}

// ---------------------------------------------------------------------------
// 素材库
// ---------------------------------------------------------------------------

// 允许的素材扩展名
var (
	videoExts = map[string]bool{".mp4": true, ".flv": true, ".mkv": true, ".mov": true, ".ts": true, ".avi": true, ".webm": true}
	logoExts  = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}
)

// ffprobeResult ffprobe 输出的关键字段。
type ffprobeResult struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

// SaveMaterial 保存上传的素材文件并解析媒体信息。
func (s *StudioService) SaveMaterial(filename string, src io.Reader, mtype string, userID uint64) (*models.StudioMaterial, error) {
	if mtype != "video" && mtype != "logo" {
		return nil, fmt.Errorf("素材类型只支持 video 或 logo")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if mtype == "video" && !videoExts[ext] {
		return nil, fmt.Errorf("不支持的视频格式 %s（支持 mp4/flv/mkv/mov/ts/avi/webm）", ext)
	}
	if mtype == "logo" && !logoExts[ext] {
		return nil, fmt.Errorf("不支持的图片格式 %s（支持 png/jpg/jpeg/webp/gif）", ext)
	}

	sub := "videos"
	if mtype == "logo" {
		sub = "logos"
	}
	dir := filepath.Join(s.cfg.Studio.DataDir, sub)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建素材目录失败: %w", err)
	}
	// 文件名：时间戳 + 随机后缀 + 原扩展名（避免中文/特殊字符问题）
	relPath := filepath.ToSlash(filepath.Join(sub, fmt.Sprintf("%d_%s%s", time.Now().Unix(), randHex(4), ext)))
	absPath := filepath.Join(s.cfg.Studio.DataDir, relPath)

	dst, err := os.Create(absPath)
	if err != nil {
		return nil, fmt.Errorf("写入素材文件失败: %w", err)
	}
	size, err := copyWithLimit(dst, src, 4<<30) // 最大 4GB，流式写入
	_ = dst.Close()
	if err != nil {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("保存素材失败: %w", err)
	}

	m := &models.StudioMaterial{
		Name:      strings.TrimSuffix(filepath.Base(filename), ext),
		Type:      mtype,
		FilePath:  relPath,
		Size:      size,
		CreatedBy: userID,
	}
	// ffprobe 解析视频信息（logo 也尝试取宽高）
	if info, err := s.probe(absPath); err == nil {
		if d, err := strconv.ParseFloat(info.Format.Duration, 64); err == nil {
			m.Duration = int(d + 0.5)
		}
		for _, st := range info.Streams {
			if st.CodecType == "video" {
				m.Width, m.Height = st.Width, st.Height
				break
			}
		}
	}
	if err := s.materialRepo.Create(m); err != nil {
		_ = os.Remove(absPath)
		return nil, err
	}
	logger.Info("素材已上传: %s (%s, %.1f MB)", m.Name, relPath, float64(size)/1048576)
	return m, nil
}

// probe 调用 ffprobe 解析媒体信息。
func (s *StudioService) probe(absPath string) (*ffprobeResult, error) {
	out, err := exec.Command(s.cfg.Studio.FFprobePath,
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", absPath,
	).Output()
	if err != nil {
		return nil, err
	}
	var r ffprobeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListMaterials 素材列表。
func (s *StudioService) ListMaterials(mtype string) ([]models.StudioMaterial, error) {
	return s.materialRepo.List(mtype)
}

// DeleteMaterial 删除素材（同时删除文件；被任务引用的素材拒绝删除）。
func (s *StudioService) DeleteMaterial(id uint64) error {
	m, err := s.materialRepo.GetByID(id)
	if err != nil {
		return err
	}
	// 引用检查
	tasks, _, err := s.taskRepo.List(1, 200, "")
	if err == nil {
		for _, t := range tasks {
			if t.MaterialID == id || t.LogoMaterialID == id {
				return fmt.Errorf("素材正被任务「%s」使用，请先修改或删除该任务", t.Name)
			}
		}
	}
	if err := s.materialRepo.Delete(id); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.cfg.Studio.DataDir, m.FilePath))
	return nil
}

// ---------------------------------------------------------------------------
// 制播任务 CRUD
// ---------------------------------------------------------------------------

// StudioTaskRequest 任务创建/更新请求。
type StudioTaskRequest struct {
	Name            string `json:"name" binding:"required"`
	Mode            string `json:"mode"` // loop / relay
	MaterialID      uint64 `json:"material_id"`
	SrcURL          string `json:"src_url"`
	App             string `json:"app"`
	Stream          string `json:"stream" binding:"required"`
	CaptionEnabled  bool   `json:"caption_enabled"`
	CaptionText     string `json:"caption_text"`
	CaptionSize     int    `json:"caption_size"`
	CaptionColor    string `json:"caption_color"`
	CaptionPosition string `json:"caption_position"`
	CaptionSpeed    int    `json:"caption_speed"`
	LogoMaterialID  uint64 `json:"logo_material_id"`
	LogoPosition    string `json:"logo_position"`
	LogoOpacity     int    `json:"logo_opacity"`
	LogoWidthPct    int    `json:"logo_width_pct"`
	VBitrate        int    `json:"vbitrate"`
	QualityMode     string `json:"quality_mode"` // bitrate=固定码率 crf=画质优先
	CRF             int    `json:"crf"`
	Status          int    `json:"status"`
	RetryCount      int    `json:"retry_count"`
}

// normalizeTask 校验并规整任务参数。
func (s *StudioService) normalizeTask(req *StudioTaskRequest, t *models.StudioTask) error {
	if req.Mode != "loop" && req.Mode != "relay" {
		return fmt.Errorf("任务模式只支持 loop（视频轮播）或 relay（拉流压制）")
	}
	if req.Mode == "loop" {
		if req.MaterialID == 0 {
			return fmt.Errorf("视频轮播任务必须选择素材")
		}
		m, err := s.materialRepo.GetByID(req.MaterialID)
		if err != nil {
			return err
		}
		if m.Type != "video" {
			return fmt.Errorf("素材类型不是视频")
		}
	} else {
		src := strings.TrimSpace(req.SrcURL)
		if src == "" {
			return fmt.Errorf("拉流压制任务必须填写源流地址")
		}
		// 本地流写法 local://<app>/<stream>：校验流存在
		if strings.HasPrefix(src, "local://") {
			parts := strings.SplitN(strings.TrimPrefix(src, "local://"), "/", 2)
			if len(parts) != 2 || parts[1] == "" {
				return fmt.Errorf("本地流地址格式应为 local://<app>/<stream>")
			}
			if _, err := s.streamRepo.GetByStream(parts[0], parts[1]); err != nil {
				return fmt.Errorf("本地流 %s/%s 不存在，请先在流管理中创建", parts[0], parts[1])
			}
		}
	}
	if err := validateStreamID(req.Stream); err != nil {
		return err
	}
	if req.LogoMaterialID > 0 {
		m, err := s.materialRepo.GetByID(req.LogoMaterialID)
		if err != nil {
			return err
		}
		if m.Type != "logo" {
			return fmt.Errorf("Logo 素材必须是图片类型")
		}
	}
	if req.CaptionEnabled && strings.TrimSpace(req.CaptionText) == "" {
		return fmt.Errorf("开启滚动字幕时必须填写字幕内容")
	}
	// 填充默认值
	if req.App == "" {
		req.App = s.cfg.ZLM.App
	}
	if req.CaptionSize <= 0 {
		req.CaptionSize = 28
	}
	if req.CaptionSpeed <= 0 {
		req.CaptionSpeed = 80
	}
	if req.CaptionPosition == "" {
		req.CaptionPosition = "bottom"
	}
	if req.CaptionColor == "" {
		req.CaptionColor = "white"
	}
	if req.LogoPosition == "" {
		req.LogoPosition = "tr"
	}
	if req.VBitrate <= 0 {
		req.VBitrate = 1500
	}
	if req.QualityMode == "" {
		req.QualityMode = "bitrate"
	}
	if req.QualityMode != "bitrate" && req.QualityMode != "crf" {
		return fmt.Errorf("编码模式只支持 bitrate（固定码率）或 crf（画质优先）")
	}
	// CRF 合法范围 0-51，常用 18-28；默认 21
	if req.CRF < 0 || req.CRF > 51 {
		return fmt.Errorf("CRF 需在 0-51 之间（越小越清晰，推荐 19-23）")
	}
	if req.QualityMode == "crf" && req.CRF == 0 {
		req.CRF = 21
	}

	t.Name = req.Name
	t.Mode = req.Mode
	t.MaterialID = req.MaterialID
	t.SrcURL = strings.TrimSpace(req.SrcURL)
	t.App = req.App
	t.Stream = req.Stream
	t.CaptionEnabled = boolToInt(req.CaptionEnabled)
	t.CaptionText = strings.TrimSpace(req.CaptionText)
	t.CaptionSize = req.CaptionSize
	t.CaptionColor = req.CaptionColor
	t.CaptionPosition = req.CaptionPosition
	t.CaptionSpeed = req.CaptionSpeed
	t.LogoMaterialID = req.LogoMaterialID
	t.LogoPosition = req.LogoPosition
	t.LogoOpacity = clampInt(req.LogoOpacity, 0, 100, 100)
	t.LogoWidthPct = clampInt(req.LogoWidthPct, 0, 100, 0)
	t.VBitrate = req.VBitrate
	t.QualityMode = req.QualityMode
	t.CRF = req.CRF
	t.Status = req.Status
	t.RetryCount = req.RetryCount
	return nil
}

// CreateTask 创建任务。
func (s *StudioService) CreateTask(req *StudioTaskRequest, userID uint64) (*models.StudioTask, error) {
	t := &models.StudioTask{CreatedBy: userID}
	if err := s.normalizeTask(req, t); err != nil {
		return nil, err
	}
	// 输出流必须已在流管理中登记（推流鉴权要求），不存在则自动创建
	if err := s.ensureStream(t.App, t.Stream, req.Name, userID); err != nil {
		return nil, err
	}
	if err := s.taskRepo.Create(t); err != nil {
		return nil, err
	}
	logger.Info("新增制播任务: %s [%s] -> %s/%s (操作人ID=%d)", t.Name, t.Mode, t.App, t.Stream, userID)
	return t, nil
}

// UpdateTask 更新任务。
func (s *StudioService) UpdateTask(id uint64, req *StudioTaskRequest) (*models.StudioTask, error) {
	t, err := s.taskRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.normalizeTask(req, t); err != nil {
		return nil, err
	}
	if err := s.ensureStream(t.App, t.Stream, req.Name, 0); err != nil {
		return nil, err
	}
	if err := s.taskRepo.Update(t); err != nil {
		return nil, err
	}
	return t, nil
}

// ensureStream 确保输出流在流管理中已登记（未登记则自动创建，供推流鉴权与播放管理使用）。
func (s *StudioService) ensureStream(app, stream, name string, userID uint64) error {
	if st, err := s.streamRepo.GetByStream(app, stream); err == nil && st != nil {
		return nil
	}
	st := &models.Stream{
		Name:      "[制播] " + name,
		App:       app,
		Stream:    stream,
		Vhost:     s.cfg.ZLM.Vhost,
		PushKey:   generatePushKey(),
		Status:    1,
		CreatedBy: userID,
	}
	return s.streamRepo.Create(st)
}

// DeleteTask 删除任务（运行中的任务先停止）。
func (s *StudioService) DeleteTask(id uint64) error {
	s.StopTask(id)
	return s.taskRepo.Delete(id)
}

// GetTask 查询任务。
func (s *StudioService) GetTask(id uint64) (*models.StudioTask, error) {
	return s.taskRepo.GetByID(id)
}

// ListTasks 分页查询任务。
func (s *StudioService) ListTasks(page, pageSize int, keyword string) ([]models.StudioTask, int64, error) {
	return s.taskRepo.List(page, pageSize, keyword)
}

// ---------------------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func clampInt(v, min, max, def int) int {
	if v < min || v > max {
		return def
	}
	return v
}

// randHex 生成随机 hex 字符串（n 字节 => 2n 字符）。
func randHex(n int) string {
	b := make([]byte, n)
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Read(b)
	return fmt.Sprintf("%x", b)
}

// copyWithLimit 限制大小的文件拷贝。
func copyWithLimit(dst *os.File, src io.Reader, limit int64) (int64, error) {
	// multipart part 每次 Read 只返回几 KB（受边界扫描窗口限制），用 bufio 聚合成 1MB 再写盘，减少系统调用
	w := bufio.NewWriterSize(dst, 1<<20)
	buf := make([]byte, 1<<20)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > limit {
				return total, fmt.Errorf("文件超出大小限制（4GB）")
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return total, err
		}
	}
	if err := w.Flush(); err != nil {
		return total, err
	}
	return total, nil
}

// InferMaterialType 按扩展名推断素材类型（上传请求未显式给出 type 时使用）。
func InferMaterialType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if logoExts[ext] {
		return "logo"
	}
	return "video"
}
