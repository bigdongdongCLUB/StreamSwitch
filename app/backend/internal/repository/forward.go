package repository

import (
	"strings"
	"time"

	"livems/internal/models"
)

// ForwardRepo 转推流任务仓储。
type ForwardRepo struct{}

// NewForwardRepo 创建转推流仓储。
func NewForwardRepo() *ForwardRepo { return &ForwardRepo{} }

// forwardColumns 字段列表。
// 注意：schema 是 MySQL 8.0 的保留字，作为列名出现时必须用反引号包裹，否则语法报错。
const forwardColumns = "id, name, src_app, src_stream, src_vhost, dst_url, `schema`, status, run_status, " +
	"retry_count, timeout_sec, zlm_key, retry_times, last_error, last_active_at, created_by, created_at, updated_at"

// Create 新增转推流任务。
func (r *ForwardRepo) Create(f *models.StreamForward) error {
	res, err := exec(
		"INSERT INTO stream_forward (name, src_app, src_stream, src_vhost, dst_url, `schema`, status, "+
			"run_status, retry_count, timeout_sec, zlm_key, created_by, created_at, updated_at) "+
			"VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
		f.Name, f.SrcApp, f.SrcStream, f.SrcVhost, f.DstURL, f.Schema, f.Status, f.RunStatus,
		f.RetryCount, f.TimeoutSec, f.ZLMKey, f.CreatedBy, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	f.ID = uint64(id)
	return nil
}

// Update 更新转推流任务。
func (r *ForwardRepo) Update(f *models.StreamForward) error {
	_, err := exec(
		"UPDATE stream_forward SET name=?, src_app=?, src_stream=?, src_vhost=?, dst_url=?, `schema`=?, "+
			"status=?, retry_count=?, timeout_sec=?, updated_at=? WHERE id=?",
		f.Name, f.SrcApp, f.SrcStream, f.SrcVhost, f.DstURL, f.Schema, f.Status,
		f.RetryCount, f.TimeoutSec, time.Now().Unix(), f.ID)
	return err
}

// UpdateStatus 更新启用状态。
func (r *ForwardRepo) UpdateStatus(id uint64, status int) error {
	_, err := exec(`UPDATE stream_forward SET status=?, updated_at=? WHERE id=?`, status, time.Now().Unix(), id)
	return err
}

// UpdateRunStatus 更新运行状态与错误信息。
func (r *ForwardRepo) UpdateRunStatus(id uint64, runStatus, lastErr string) error {
	_, err := exec(`UPDATE stream_forward SET run_status=?, last_error=?, updated_at=? WHERE id=?`,
		runStatus, lastErr, time.Now().Unix(), id)
	return err
}

// UpdateZLMKey 记录 ZLM 任务 key。
func (r *ForwardRepo) UpdateZLMKey(id uint64, key string) error {
	_, err := exec(`UPDATE stream_forward SET zlm_key=?, updated_at=? WHERE id=?`, key, time.Now().Unix(), id)
	return err
}

// Delete 删除转推流任务。
func (r *ForwardRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM stream_forward WHERE id=?`, id)
	return err
}

// IncrRetryTimes 累加重试次数。
func (r *ForwardRepo) IncrRetryTimes(id uint64) error {
	_, err := exec(`UPDATE stream_forward SET retry_times=retry_times+1, updated_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// ResetRetryTimes 清零重试次数。
func (r *ForwardRepo) ResetRetryTimes(id uint64) error {
	_, err := exec(`UPDATE stream_forward SET retry_times=0, updated_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// GetByID 按ID查询。
func (r *ForwardRepo) GetByID(id uint64) (*models.StreamForward, error) {
	row := queryRow(`SELECT `+forwardColumns+` FROM stream_forward WHERE id=?`, id)
	f, err := scanForward(row)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// List 分页查询转推流任务。
func (r *ForwardRepo) List(page, pageSize int, keyword string, status int) ([]models.StreamForward, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if keyword != "" {
		where = append(where, "(name LIKE ? OR dst_url LIKE ? OR src_stream LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	if status >= 0 {
		where = append(where, "status=?")
		args = append(args, status)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM stream_forward WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := query(`SELECT `+forwardColumns+` FROM stream_forward WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	list := make([]models.StreamForward, 0, pageSize)
	for rows.Next() {
		f, err := scanForward(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	return list, total, rows.Err()
}

// ListEnabled 获取所有启用的转推流任务（用于启动时恢复）。
func (r *ForwardRepo) ListEnabled() ([]models.StreamForward, error) {
	rows, err := query(`SELECT ` + forwardColumns + ` FROM stream_forward WHERE status=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.StreamForward, 0)
	for rows.Next() {
		f, err := scanForward(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *f)
	}
	return list, rows.Err()
}

// ListBySource 查询某个源流的所有转推任务。
func (r *ForwardRepo) ListBySource(app, stream string) ([]models.StreamForward, error) {
	rows, err := query(`SELECT `+forwardColumns+` FROM stream_forward WHERE src_app=? AND src_stream=?`, app, stream)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.StreamForward, 0)
	for rows.Next() {
		f, err := scanForward(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *f)
	}
	return list, rows.Err()
}

// scanForward 扫描一行转推流记录。
func scanForward(row scanner) (*models.StreamForward, error) {
	var f models.StreamForward
	err := row.Scan(&f.ID, &f.Name, &f.SrcApp, &f.SrcStream, &f.SrcVhost, &f.DstURL, &f.Schema,
		&f.Status, &f.RunStatus, &f.RetryCount, &f.TimeoutSec, &f.ZLMKey, &f.RetryTimes,
		&f.LastError, &f.LastActiveAt, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}
