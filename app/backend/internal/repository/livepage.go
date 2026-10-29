package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"livems/internal/models"
)

// LivePageRepo 直播分享页仓储。
type LivePageRepo struct{}

// NewLivePageRepo 创建直播分享页仓储。
func NewLivePageRepo() *LivePageRepo { return &LivePageRepo{} }

const livePageCols = `id, name, code, stream_id, password, announcement, status, view_count, created_by, created_at, updated_at`

// Create 新增直播页。
func (r *LivePageRepo) Create(p *models.LivePage) error {
	now := time.Now().Unix()
	res, err := exec(`
		INSERT INTO live_page (name, code, stream_id, password, announcement, status, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		p.Name, p.Code, p.StreamID, p.Password, p.Announcement, p.Status, p.CreatedBy, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	p.ID = uint64(id)
	return nil
}

// Update 更新直播页。
func (r *LivePageRepo) Update(p *models.LivePage) error {
	_, err := exec(`
		UPDATE live_page SET name=?, stream_id=?, password=?, announcement=?, status=?, updated_at=? WHERE id=?`,
		p.Name, p.StreamID, p.Password, p.Announcement, p.Status, time.Now().Unix(), p.ID)
	return err
}

// Delete 删除直播页。
func (r *LivePageRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM live_page WHERE id=?`, id)
	return err
}

// IncrView 累加访问次数。
func (r *LivePageRepo) IncrView(id uint64) {
	_, _ = exec(`UPDATE live_page SET view_count=view_count+1 WHERE id=?`, id)
}

func (r *LivePageRepo) scanOne(row *sql.Row) (*models.LivePage, error) {
	var p models.LivePage
	err := row.Scan(&p.ID, &p.Name, &p.Code, &p.StreamID, &p.Password, &p.Announcement,
		&p.Status, &p.ViewCount, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("直播页不存在")
		}
		return nil, err
	}
	return &p, nil
}

// GetByID 按 ID 查询。
func (r *LivePageRepo) GetByID(id uint64) (*models.LivePage, error) {
	return r.scanOne(queryRow(`SELECT `+livePageCols+` FROM live_page WHERE id=?`, id))
}

// GetByCode 按访问码查询。
func (r *LivePageRepo) GetByCode(code string) (*models.LivePage, error) {
	return r.scanOne(queryRow(`SELECT `+livePageCols+` FROM live_page WHERE code=?`, code))
}

// List 分页查询。
func (r *LivePageRepo) List(page, pageSize int, keyword string) ([]models.LivePage, int64, error) {
	where := ` WHERE 1=1`
	var args []any
	if strings.TrimSpace(keyword) != "" {
		where += ` AND (name LIKE ? OR code LIKE ?)`
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}
	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM live_page`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	rows, err := query(`SELECT `+livePageCols+` FROM live_page`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []models.LivePage{}
	for rows.Next() {
		var p models.LivePage
		if err := rows.Scan(&p.ID, &p.Name, &p.Code, &p.StreamID, &p.Password, &p.Announcement,
			&p.Status, &p.ViewCount, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}
