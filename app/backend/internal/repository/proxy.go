package repository

import (
	"strings"
	"time"

	"livems/internal/models"
)

// ProxyRepo 拉流代理任务仓储。
type ProxyRepo struct{}

// NewProxyRepo 创建拉流代理仓储。
func NewProxyRepo() *ProxyRepo { return &ProxyRepo{} }

const proxyColumns = `id, name, src_url, app, stream, vhost, status, retry_count, retry_interval,
	timeout_sec, enable_rtsp, enable_rtmp, enable_ts, enable_fmp4, enable_hls, zlm_key, run_status,
	last_error, retry_times, last_active_at, created_by, created_at, updated_at`

// Create 新增拉流代理任务。
func (r *ProxyRepo) Create(p *models.StreamProxy) error {
	res, err := exec(`
		INSERT INTO stream_proxy (name, src_url, app, stream, vhost, status, retry_count, retry_interval,
			timeout_sec, enable_rtsp, enable_rtmp, enable_ts, enable_fmp4, enable_hls, zlm_key, run_status,
			created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Name, p.SrcURL, p.App, p.Stream, p.Vhost, p.Status, p.RetryCount, p.RetryInterval,
		p.TimeoutSec, p.EnableRTSP, p.EnableRTMP, p.EnableTS, p.EnableFMP4, p.EnableHLS, p.ZLMKey,
		p.RunStatus, p.CreatedBy, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	p.ID = uint64(id)
	return nil
}

// Update 更新拉流代理任务。
func (r *ProxyRepo) Update(p *models.StreamProxy) error {
	_, err := exec(`
		UPDATE stream_proxy SET name=?, src_url=?, app=?, stream=?, vhost=?, status=?, retry_count=?,
			retry_interval=?, timeout_sec=?, enable_rtsp=?, enable_rtmp=?, enable_ts=?, enable_fmp4=?,
			enable_hls=?, updated_at=? WHERE id=?`,
		p.Name, p.SrcURL, p.App, p.Stream, p.Vhost, p.Status, p.RetryCount, p.RetryInterval,
		p.TimeoutSec, p.EnableRTSP, p.EnableRTMP, p.EnableTS, p.EnableFMP4, p.EnableHLS,
		time.Now().Unix(), p.ID)
	return err
}

// UpdateStatus 更新启用状态。
func (r *ProxyRepo) UpdateStatus(id uint64, status int) error {
	_, err := exec(`UPDATE stream_proxy SET status=?, updated_at=? WHERE id=?`, status, time.Now().Unix(), id)
	return err
}

// UpdateRunStatus 更新运行状态与错误信息。
func (r *ProxyRepo) UpdateRunStatus(id uint64, runStatus, lastErr string) error {
	_, err := exec(`UPDATE stream_proxy SET run_status=?, last_error=?, updated_at=? WHERE id=?`,
		runStatus, lastErr, time.Now().Unix(), id)
	return err
}

// UpdateZLMKey 记录 ZLM 任务 key。
func (r *ProxyRepo) UpdateZLMKey(id uint64, key string) error {
	_, err := exec(`UPDATE stream_proxy SET zlm_key=?, updated_at=? WHERE id=?`, key, time.Now().Unix(), id)
	return err
}

// Delete 删除拉流代理任务。
func (r *ProxyRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM stream_proxy WHERE id=?`, id)
	return err
}

// GetByID 按ID查询。
func (r *ProxyRepo) GetByID(id uint64) (*models.StreamProxy, error) {
	row := queryRow(`SELECT `+proxyColumns+` FROM stream_proxy WHERE id=?`, id)
	p, err := scanProxy(row)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// List 分页查询拉流代理任务。
func (r *ProxyRepo) List(page, pageSize int, keyword string, status int) ([]models.StreamProxy, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if keyword != "" {
		where = append(where, "(name LIKE ? OR src_url LIKE ? OR stream LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	if status >= 0 {
		where = append(where, "status=?")
		args = append(args, status)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM stream_proxy WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := query(`SELECT `+proxyColumns+` FROM stream_proxy WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	list := make([]models.StreamProxy, 0, pageSize)
	for rows.Next() {
		p, err := scanProxy(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *p)
	}
	return list, total, rows.Err()
}

// ListEnabled 获取所有启用的拉流代理任务（用于启动时恢复）。
func (r *ProxyRepo) ListEnabled() ([]models.StreamProxy, error) {
	rows, err := query(`SELECT `+proxyColumns+` FROM stream_proxy WHERE status=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.StreamProxy, 0)
	for rows.Next() {
		p, err := scanProxy(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

// scanProxy 扫描一行拉流代理记录。
func scanProxy(row scanner) (*models.StreamProxy, error) {
	var p models.StreamProxy
	err := row.Scan(&p.ID, &p.Name, &p.SrcURL, &p.App, &p.Stream, &p.Vhost, &p.Status, &p.RetryCount,
		&p.RetryInterval, &p.TimeoutSec, &p.EnableRTSP, &p.EnableRTMP, &p.EnableTS, &p.EnableFMP4,
		&p.EnableHLS, &p.ZLMKey, &p.RunStatus, &p.LastError, &p.RetryTimes, &p.LastActiveAt,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
