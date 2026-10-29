package repository

import (
	"strings"
	"time"

	"livems/internal/models"
)

// DomainRepo 播放域名仓储。
type DomainRepo struct{}

// NewDomainRepo 创建播放域名仓储。
func NewDomainRepo() *DomainRepo { return &DomainRepo{} }

// domainColumns 字段列表。
// 注意：schema 是 MySQL 8.0 的保留字，作为列名出现时必须用反引号包裹，否则语法报错。
const domainColumns = "id, name, `schema`, host, port, is_default, status, sort, remark, created_at, updated_at"

// Create 新增播放域名。
func (r *DomainRepo) Create(d *models.PlayDomain) error {
	res, err := exec(
		"INSERT INTO play_domain (name, `schema`, host, port, is_default, status, sort, remark, created_at, updated_at) "+
			"VALUES (?,?,?,?,?,?,?,?,?,?)",
		d.Name, d.Schema, d.Host, d.Port, d.IsDefault, d.Status, d.Sort, d.Remark,
		time.Now().Unix(), time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	d.ID = uint64(id)
	return nil
}

// Update 更新播放域名。
func (r *DomainRepo) Update(d *models.PlayDomain) error {
	_, err := exec(
		"UPDATE play_domain SET name=?, `schema`=?, host=?, port=?, is_default=?, status=?, sort=?, remark=?, updated_at=? "+
			"WHERE id=?",
		d.Name, d.Schema, d.Host, d.Port, d.IsDefault, d.Status, d.Sort, d.Remark, time.Now().Unix(), d.ID)
	return err
}

// Delete 删除播放域名。
func (r *DomainRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM play_domain WHERE id=?`, id)
	return err
}

// GetByID 按ID查询。
func (r *DomainRepo) GetByID(id uint64) (*models.PlayDomain, error) {
	row := queryRow(`SELECT `+domainColumns+` FROM play_domain WHERE id=?`, id)
	d, err := scanDomain(row)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// List 查询全部播放域名。
func (r *DomainRepo) List() ([]models.PlayDomain, error) {
	rows, err := query(`SELECT ` + domainColumns + ` FROM play_domain ORDER BY is_default DESC, sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.PlayDomain, 0)
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, rows.Err()
}

