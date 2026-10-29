package service

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"livems/internal/models"
	"livems/internal/pkg/logger"
)

// ---------------------------------------------------------------------------
// ffmpeg 命令构建
// ---------------------------------------------------------------------------

// escapeDrawtext 转义 drawtext 滤镜文本中的特殊字符。
func escapeDrawtext(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`:`, `\:`,
		`'`, `\'`,
		`%`, `\%`,
	)
	return r.Replace(s)
}

// captionY 字幕纵向位置表达式。
func captionY(position string) string {
	if position == "top" {
		return "24"
	}
	return "h-th-24"
}

// logoOverlayPos Logo 四角位置表达式。
func logoOverlayPos(position string) string {
	switch position {
	case "tl":
		return "24:24"
	case "bl":
		return "24:H-h-24"
	case "br":
		return "W-w-24:H-h-24"
	default: // tr
		return "W-w-24:24"
	}
}

// filterResult 滤镜构建结果。
type filterResult struct {
	filter   string // -filter_complex 内容
	videoOut string // 视频输出标签，如 [v2]
	active   bool   // 是否有滤镜（决定是否重编码）
}

// buildFilters 构建滤镜链：Logo overlay（含透明度/缩放）→ 滚动字幕 drawtext（压在最上层）。
func (s *StudioService) buildFilters(t *models.StudioTask, logoPath string) filterResult {
	var b strings.Builder
	cur := "[0:v]"
	n := 0

	// Logo 叠加
	if t.LogoMaterialID > 0 && logoPath != "" {
		opacity := float64(t.LogoOpacity) / 100.0
		chain := "[1:v]format=rgba"
		if opacity < 1 {
			chain += fmt.Sprintf(",colorchannelmixer=aa=%.2f", opacity)
		}
		if t.LogoWidthPct > 0 {
			// scale2ref：按主画面宽度百分比缩放 Logo
			chain += "[lg0];[lg0][0:v]scale2ref=w='iw*" + strconv.Itoa(t.LogoWidthPct) + "/100':h=-1[lg][lgbase];[lgbase]"
			b.WriteString(chain)
			n++
			b.WriteString(fmt.Sprintf("[lg]overlay=%s[v%d];", logoOverlayPos(t.LogoPosition), n))
		} else {
			chain += "[lg];[0:v]"
			b.WriteString(chain)
			n++
			b.WriteString(fmt.Sprintf("[lg]overlay=%s[v%d];", logoOverlayPos(t.LogoPosition), n))
		}
		cur = fmt.Sprintf("[v%d]", n)
	}

	// 滚动字幕（从右向左匀速滚动）
	if t.CaptionEnabled == 1 && t.CaptionText != "" {
		n++
		b.WriteString(fmt.Sprintf(
			"%sdrawtext=fontfile='%s':text='%s':fontsize=%d:fontcolor=%s:borderw=1:bordercolor=black@0.7:"+
				"x=w-mod(t\\*%d\\, w+tw):y=%s[v%d]",
			cur, s.cfg.Studio.FontFile, escapeDrawtext(t.CaptionText), t.CaptionSize, t.CaptionColor,
			t.CaptionSpeed, captionY(t.CaptionPosition), n))
		cur = fmt.Sprintf("[v%d]", n)
	}

	fc := strings.TrimSuffix(b.String(), ";")
	return filterResult{filter: fc, videoOut: cur, active: fc != ""}
}

