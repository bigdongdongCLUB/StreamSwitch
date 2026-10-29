package repository

import (
	"time"

	"livems/internal/models"
)

// TokenRepo 播放令牌仓储（MySQL 留档）。
type TokenRepo struct{}

// NewTokenRepo 创建令牌仓储。
func NewTokenRepo() *TokenRepo { return &TokenRepo{} }

// Create 保存令牌记录。
func (r *TokenRepo) Create(t *models.PlayToken) error {
	res, err := exec(`
		INSERT INTO play_token (token, stream_id, app, stream, expire_at, ip_bind, format, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		t.Token, t.StreamID, t.App, t.Stream, t.ExpireAt, t.IPBind, t.Format, t.CreatedBy, time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

// GetByToken 按令牌值查询。
func (r *TokenRepo) GetByToken(token string) (*models.PlayToken, error) {
	row := queryRow(`SELECT id, token, stream_id, app, stream, expire_at, ip_bind, format, created_by, created_at
		FROM play_token WHERE token=?`, token)
	var t models.PlayToken
	err := row.Scan(&t.ID, &t.Token, &t.StreamID, &t.App, &t.Stream, &t.ExpireAt, &t.IPBind,
		&t.Format, &t.CreatedBy, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetByID 按ID查询令牌。
func (r *TokenRepo) GetByID(id uint64) (*models.PlayToken, error) {
	row := queryRow(`SELECT id, token, stream_id, app, stream, expire_at, ip_bind, format, created_by, created_at
		FROM play_token WHERE id=?`, id)
	var t models.PlayToken
	err := row.Scan(&t.ID, &t.Token, &t.StreamID, &t.App, &t.Stream, &t.ExpireAt, &t.IPBind,
		&t.Format, &t.CreatedBy, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List 分页查询令牌。
func (r *TokenRepo) List(page, pageSize int) ([]models.PlayToken, int64, error) {
	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM play_token`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := query(`SELECT id, token, stream_id, app, stream, expire_at, ip_bind, format, created_by, created_at
		FROM play_token ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.PlayToken, 0, pageSize)
	for rows.Next() {
		var t models.PlayToken
		if err := rows.Scan(&t.ID, &t.Token, &t.StreamID, &t.App, &t.Stream, &t.ExpireAt, &t.IPBind,
			&t.Format, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

// Delete 删除令牌。
func (r *TokenRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM play_token WHERE id=?`, id)
	return err
}

// DeleteExpired 清理过期的令牌留档。
func (r *TokenRepo) DeleteExpired() (int64, error) {
	res, err := exec(`DELETE FROM play_token WHERE expire_at > 0 AND expire_at < ?`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// EncryptRepo HLS 加密配置仓储。
type EncryptRepo struct{}

// NewEncryptRepo 创建加密配置仓储。
func NewEncryptRepo() *EncryptRepo { return &EncryptRepo{} }

const encryptColumns = `id, stream_id, enabled, key_hex, key_uri, key_expire_sec, iv_strategy,
	expire_at, last_rotate_at, created_at, updated_at`

// Upsert 保存或更新某条流的加密配置。
func (r *EncryptRepo) Upsert(e *models.HLSEncrypt) error {
	now := time.Now().Unix()
	_, err := exec(`
		INSERT INTO hls_encrypt (stream_id, enabled, key_hex, key_uri, key_expire_sec, iv_strategy,
			expire_at, last_rotate_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), key_hex=VALUES(key_hex), key_uri=VALUES(key_uri),
			key_expire_sec=VALUES(key_expire_sec), iv_strategy=VALUES(iv_strategy), expire_at=VALUES(expire_at),
			last_rotate_at=VALUES(last_rotate_at), updated_at=VALUES(updated_at)`,
		e.StreamID, e.Enabled, e.KeyHex, e.KeyURI, e.KeyExpireSec, e.IVStrategy,
		e.ExpireAt, e.LastRotateAt, now, now)
	if err != nil {
		return err
	}
	return nil
}

// GetByStreamID 查询某条流的加密配置。
func (r *EncryptRepo) GetByStreamID(streamID uint64) (*models.HLSEncrypt, error) {
	row := queryRow(`SELECT `+encryptColumns+` FROM hls_encrypt WHERE stream_id=?`, streamID)
	e, err := scanEncrypt(row)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// List 查询全部加密配置。
func (r *EncryptRepo) List() ([]models.HLSEncrypt, error) {
	rows, err := query(`SELECT ` + encryptColumns + ` FROM hls_encrypt ORDER BY stream_id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.HLSEncrypt, 0)
	for rows.Next() {
		e, err := scanEncrypt(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *e)
	}
	return list, rows.Err()
}

// ListEnabled 查询所有启用了加密的流配置。
func (r *EncryptRepo) ListEnabled() ([]models.HLSEncrypt, error) {
	rows, err := query(`SELECT `+encryptColumns+` FROM hls_encrypt WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.HLSEncrypt, 0)
	for rows.Next() {
		e, err := scanEncrypt(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *e)
	}
	return list, rows.Err()
}

// Delete 删除某条流的加密配置。
func (r *EncryptRepo) Delete(streamID uint64) error {
	_, err := exec(`DELETE FROM hls_encrypt WHERE stream_id=?`, streamID)
	return err
}

// scanEncrypt 扫描一行加密配置。
func scanEncrypt(row scanner) (*models.HLSEncrypt, error) {
	var e models.HLSEncrypt
	err := row.Scan(&e.ID, &e.StreamID, &e.Enabled, &e.KeyHex, &e.KeyURI, &e.KeyExpireSec,
		&e.IVStrategy, &e.ExpireAt, &e.LastRotateAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// LogRepo 日志仓储（操作日志 + 任务错误日志）。
type LogRepo struct{}

// NewLogRepo 创建日志仓储。
func NewLogRepo() *LogRepo { return &LogRepo{} }

// CreateLog 写入一条操作日志。
func (r *LogRepo) CreateLog(l *models.SysLog) error {
	_, err := exec(`
		INSERT INTO sys_log (user_id, username, module, action, method, path, ip, params, result,
			status, error_msg, duration, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.UserID, l.Username, l.Module, l.Action, l.Method, l.Path, l.IP, l.Params, l.Result,
		l.Status, l.ErrorMsg, l.Duration, l.CreatedAt)
	return err
}

// ListLogs 分页查询操作日志。
func (r *LogRepo) ListLogs(page, pageSize int, module, username string) ([]models.SysLog, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if module != "" {
		where = append(where, "module=?")
		args = append(args, module)
	}
	if username != "" {
		where = append(where, "username=?")
		args = append(args, username)
	}
	cond := ""
	for i, w := range where {
		if i > 0 {
			cond += " AND "
		}
		cond += w
	}

	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM sys_log WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := query(`SELECT id, user_id, username, module, action, method, path, ip, params, result,
		status, error_msg, duration, created_at FROM sys_log WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.SysLog, 0, pageSize)
	for rows.Next() {
		var l models.SysLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Username, &l.Module, &l.Action, &l.Method, &l.Path,
			&l.IP, &l.Params, &l.Result, &l.Status, &l.ErrorMsg, &l.Duration, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}

// CreateTaskError 写入一条任务错误日志。
func (r *LogRepo) CreateTaskError(taskType string, taskID uint64, taskName, level, msg string) error {
	_, err := exec(`
		INSERT INTO task_error_log (task_type, task_id, task_name, level, message, created_at)
		VALUES (?,?,?,?,?,?)`,
		taskType, taskID, taskName, level, msg, time.Now().Unix())
	return err
}

// ListTaskErrors 分页查询任务错误日志。
func (r *LogRepo) ListTaskErrors(page, pageSize int, taskType string, taskID uint64) ([]models.TaskErrorLog, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if taskType != "" {
		where = append(where, "task_type=?")
		args = append(args, taskType)
	}
	if taskID > 0 {
		where = append(where, "task_id=?")
		args = append(args, taskID)
	}
	cond := ""
	for i, w := range where {
		if i > 0 {
			cond += " AND "
		}
		cond += w
	}

	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM task_error_log WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := query(`SELECT id, task_type, task_id, task_name, level, message, created_at
		FROM task_error_log WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.TaskErrorLog, 0, pageSize)
	for rows.Next() {
		var l models.TaskErrorLog
		if err := rows.Scan(&l.ID, &l.TaskType, &l.TaskID, &l.TaskName, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}

// CleanOldLogs 清理过期的历史日志，保留 days 天。
func (r *LogRepo) CleanOldLogs(days int) error {
	deadline := time.Now().AddDate(0, 0, -days).Unix()
	if _, err := exec(`DELETE FROM sys_log WHERE created_at < ?`, deadline); err != nil {
		return err
	}
	_, err := exec(`DELETE FROM task_error_log WHERE created_at < ?`, deadline)
	return err
}