// ListEnabled 查询启用的播放域名。
func (r *DomainRepo) ListEnabled() ([]models.PlayDomain, error) {
	rows, err := query(`SELECT ` + domainColumns + ` FROM play_domain WHERE status=1 ORDER BY is_default DESC, sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.PlayDomain, 0)
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, rows.Err()
}

// ClearDefault 清除所有默认标记（保证唯一默认域名）。
func (r *DomainRepo) ClearDefault() error {
	_, err := exec(`UPDATE play_domain SET is_default=0`)
	return err
}

// SetDefault 设置默认域名（同时清除其他默认标记）。
func (r *DomainRepo) SetDefault(id uint64) error {
	return tx(func(t execTx) error {
		if _, err := t.Exec(`UPDATE play_domain SET is_default=0`); err != nil {
			return err
		}
		_, err := t.Exec(`UPDATE play_domain SET is_default=1, updated_at=? WHERE id=?`, time.Now().Unix(), id)
		return err
	})
}

// scanDomain 扫描一行播放域名记录。
func scanDomain(row scanner) (*models.PlayDomain, error) {
	var d models.PlayDomain
	err := row.Scan(&d.ID, &d.Name, &d.Schema, &d.Host, &d.Port, &d.IsDefault, &d.Status,
		&d.Sort, &d.Remark, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// FormatRepo 播放格式开关仓储。
type FormatRepo struct{}

// NewFormatRepo 创建格式开关仓储。
func NewFormatRepo() *FormatRepo { return &FormatRepo{} }

const formatColumns = `id, format, name, enabled, path_prefix, sort, remark, created_at, updated_at`

// List 查询全部播放格式。
func (r *FormatRepo) List() ([]models.PlayFormat, error) {
	rows, err := query(`SELECT ` + formatColumns + ` FROM play_format ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.PlayFormat, 0)
	for rows.Next() {
		var f models.PlayFormat
		if err := rows.Scan(&f.ID, &f.Format, &f.Name, &f.Enabled, &f.PathPrefix, &f.Sort, &f.Remark,
			&f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// GetByFormat 按格式标识查询。
func (r *FormatRepo) GetByFormat(format string) (*models.PlayFormat, error) {
	row := queryRow(`SELECT `+formatColumns+` FROM play_format WHERE format=?`, format)
	var f models.PlayFormat
	err := row.Scan(&f.ID, &f.Format, &f.Name, &f.Enabled, &f.PathPrefix, &f.Sort, &f.Remark,
		&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateEnabled 更新某个格式的分发开关。
func (r *FormatRepo) UpdateEnabled(format string, enabled int) error {
	_, err := exec(`UPDATE play_format SET enabled=?, updated_at=? WHERE format=?`,
		enabled, time.Now().Unix(), format)
	return err
}

// Update 更新格式配置。
func (r *FormatRepo) Update(f *models.PlayFormat) error {
	_, err := exec(`UPDATE play_format SET name=?, enabled=?, path_prefix=?, sort=?, remark=?, updated_at=? WHERE id=?`,
		f.Name, f.Enabled, f.PathPrefix, f.Sort, f.Remark, time.Now().Unix(), f.ID)
	return err
}

// EnabledFormats 返回所有已启用的格式标识集合。
func (r *FormatRepo) EnabledFormats() (map[string]bool, error) {
	rows, err := query(`SELECT format FROM play_format WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var format string
		if err := rows.Scan(&format); err != nil {
			return nil, err
		}
		out[format] = true
	}
	return out, rows.Err()
}

// EnsureFormats 确保默认的四种播放格式存在（服务启动时调用，兼容老数据）。
func (r *FormatRepo) EnsureFormats() error {
	defaults := []struct {
		format, name, prefix string
		sort                 int
	}{
		{"rtmp", "RTMP", "/live", 1},
		{"http-flv", "HTTP-FLV", "/live", 2},
		{"ws-flv", "WS-FLV", "/live", 3},
		{"hls", "HLS", "/live", 4},
	}
	for _, d := range defaults {
		_, err := exec(`
			INSERT INTO play_format (format, name, enabled, path_prefix, sort, created_at, updated_at)
			VALUES (?,?,1,?,?,?,?)
			ON DUPLICATE KEY UPDATE updated_at=updated_at`,
			d.format, d.name, d.prefix, d.sort, time.Now().Unix(), time.Now().Unix())
		if err != nil {
			return err
		}
	}
	return nil
}

// ConfigRepo 全局配置仓储。
type ConfigRepo struct{}

// NewConfigRepo 创建全局配置仓储。
func NewConfigRepo() *ConfigRepo { return &ConfigRepo{} }

// Get 按键读取配置值。
func (r *ConfigRepo) Get(key string) (string, error) {
	var val string
	err := queryRow(`SELECT config_val FROM sys_config WHERE config_key=?`, key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

// Set 写入配置值。
func (r *ConfigRepo) Set(key, val, remark string) error {
	_, err := exec(`
		INSERT INTO sys_config (config_key, config_val, remark, updated_at) VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE config_val=VALUES(config_val), remark=VALUES(remark), updated_at=VALUES(updated_at)`,
		key, val, remark, time.Now().Unix())
	return err
}

// All 读取全部配置。
func (r *ConfigRepo) All() (map[string]string, error) {
	rows, err := query(`SELECT config_key, config_val FROM sys_config`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Contains 判断字符串是否包含子串（供仓储层复用，避免额外 import）。
func Contains(s, sub string) bool { return strings.Contains(s, sub) }