// resolveSrcURL 解析 relay 模式的源流地址。
//
// 支持 local://<app>/<stream> 写法：转为本机 ZLM 的 HTTP-FLV 地址，
// 并自动签发绑定该流的长期播放令牌（30 天，任务重启/崩溃恢复时重新签发），
// 用户无需关心本机播放鉴权细节。
func (s *StudioService) resolveSrcURL(t *models.StudioTask) (string, error) {
	src := strings.TrimSpace(t.SrcURL)
	if !strings.HasPrefix(src, "local://") {
		return src, nil
	}
	parts := strings.SplitN(strings.TrimPrefix(src, "local://"), "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("本地流地址格式应为 local://<app>/<stream>")
	}
	st, err := s.streamRepo.GetByStream(parts[0], parts[1])
	if err != nil || st == nil {
		return "", fmt.Errorf("本地流 %s/%s 不存在", parts[0], parts[1])
	}
	// 自动签发内部播放令牌（绑定该流，不限定格式——ZLM 对 .live.flv 上报的 schema 为 rtmp）
	authSvc := NewAuthService(s.cfg)
	tk, err := authSvc.CreateToken(st.ID, 30*86400, "", "")
	if err != nil {
		return "", fmt.Errorf("内部播放令牌签发失败: %w", err)
	}
	zlmHTTP := strings.TrimSuffix(s.cfg.HLS.UpstreamURL, "/")
	if zlmHTTP == "" {
		zlmHTTP = "http://127.0.0.1:8080"
	}
	return fmt.Sprintf("%s/%s/%s.live.flv?token=%s", zlmHTTP, st.App, st.Stream, tk.Token), nil
}

// buildFFmpegArgs 构建完整 ffmpeg 参数。
func (s *StudioService) buildFFmpegArgs(t *models.StudioTask) ([]string, error) {
	// 推流地址（输出流必须已登记，取其推流 Key 通过 on_publish 鉴权）
	st, err := s.streamRepo.GetByStream(t.App, t.Stream)
	if err != nil || st == nil {
		return nil, fmt.Errorf("输出流 %s/%s 未在流管理中登记", t.App, t.Stream)
	}
	if st.Status != 1 {
		return nil, fmt.Errorf("输出流 %s/%s 已在流管理中被禁用，请先启用该流", t.App, t.Stream)
	}
	pushURL := fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(s.cfg.Studio.PushBase, "/"), t.App, t.Stream)
	if st.PushKey != "" {
		pushURL += "?key=" + st.PushKey
	}

	// Logo 素材路径
	logoPath := ""
	if t.LogoMaterialID > 0 {
		lm, err := s.materialRepo.GetByID(t.LogoMaterialID)
		if err != nil {
			return nil, fmt.Errorf("Logo 素材不存在")
		}
		logoPath = filepath.Join(s.cfg.Studio.DataDir, lm.FilePath)
	}

	args := []string{"-hide_banner", "-loglevel", "warning", "-nostdin"}

	// 输入
	if t.Mode == "loop" {
		m, err := s.materialRepo.GetByID(t.MaterialID)
		if err != nil {
			return nil, err
		}
		args = append(args, "-re", "-stream_loop", "-1", "-i", filepath.Join(s.cfg.Studio.DataDir, m.FilePath))
	} else {
		srcURL, err := s.resolveSrcURL(t)
		if err != nil {
			return nil, err
		}
		args = append(args, "-i", srcURL)
	}
	if logoPath != "" {
		args = append(args, "-i", logoPath)
	}

	fr := s.buildFilters(t, logoPath)
	if fr.active {
		// 有滤镜：重编码视频，音频转 AAC 保证兼容
		args = append(args,
			"-filter_complex", fr.filter,
			"-map", fr.videoOut,
			"-map", "0:a?",
			"-c:v", "libx264", "-preset", "veryfast",
			"-pix_fmt", "yuv420p",
		)
		if t.QualityMode == "crf" {
			// 画质优先：CRF 固定质量因子，vbitrate 作为最大码率上限防止带宽超标
			crf := t.CRF
			if crf <= 0 {
				crf = 21
			}
			args = append(args,
				"-crf", strconv.Itoa(crf),
				"-maxrate", strconv.Itoa(t.VBitrate*2)+"k", "-bufsize", strconv.Itoa(t.VBitrate*4)+"k",
			)
		} else {
			// 固定码率（VBR 约束）
			args = append(args,
				"-b:v", strconv.Itoa(t.VBitrate)+"k",
				"-maxrate", strconv.Itoa(t.VBitrate*2)+"k", "-bufsize", strconv.Itoa(t.VBitrate*2)+"k",
			)
		}
		args = append(args,
			"-c:a", "aac", "-b:a", "128k",
			"-f", "flv", pushURL,
		)
		return args, nil
	}
	// 无滤镜：直接转封装（零重编码，CPU 开销极低）
	args = append(args, "-c", "copy", "-f", "flv", pushURL)
	return args, nil
}

