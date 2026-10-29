package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"livems/internal/models"
)

// StudioMaterialRepo 制播素材仓储。
type StudioMaterialRepo struct{}

// NewStudioMaterialRepo 创建素材仓储。
func NewStudioMaterialRepo() *StudioMaterialRepo { return &StudioMaterialRepo{} }

// Create 新增素材。
func (r *StudioMaterialRepo) Create(m *models.StudioMaterial) error {
	res, err := exec(`
		INSERT INTO studio_material (name, type, file_path, size, duration, width, height, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		m.Name, m.Type, m.FilePath, m.Size, m.Duration, m.Width, m.Height, m.CreatedBy, time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	m.ID = uint64(id)
	return nil
}

// List 按类型查询素材（可选类型过滤）。
func (r *StudioMaterialRepo) List(mtype string) ([]models.StudioMaterial, error) {
	sq := `SELECT id, name, type, file_path, size, duration, width, height, created_by, created_at FROM studio_material`
	var args []any
	if mtype != "" {
		sq += ` WHERE type=?`
		args = append(args, mtype)
	}
	sq += ` ORDER BY id DESC`
	rows, err := query(sq, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []models.StudioMaterial{}
	for rows.Next() {
		var m models.StudioMaterial
		if err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.FilePath, &m.Size, &m.Duration, &m.Width, &m.Height, &m.CreatedBy, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetByID 按 ID 查询素材。
func (r *StudioMaterialRepo) GetByID(id uint64) (*models.StudioMaterial, error) {
	row := queryRow(`SELECT id, name, type, file_path, size, duration, width, height, created_by, created_at FROM studio_material WHERE id=?`, id)
	var m models.StudioMaterial
	if err := row.Scan(&m.ID, &m.Name, &m.Type, &m.FilePath, &m.Size, &m.Duration, &m.Width, &m.Height, &m.CreatedBy, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("素材不存在")
		}
		return nil, err
	}
	return &m, nil
}

// Delete 删除素材。
func (r *StudioMaterialRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM studio_material WHERE id=?`, id)
	return err
}

// -----------------------------------------------------------------------------

// StudioTaskRepo 制播任务仓储。
type StudioTaskRepo struct{}

// NewStudioTaskRepo 创建任务仓储。
func NewStudioTaskRepo() *StudioTaskRepo { return &StudioTaskRepo{} }

