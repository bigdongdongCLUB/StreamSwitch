// Package repository 封装数据访问层，隔离 SQL 语句。
package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"livems/internal/models"
)

// StreamRepo 推流管理仓储。
type StreamRepo struct{}

// NewStreamRepo 创建推流仓储。
func NewStreamRepo() *StreamRepo { return &StreamRepo{} }

// streamColumns 统一的列清单，避免查询遗漏字段。
const streamColumns = `id, name, app, stream, vhost, push_key, status, remark, online, publish_ip,
	publish_time, offline_time, video_codec, audio_codec, width, height, fps, bitrate, video_bitrate,
	audio_bitrate, sample_rate, reader_count, total_reader, total_bytes, created_by, created_at, updated_at`

// Create 新增一条流。
func (r *StreamRepo) Create(s *models.Stream) error {
	res, err := exec(`
		INSERT INTO stream (name, app, stream, vhost, push_key, status, remark, online, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		s.Name, s.App, s.Stream, s.Vhost, s.PushKey, s.Status, s.Remark, s.Online, s.CreatedBy, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	s.ID = uint64(id)
	return nil
}

// Update 更新一条流。
func (r *StreamRepo) Update(s *models.Stream) error {
	_, err := exec(`
		UPDATE stream SET name=?, app=?, stream=?, vhost=?, push_key=?, status=?, remark=?,
			updated_at=? WHERE id=?`,
		s.Name, s.App, s.Stream, s.Vhost, s.PushKey, s.Status, s.Remark, s.UpdatedAt, s.ID)
	return err
}

// UpdateStatus 更新启用/禁用状态。
func (r *StreamRepo) UpdateStatus(id uint64, status int) error {
	_, err := exec(`UPDATE stream SET status=?, updated_at=? WHERE id=?`, status, time.Now().Unix(), id)
	return err
}

// Delete 删除一条流。
func (r *StreamRepo) Delete(id uint64) error {
	_, err := exec(`DELETE FROM stream WHERE id=?`, id)
	return err
}

// GetByID 按ID查询。
func (r *StreamRepo) GetByID(id uint64) (*models.Stream, error) {
	row := queryRow(`SELECT `+streamColumns+` FROM stream WHERE id=?`, id)
	s, err := scanStream(row)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// GetByStream 按 app+stream 查询（推流鉴权与 Hook 处理常用）。
func (r *StreamRepo) GetByStream(app, stream string) (*models.Stream, error) {
	row := queryRow(`SELECT `+streamColumns+` FROM stream WHERE app=? AND stream=? LIMIT 1`, app, stream)
	s, err := scanStream(row)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// List 分页查询流列表。keyword 匹配名称/流ID，status<0 表示不限。
func (r *StreamRepo) List(page, pageSize int, keyword string, status int) ([]models.Stream, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if keyword != "" {
		where = append(where, "(name LIKE ? OR stream LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	if status >= 0 {
		where = append(where, "status=?")
		args = append(args, status)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM stream WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := query(`SELECT `+streamColumns+` FROM stream WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	list := make([]models.Stream, 0, pageSize)
	for rows.Next() {
		s, err := scanStream(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *s)
	}
	return list, total, rows.Err()
}

// AllOnline 获取所有处于在线状态的流。
func (r *StreamRepo) AllOnline() ([]models.Stream, error) {
	rows, err := query(`SELECT `+streamColumns+` FROM stream WHERE online=1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.Stream, 0)
	for rows.Next() {
		s, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	return list, rows.Err()
}

// All 获取全部流。
func (r *StreamRepo) All() ([]models.Stream, error) {
	rows, err := query(`SELECT ` + streamColumns + ` FROM stream ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.Stream, 0)
	for rows.Next() {
		s, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	return list, rows.Err()
}

// MarkOnline 标记流上线。
func (r *StreamRepo) MarkOnline(id uint64, ip string, at int64) error {
	_, err := exec(`UPDATE stream SET online=1, publish_ip=?, publish_time=?, offline_time=0, updated_at=? WHERE id=?`,
		ip, at, at, id)
	return err
}

// MarkOffline 标记流下线。
//
// 下线即无人观看、无码率：一并清零实时字段，否则同步最后一次写入的 reader_count/bitrate
// 会一直残留，仪表盘把它累加进「当前观看人数」。
func (r *StreamRepo) MarkOffline(id uint64, at int64) error {
	_, err := exec(`UPDATE stream SET online=0, offline_time=?, updated_at=?,
		reader_count=0, bitrate=0, video_bitrate=0, audio_bitrate=0 WHERE id=?`, at, at, id)
	return err
}

// ClearLiveStats 清零离线流上残留的实时统计（观看数/码率），不改动在线状态与时间。
func (r *StreamRepo) ClearLiveStats(id uint64) error {
	_, err := exec(`UPDATE stream SET reader_count=0, bitrate=0, video_bitrate=0, audio_bitrate=0, updated_at=? WHERE id=?`,
		time.Now().Unix(), id)
	return err
}

// UpdateMediaInfo 更新流的实时媒体信息（编码/分辨率/码率等）。
func (r *StreamRepo) UpdateMediaInfo(id uint64, info models.Stream) error {
	_, err := exec(`
		UPDATE stream SET video_codec=?, audio_codec=?, width=?, height=?, fps=?, bitrate=?,
			video_bitrate=?, audio_bitrate=?, sample_rate=?, reader_count=?, total_bytes=?, updated_at=?
		WHERE id=?`,
		info.VideoCodec, info.AudioCodec, info.Width, info.Height, info.FPS, info.Bitrate,
		info.VideoBitrate, info.AudioBitrate, info.SampleRate, info.ReaderCount, info.TotalBytes,
		time.Now().Unix(), id)
	return err
}

// IncReader 累加观看人次。
func (r *StreamRepo) IncReader(id uint64) error {
	_, err := exec(`UPDATE stream SET total_reader = total_reader + 1 WHERE id=?`, id)
	return err
}

// scanStream 从一行结果中扫描流记录。
func scanStream(row scanner) (*models.Stream, error) {
	var s models.Stream
	err := row.Scan(&s.ID, &s.Name, &s.App, &s.Stream, &s.Vhost, &s.PushKey, &s.Status, &s.Remark,
		&s.Online, &s.PublishIP, &s.PublishTime, &s.OfflineTime, &s.VideoCodec, &s.AudioCodec,
		&s.Width, &s.Height, &s.FPS, &s.Bitrate, &s.VideoBitrate, &s.AudioBitrate, &s.SampleRate,
		&s.ReaderCount, &s.TotalReader, &s.TotalBytes, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// scanner 抽象 *sql.Row 与 *sql.Rows 的公共 Scan 能力。
type scanner interface {
	Scan(dest ...any) error
}

// ensure sql 包被引用（Row/Rows 都实现了 scanner）。
var _ scanner = (*sql.Row)(nil)
var _ scanner = (*sql.Rows)(nil)

// fmtErr 统一包装 SQL 错误信息，便于定位。
func fmtErr(op string, err error) error {
	return fmt.Errorf("%s: %w", op, err)
}
