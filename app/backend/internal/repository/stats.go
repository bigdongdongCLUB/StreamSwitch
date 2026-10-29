package repository

import (
	"time"
)

// TrafficStatPoint 一个流量采样点。
type TrafficStatPoint struct {
	TS            int64 `json:"ts"`
	RecvBps       int64 `json:"recv_bps"`       // 入流量速率（字节/秒）
	SendBps       int64 `json:"send_bps"`       // 出流量速率（字节/秒）
	OnlineStreams int   `json:"online_streams"` // 在线流数
	Readers       int   `json:"readers"`        // 观看人数
}

// TrafficRepo 流量采样仓储。
type TrafficRepo struct{}

// NewTrafficRepo 创建流量采样仓储。
func NewTrafficRepo() *TrafficRepo { return &TrafficRepo{} }

// trafficRetentionSec 采样数据保留时长（7 天）。
const trafficRetentionSec = 7 * 24 * 3600

// Insert 写入一个采样点，并顺带清理过期数据。
func (r *TrafficRepo) Insert(p *TrafficStatPoint) error {
	now := time.Now().Unix()
	if _, err := exec(
		`INSERT INTO traffic_stat (ts, recv_bps, send_bps, online_streams, readers, created_at) VALUES (?,?,?,?,?,?)`,
		p.TS, p.RecvBps, p.SendBps, p.OnlineStreams, p.Readers, now,
	); err != nil {
		return err
	}
	// 顺带清理过期数据（删除操作走 idx_ts 索引，开销可忽略）
	_, _ = exec(`DELETE FROM traffic_stat WHERE ts < ?`, now-trafficRetentionSec)
	return nil
}

// Range 查询最近 minutes 分钟的采样点（按时间升序）。
func (r *TrafficRepo) Range(minutes int) ([]TrafficStatPoint, error) {
	if minutes <= 0 {
		minutes = 60
	}
	if minutes > 7*24*60 {
		minutes = 7 * 24 * 60
	}
	since := time.Now().Unix() - int64(minutes)*60
	rows, err := query(
		`SELECT ts, recv_bps, send_bps, online_streams, readers FROM traffic_stat WHERE ts >= ? ORDER BY ts ASC`,
		since,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]TrafficStatPoint, 0, 512)
	for rows.Next() {
		var p TrafficStatPoint
		if err := rows.Scan(&p.TS, &p.RecvBps, &p.SendBps, &p.OnlineStreams, &p.Readers); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