// ---------------------------------------------------------------------------
// 进程管理器
// ---------------------------------------------------------------------------

// procEntry 运行中的 ffmpeg 进程。
type procEntry struct {
	cmd        *exec.Cmd
	manualStop bool
	startedAt  time.Time
	done       chan struct{} // 进程退出并从 studioProcs 移除后关闭，供 StopTask 同步等待
}

var (
	studioMu    sync.Mutex
	studioProcs = map[uint64]*procEntry{}
)

// tailBuffer 只保留尾部内容的缓冲（记录 ffmpeg 报错尾部）。
type tailBuffer struct {
	buf bytes.Buffer
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	if t.buf.Len()+len(p) > t.max {
		over := t.buf.Len() + len(p) - t.max
		b := t.buf.Bytes()
		t.buf.Reset()
		if over < len(b) {
			t.buf.Write(b[over:])
		}
	}
	return t.buf.Write(p)
}

// StartTask 启动制播任务（拉起 ffmpeg 进程）。
func (s *StudioService) StartTask(id uint64) error {
	studioMu.Lock()
	if _, ok := studioProcs[id]; ok {
		studioMu.Unlock()
		return fmt.Errorf("任务已在运行中")
	}
	studioMu.Unlock()

	t, err := s.taskRepo.GetByID(id)
	if err != nil {
		return err
	}
	if t.Status != 1 {
		return fmt.Errorf("任务已禁用，请先启用")
	}
	args, err := s.buildFFmpegArgs(t)
	if err != nil {
		return err
	}

	cmd := exec.Command(s.cfg.Studio.FFmpegPath, args...)
	stderr := &tailBuffer{max: 2048}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = s.taskRepo.UpdateRunStatus(id, "failed", 0, "ffmpeg 启动失败: "+err.Error())
		return fmt.Errorf("ffmpeg 启动失败: %w", err)
	}

	studioMu.Lock()
	studioProcs[id] = &procEntry{cmd: cmd, startedAt: time.Now(), done: make(chan struct{})}
	studioMu.Unlock()
	_ = s.taskRepo.UpdateRunStatus(id, "running", cmd.Process.Pid, "")
	logger.Info("制播任务已启动: id=%d name=%s pid=%d", id, t.Name, cmd.Process.Pid)

	// 监控进程退出
	go s.watchTask(id, cmd, stderr)
	return nil
}

