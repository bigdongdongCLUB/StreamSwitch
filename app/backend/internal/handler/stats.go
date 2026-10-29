package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"livems/internal/pkg/response"
	"livems/internal/repository"
)

// StatsHandler 统计数据接口（仪表盘）。
type StatsHandler struct {
	trafficRepo *repository.TrafficRepo
}

// NewStatsHandler 创建统计处理器。
func NewStatsHandler() *StatsHandler {
	return &StatsHandler{trafficRepo: repository.NewTrafficRepo()}
}

// TrafficTrend 处理流量趋势查询。
//
// GET /api/v1/stats/traffic?minutes=60
//
// 返回最近 N 分钟的采样点（默认 60，最大 7 天）。
func (h *StatsHandler) TrafficTrend(c *gin.Context) {
	minutes, _ := strconv.Atoi(c.DefaultQuery("minutes", "60"))
	points, err := h.trafficRepo.Range(minutes)
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, points)
}
