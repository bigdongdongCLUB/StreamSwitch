package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"livems/internal/models"
	"livems/internal/pkg/logger"
	"livems/internal/pkg/util"
	"livems/internal/zlm"
)

// ---------------------------------------------------------------------------
// RTMPS 转推通道（ffmpeg 进程）
//
// ZLM 的 addStreamPusherProxy 不支持 rtmps 目标地址（URL 解析失败）。
// rtmps 目标（YouTube/Facebook/Instagram 等强制 TLS 的平台）改由本机完整版
// ffmpeg 进程转推：拉本机 ZLM 的 HTTP-FLV（带内部令牌），-c copy 转封装直推目标。
// ---------------------------------------------------------------------------

// isRTMPS 判断目标地址是否为 RTMPS。
func isRTMPS(dstURL string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(dstURL)), "rtmps://")
}

// forwardProcEntry 运行中的转推 ffmpeg 进程。
type forwardProcEntry struct {
	cmd        *exec.Cmd
	manualStop bool
	startedAt  time.Time
}

var (
	forwardMu    sync.Mutex
	forwardProcs = map[uint64]*forwardProcEntry{}
)

// buildLocalSrcURL 生成本机源流地址（HTTP-FLV + 内部播放令牌，30 天）。
func (s *ForwardService) buildLocalSrcURL(app, stream string) (string, error) {
	st, err := s.streamRepo.GetByStream(app, stream)
	if err != nil || st == nil {
		return "", fmt.Errorf("源流 %s/%s 未登记", app, stream)
	}
	authSvc := NewAuthService(s.cfg)
	tk, err := authSvc.CreateToken(st.ID, 30*86400, "", "")
	if err != nil {
		return "", fmt.Errorf("内部播放令牌签发失败: %w", err)
	}
	zlmHTTP := strings.TrimSuffix(s.cfg.HLS.UpstreamURL, "/")
	if zlmHTTP == "" {
		zlmHTTP = "http://127.0.0.1:8080"
	}
	return fmt.Sprintf("%s/%s/%s.live.flv?token=%s", zlmHTTP, app, stream, tk.Token), nil
}

// buildForwardFFmpegArgs 构建转推 ffmpeg 参数（转封装，零重编码）。
func (s *ForwardService) buildForwardFFmpegArgs(f *models.StreamForward) ([]string, error) {
	src, err := s.buildLocalSrcURL(f.SrcApp, f.SrcStream)
	if err != nil {
		return nil, err
	}
	return []string{
		"-hide_banner", "-loglevel", "warning", "-nostdin",
		"-i", src,
		"-c", "copy",
		"-f", "flv", f.DstURL,
	}, nil
}

// startFFmpeg 启动 ffmpeg 转推进程。
func (s *ForwardService) startFFmpeg(f *models.StreamForward) error {
	forwardMu.Lock()
	if _, ok := forwardProcs[f.ID]; ok {
		forwardMu.Unlock()
		return fmt.Errorf("任务已在运行中")
	}
	forwardMu.Unlock()

	args, err := s.buildForwardFFmpegArgs(f)
	if err != nil {
		return err
	}
	cmd := exec.Command(s.cfg.Studio.FFmpegPath, args...)
	stderr := &tailBuffer{max: 2048}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg 启动失败: %w", err)
	}
	forwardMu.Lock()
	forwardProcs[f.ID] = &forwardProcEntry{cmd: cmd, startedAt: time.Now()}
	forwardMu.Unlock()
	s.repo.UpdateZLMKey(f.ID, "ffmpeg")
	s.repo.UpdateRunStatus(f.ID, "running", "")
	logger.Info("转推任务已启动（ffmpeg rtmps 通道）: id=%d %s/%s -> %s pid=%d", f.ID, f.SrcApp, f.SrcStream, f.DstURL, cmd.Process.Pid)

	go s.watchForwardProc(f.ID, cmd, stderr)
	return nil
}