// Create 新增任务。
func (r *StudioTaskRepo) Create(t *models.StudioTask) error {
	now := time.Now().Unix()
	res, err := exec(`
		INSERT INTO studio_task (name, mode, material_id, src_url, app, stream,
			caption_enabled, caption_text, caption_size, caption_color, caption_position, caption_speed,
			logo_material_id, logo_position, logo_opacity, logo_width_pct, vbitrate,
			quality_mode, crf, status, run_status, retry_count, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'idle', ?, ?, ?, ?)`,
		t.Name, t.Mode, t.MaterialID, t.SrcURL, t.App, t.Stream,
		t.CaptionEnabled, t.CaptionText, t.CaptionSize, t.CaptionColor, t.CaptionPosition, t.CaptionSpeed,
		t.LogoMaterialID, t.LogoPosition, t.LogoOpacity, t.LogoWidthPct, t.VBitrate,
		t.QualityMode, t.CRF, t.Status, t.RetryCount, t.CreatedBy, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

// Update 更新任务配置（不动运行态字段）。
func (r *StudioTaskRepo) Update(t *models.StudioTask) error {
	_, err := exec(`
		UPDATE studio_task SET name=?, mode=?, material_id=?, src_url=?, app=?, stream=?,
			caption_enabled=?, caption_text=?, caption_size=?, caption_color=?, caption_position=?, caption_speed=?,
			logo_material_id=?, logo_position=?, logo_opacity=?, logo_width_pct=?, vbitrate=?,
			quality_mode=?, crf=?, status=?, retry_count=?, updated_at=?
		WHERE id=?`,
		t.Name, t.Mode, t.MaterialID, t.SrcURL, t.App, t.Stream,
		t.CaptionEnabled, t.CaptionText, t.CaptionSize, t.CaptionColor, t.CaptionPosition, t.CaptionSpeed,
		t.LogoMaterialID, t.LogoPosition, t.LogoOpacity, t.LogoWidthPct, t.VBitrate,
		t.QualityMode, t.CRF, t.Status, t.RetryCount, time.Now().Unix(), t.ID)
	return err
}

// UpdateRunStatus 更新运行状态。
func (r *StudioTaskRepo) UpdateRunStatus(id uint64, runStatus string, pid int, lastError string) error {
	_, err := exec(`UPDATE studio_task SET run_status=?, pid=?, last_error=?, last_active_at=?, updated_at=? WHERE id=?`,
		runStatus, pid, lastError, time.Now().Unix(), time.Now().Unix(), id)
	return err
}

// IncrRetryTimes 累加重试次数。
func (r *StudioTaskRepo) IncrRetryTimes(id uint64) error {
	_, err := exec(`UPDATE studio_task SET retry_times=retry_times+1, updated_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// ResetRetryTimes 清零重试次数。
func (r *StudioTaskRepo) ResetRetryTimes(id uint64) error {
	_, err := exec(`UPDATE studio_task SET retry_times=0, updated_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// List 分页查询任务。
func (r *StudioTaskRepo) List(page, pageSize int, keyword string) ([]models.StudioTask, int64, error) {
	where := ` WHERE 1=1`
	var args []any
	if strings.TrimSpace(keyword) != "" {
		where += ` AND (name LIKE ? OR stream LIKE ?)`
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}
	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM studio_task`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	rows, err := query(`
		SELECT id, name, mode, material_id, src_url, app, stream,
			caption_enabled, caption_text, caption_size, caption_color, caption_position, caption_speed,
			logo_material_id, logo_position, logo_opacity, logo_width_pct, vbitrate, quality_mode, crf,
			status, run_status, pid, last_error, retry_count, retry_times, last_active_at,
			created_by, created_at, updated_at
		FROM studio_task`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []models.StudioTask{}
	for rows.Next() {
		var t models.StudioTask
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &t.MaterialID, &t.SrcURL, &t.App, &t.Stream,
			&t.CaptionEnabled, &t.CaptionText, &t.CaptionSize, &t.CaptionColor, &t.CaptionPosition, &t.CaptionSpeed,
			&t.LogoMaterialID, &t.LogoPosition, &t.LogoOpacity, &t.LogoWidthPct, &t.VBitrate, &t.QualityMode, &t.CRF,
			&t.Status, &t.RunStatus, &t.PID, &t.LastError, &t.RetryCount, &t.RetryTimes, &t.LastActiveAt,
			&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// GetByID 按 ID 查询任务。
func (r *StudioTaskRepo) GetByID(id uint64) (*models.StudioTask, error) {
	row := queryRow(`
		SELECT id, name, mode, material_id, src_url, app, stream,
			caption_enabled, caption_text, caption_size, caption_color, caption_position, caption_speed,
			logo_material_id, logo_position, logo_opacity, logo_width_pct, vbitrate, quality_mode, crf,
			status, run_status, pid, last_error, retry_count, retry_times, last_active_at,
			created_by, created_at, updated_at
		FROM studio_task WHERE id=?`, id)
	var t models.StudioTask
	err := row.Scan(&t.ID, &t.Name, &t.Mode, &t.MaterialID, &t.SrcURL, &t.App, &t.Stream,
		&t.CaptionEnabled, &t.CaptionText, &t.CaptionSize, &t.CaptionColor, &t.CaptionPosition, &t.CaptionSpeed,
		&t.LogoMaterialID, &t.LogoPosition, &t.LogoOpacity, &t.LogoWidthPct, &t.VBitrate, &t.QualityMode, &t.CRF,
		&t.Status, &t.RunStatus, &t.PID, &t.LastError, &t.RetryCount, &t.RetryTimes, &t.LastActiveAt,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("任务不存在")
		}
		return nil, err
	}
	return &t, nil
}

// ListEnabled 查询全部启用任务（服务启动时恢复运行用）。
func (r *StudioTaskRepo) ListEnabled() ([]models.StudioTask, error) {
	rows, err := query(`
		SELECT id, name, mode, material_id, src_url, app, stream,
			caption_enabled, caption_text, caption_size, caption_color, caption_position, caption_speed,
			logo_material_id, logo_position, logo_opacity, logo_width_pct, vbitrate, quality_mode, crf,
			status, run_status, pid, last_error, retry_count, retry_times, last_active_at,
			created_by, created_at, updated_at
		FROM studio_task WHERE status=1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []models.StudioTask{}
	for rows.Next() {
		var t models.StudioTask
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &t.MaterialID, &t.SrcURL, &t.App, &t.Stream,
			&t.CaptionEnabled, &t.CaptionText, &t.CaptionSize, &t.CaptionColor, &t.CaptionPosition, &t.CaptionSpeed,
			&t.LogoMaterialID, &t.LogoPosition, &t.LogoOpacity, &t.LogoWidthPct, &t.VBitrate, &t.QualityMode, &t.CRF,
			&t.Status, &t.RunStatus, &t.PID, &t.LastError, &t.RetryCount, &t.RetryTimes, &t.LastActiveAt,
			&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete 删除任务。
func (r *StudioTaskRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM studio_task WHERE id=?`, id)
	return err
}