// watchTask 监控 ffmpeg 进程，异常退出时按重试策略重启。
func (s *StudioService) watchTask(id uint64, cmd *exec.Cmd, stderr *tailBuffer) {
	err := cmd.Wait()

	studioMu.Lock()
	entry, ok := studioProcs[id]
	if ok && entry.cmd == cmd {
		delete(studioProcs, id)
	}
	manual := ok && entry.manualStop
	studioMu.Unlock()
	if ok && entry.cmd == cmd {
		close(entry.done) // 通知 StopTask：已清理，可以安全重新启动
	}

	if manual {
		_ = s.taskRepo.UpdateRunStatus(id, "stopped", 0, "")
		logger.Info("制播任务已手动停止: id=%d", id)
		return
	}

	// 稳定运行超 60 秒的退出视为偶发故障，清零重试计数
	if ok && time.Since(entry.startedAt) > 60*time.Second {
		_ = s.taskRepo.ResetRetryTimes(id)
	}

	t, err2 := s.taskRepo.GetByID(id)
	if err2 != nil || t.Status != 1 {
		_ = s.taskRepo.UpdateRunStatus(id, "stopped", 0, "")
		return
	}
	// 输出流在流管理中被禁用（on_publish 会一直拒绝）：不再无意义地重试，标记失败并说明原因
	if st, err := s.streamRepo.GetByStream(t.App, t.Stream); err == nil && st != nil && st.Status != 1 {
		_ = s.taskRepo.UpdateRunStatus(id, "failed", 0, "输出流 "+t.App+"/"+t.Stream+" 已在流管理中被禁用，启用该流后请手动启动任务")
		logger.Warn("制播任务因输出流被禁用而停止: id=%d name=%s stream=%s/%s", id, t.Name, t.App, t.Stream)
		return
	}

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	tail := strings.TrimSpace(stderr.buf.String())
	if tail != "" {
		errMsg = errMsg + " | " + tail
	}
	if len(errMsg) > 900 {
		errMsg = errMsg[len(errMsg)-900:]
	}

	if t.RetryCount >= 0 && t.RetryTimes >= t.RetryCount {
		_ = s.taskRepo.UpdateRunStatus(id, "failed", 0, "已达最大重启次数: "+errMsg)
		logger.Warn("制播任务达到最大重启次数: id=%d name=%s", id, t.Name)
		return
	}
	_ = s.taskRepo.IncrRetryTimes(id)
	_ = s.taskRepo.UpdateRunStatus(id, "failed", 0, errMsg)
	logger.Warn("制播任务异常退出，5 秒后重启: id=%d name=%s err=%s", id, t.Name, errMsg)

	time.Sleep(5 * time.Second)
	// 再次确认任务仍启用后重启
	if t2, err := s.taskRepo.GetByID(id); err == nil && t2.Status == 1 {
		if err := s.StartTask(id); err != nil {
			logger.Warn("制播任务重启失败: id=%d err=%v", id, err)
		}
	}
}

// StopTask 停止制播任务。
func (s *StudioService) StopTask(id uint64) {
	studioMu.Lock()
	entry, ok := studioProcs[id]
	if ok {
		entry.manualStop = true
	}
	studioMu.Unlock()
	if !ok {
		_ = s.taskRepo.UpdateRunStatus(id, "stopped", 0, "")
		return
	}
	_ = entry.cmd.Process.Kill()
	// 等待 watchTask 回收进程并从运行表移除（最多 5 秒），
	// 否则紧接着的 StartTask（如编辑后重启）会误判「任务已在运行中」。
	select {
	case <-entry.done:
	case <-time.After(5 * time.Second):
		logger.Warn("制播任务停止超时，进程可能仍在退出: id=%d pid=%d", id, entry.cmd.Process.Pid)
	}
}

// ToggleTask 启停切换。
func (s *StudioService) ToggleTask(id uint64) (string, error) {
	t, err := s.taskRepo.GetByID(id)
	if err != nil {
		return "", err
	}
	if t.RunStatus == "running" {
		s.StopTask(id)
		return "stopped", nil
	}
	if err := s.StartTask(id); err != nil {
		return "", err
	}
	return "running", nil
}

// RestoreTasks 服务启动时恢复全部启用任务。
func (s *StudioService) RestoreTasks() {
	tasks, err := s.taskRepo.ListEnabled()
	if err != nil {
		logger.Warn("恢复制播任务失败: %v", err)
		return
	}
	n := 0
	for _, t := range tasks {
		if err := s.StartTask(t.ID); err != nil {
			logger.Warn("恢复制播任务失败: id=%d name=%s err=%v", t.ID, t.Name, err)
		} else {
			n++
		}
	}
	if len(tasks) > 0 {
		logger.Info("已恢复 %d/%d 个制播任务", n, len(tasks))
	}
}

// StopAllTasks 服务退出前停止全部制播进程。
func (s *StudioService) StopAllTasks() {
	studioMu.Lock()
	entries := make([]*procEntry, 0, len(studioProcs))
	for _, e := range studioProcs {
		e.manualStop = true
		entries = append(entries, e)
	}
	studioMu.Unlock()
	for _, e := range entries {
		_ = e.cmd.Process.Kill()
	}
}