// watchForwardProc 监控转推进程，异常退出按重试策略重启。
func (s *ForwardService) watchForwardProc(id uint64, cmd *exec.Cmd, stderr *tailBuffer) {
	err := cmd.Wait()

	forwardMu.Lock()
	entry, ok := forwardProcs[id]
	if ok && entry.cmd == cmd {
		delete(forwardProcs, id)
	}
	manual := ok && entry.manualStop
	forwardMu.Unlock()

	if manual {
		s.repo.UpdateZLMKey(id, "")
		s.repo.UpdateRunStatus(id, "stopped", "")
		logger.Info("转推任务已手动停止: id=%d", id)
		return
	}

	// 稳定运行超 60 秒清零重试计数
	if ok && time.Since(entry.startedAt) > 60*time.Second {
		s.repo.ResetRetryTimes(id)
	}

	f, err2 := s.repo.GetByID(id)
	if err2 != nil || f.Status != 1 {
		s.repo.UpdateZLMKey(id, "")
		s.repo.UpdateRunStatus(id, "stopped", "")
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

	if f.RetryCount >= 0 && f.RetryTimes >= f.RetryCount {
		s.repo.UpdateRunStatus(id, "failed", "已达最大重启次数: "+errMsg)
		s.logRepo.CreateTaskError("forward", id, f.Name, "error", errMsg)
		logger.Warn("转推任务达到最大重启次数: id=%d name=%s", id, f.Name)
		return
	}
	s.repo.IncrRetryTimes(id)
	s.repo.UpdateRunStatus(id, "failed", errMsg)
	s.logRepo.CreateTaskError("forward", id, f.Name, "warn", "转推中断，5 秒后重启: "+errMsg)
	logger.Warn("转推任务异常退出，5 秒后重启: id=%d name=%s err=%s", id, f.Name, errMsg)

	time.Sleep(5 * time.Second)
	if f2, err := s.repo.GetByID(id); err == nil && f2.Status == 1 {
		if err := s.startFFmpeg(f2); err != nil {
			logger.Warn("转推任务重启失败: id=%d err=%v", id, err)
		}
	}
}

// stopFFmpeg 停止 ffmpeg 转推进程。
func (s *ForwardService) stopFFmpeg(f *models.StreamForward) {
	forwardMu.Lock()
	entry, ok := forwardProcs[f.ID]
	if ok {
		entry.manualStop = true
	}
	forwardMu.Unlock()
	if !ok {
		s.repo.UpdateZLMKey(f.ID, "")
		s.repo.UpdateRunStatus(f.ID, "stopped", "")
		return
	}
	_ = entry.cmd.Process.Kill()
}

// ---------------------------------------------------------------------------
// Start/Stop 分流
// ---------------------------------------------------------------------------

// Start 启动转推任务：rtmp 目标走 ZLM proxy，rtmps 目标走 ffmpeg 通道。
func (s *ForwardService) Start(id uint64) error {
	f, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("转推流任务不存在")
	}
	if f.Status != 1 {
		return fmt.Errorf("任务已禁用，无法启动")
	}
	// rtmps 目标走 ffmpeg 通道（ZLM proxy 不支持 rtmps）
	if isRTMPS(f.DstURL) || strings.EqualFold(f.Schema, "rtmps") {
		return s.startFFmpeg(f)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key, err := s.zlmClient.AddStreamPusherProxy(ctx, zlm.ForwardAddReq{
		Schema:     "rtmp", // ZLM 侧固定 rtmp（目标协议由 dst_url 决定）
		Vhost:      f.SrcVhost,
		App:        f.SrcApp,
		Stream:     f.SrcStream,
		DstURL:     f.DstURL,
		RetryCount: f.RetryCount,
		TimeoutSec: f.TimeoutSec,
	})
	if err != nil {
		// ZLM 报 "can not find the source stream" 时给出更友好的提示
		errMsg := err.Error()
		if strings.Contains(errMsg, "can not find the source stream") {
			errMsg = fmt.Sprintf("源流 %s/%s 当前不在线，请先推流再启动转推（源上线后会自动拉起）", f.SrcApp, f.SrcStream)
		}
		s.repo.UpdateRunStatus(id, "failed", util.TruncateString(errMsg, 1000))
		s.logRepo.CreateTaskError("forward", id, f.Name, "error", errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	s.repo.UpdateZLMKey(id, key)
	s.repo.UpdateRunStatus(id, "running", "")
	logger.Info("转推流任务已启动: id=%d %s/%s -> %s key=%s", id, f.SrcApp, f.SrcStream, f.DstURL, key)
	return nil
}

// Stop 停止转推任务。
func (s *ForwardService) Stop(id uint64) error {
	f, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("转推流任务不存在")
	}
	// ffmpeg 通道
	if f.ZLMKey == "ffmpeg" {
		s.stopFFmpeg(f)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if f.ZLMKey != "" {
		if err := s.zlmClient.DelStreamPusherProxy(ctx, f.ZLMKey); err != nil {
			logger.Warn("关闭转推流失败 id=%d: %v", id, err)
		}
	}
	s.repo.UpdateZLMKey(id, "")
	s.repo.UpdateRunStatus(id, "stopped", "")
	logger.Info("转推流任务已停止: id=%d", id)
	return nil
}
