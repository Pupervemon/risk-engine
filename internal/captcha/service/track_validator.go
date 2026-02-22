package service

import (
	"fmt"
	"math"

	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"go.uber.org/zap"
)

// TrackPoint 轨迹点
type TrackPoint struct {
	X    int   `json:"x"`
	Y    int   `json:"y"`
	Time int64 `json:"t"` // 毫秒时间戳
}

// TrackValidator 轨迹校验器
type TrackValidator struct {
	config config.TrackValidationConfig
	logger *zap.Logger
}

// NewTrackValidator 创建轨迹校验器
func NewTrackValidator(cfg config.TrackValidationConfig, logger *zap.Logger) *TrackValidator {
	return &TrackValidator{
		config: cfg,
		logger: logger,
	}
}

// ValidationResult 校验结果
type ValidationResult struct {
	Valid   bool   // 是否有效
	Code    string // 错误码
	Message string // 描述信息
}

// Validate 校验轨迹数据
func (v *TrackValidator) Validate(track []TrackPoint, targetX int) ValidationResult {
	if !v.config.Enabled {
		return ValidationResult{Valid: true, Code: "OK", Message: "track validation disabled"}
	}

	// 1. 检查轨迹点数
	if len(track) < v.config.MinPoints {
		v.logger.Debug("轨迹点数不足",
			zap.Int("actual", len(track)),
			zap.Int("required", v.config.MinPoints))
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SHORT",
			Message: fmt.Sprintf("track points too few: %d < %d", len(track), v.config.MinPoints),
		}
	}

	// 2. 检查轨迹时长
	if len(track) < 2 {
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID",
			Message: "track must have at least 2 points",
		}
	}

	duration := track[len(track)-1].Time - track[0].Time

	if duration < v.config.MinDurationMs {
		v.logger.Debug("轨迹时长过短",
			zap.Int64("duration_ms", duration),
			zap.Int64("min_ms", v.config.MinDurationMs))
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_FAST",
			Message: fmt.Sprintf("track duration too short: %dms < %dms", duration, v.config.MinDurationMs),
		}
	}

	if duration > v.config.MaxDurationMs {
		v.logger.Debug("轨迹时长过长",
			zap.Int64("duration_ms", duration),
			zap.Int64("max_ms", v.config.MaxDurationMs))
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SLOW",
			Message: fmt.Sprintf("track duration too long: %dms > %dms", duration, v.config.MaxDurationMs),
		}
	}

	// 3. 检查起点位置（应该接近左侧）
	startX := track[0].X
	if startX > v.config.PointTolerance*2 {
		v.logger.Debug("起点位置异常",
			zap.Int("start_x", startX),
			zap.Int("tolerance", v.config.PointTolerance))
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_START",
			Message: fmt.Sprintf("invalid start position: x=%d", startX),
		}
	}

	// 4. 检查终点位置（应该接近目标X坐标）
	endX := track[len(track)-1].X
	distanceToTarget := abs(endX - targetX)
	if distanceToTarget > v.config.PointTolerance {
		v.logger.Debug("终点位置与目标偏差过大",
			zap.Int("end_x", endX),
			zap.Int("target_x", targetX),
			zap.Int("distance", distanceToTarget),
			zap.Int("tolerance", v.config.PointTolerance))
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_END",
			Message: fmt.Sprintf("end position mismatch: %d vs %d (distance=%d)", endX, targetX, distanceToTarget),
		}
	}

	// 5. 检查轨迹连续性（相邻点不应该有突然的大跳跃）
	if !v.checkContinuity(track) {
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_DISCONTINUOUS",
			Message: "track has discontinuous jumps",
		}
	}

	// 6. 检查轨迹方向（应该大致向右移动）
	if !v.checkDirection(track, targetX) {
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_DIRECTION",
			Message: "track direction is invalid",
		}
	}

	v.logger.Debug("轨迹校验通过",
		zap.Int("points", len(track)),
		zap.Int64("duration_ms", duration),
		zap.Int("start_x", startX),
		zap.Int("end_x", endX),
		zap.Int("target_x", targetX))

	return ValidationResult{Valid: true, Code: "OK", Message: "track validation passed"}
}

// checkContinuity 检查轨迹连续性
func (v *TrackValidator) checkContinuity(track []TrackPoint) bool {
	maxJump := 100 // 相邻点最大跳跃距离（像素）

	for i := 1; i < len(track); i++ {
		dx := track[i].X - track[i-1].X
		dy := track[i].Y - track[i-1].Y
		distance := math.Sqrt(float64(dx*dx + dy*dy))

		if distance > float64(maxJump) {
			v.logger.Debug("检测到轨迹跳跃",
				zap.Int("index", i),
				zap.Float64("distance", distance),
				zap.Int("max", maxJump))
			return false
		}

		// 检查时间戳单调递增
		if track[i].Time < track[i-1].Time {
			v.logger.Debug("时间戳逆序",
				zap.Int("index", i),
				zap.Int64("prev_time", track[i-1].Time),
				zap.Int64("curr_time", track[i].Time))
			return false
		}
	}

	return true
}

// checkDirection 检查轨迹方向
func (v *TrackValidator) checkDirection(track []TrackPoint, targetX int) bool {
	// 统计向右移动的总距离
	rightwardDistance := 0
	leftwardDistance := 0

	for i := 1; i < len(track); i++ {
		dx := track[i].X - track[i-1].X
		if dx > 0 {
			rightwardDistance += dx
		} else {
			leftwardDistance += abs(dx)
		}
	}

	// 主要方向应该向右
	if rightwardDistance < targetX/2 {
		v.logger.Debug("向右移动距离不足",
			zap.Int("rightward", rightwardDistance),
			zap.Int("leftward", leftwardDistance),
			zap.Int("target", targetX))
		return false
	}

	// 回退距离不应该过多（允许少量修正）
	if leftwardDistance > rightwardDistance/3 {
		v.logger.Debug("回退距离过多",
			zap.Int("leftward", leftwardDistance),
			zap.Int("rightward", rightwardDistance))
		return false
	}

	return true
}

// ValidateSimple 简化版校验（仅检查基础指标）
func (v *TrackValidator) ValidateSimple(track []TrackPoint) ValidationResult {
	if !v.config.Enabled {
		return ValidationResult{Valid: true, Code: "OK", Message: "track validation disabled"}
	}

	// 仅检查点数和时长
	if len(track) < v.config.MinPoints {
		return ValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SHORT",
			Message: fmt.Sprintf("track points too few: %d < %d", len(track), v.config.MinPoints),
		}
	}

	if len(track) >= 2 {
		duration := track[len(track)-1].Time - track[0].Time
		if duration < v.config.MinDurationMs || duration > v.config.MaxDurationMs {
			return ValidationResult{
				Valid:   false,
				Code:    "TRACK_INVALID_DURATION",
				Message: fmt.Sprintf("invalid duration: %dms", duration),
			}
		}
	}

	return ValidationResult{Valid: true, Code: "OK", Message: "simple validation passed"}
}

// abs 绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetStats 获取轨迹统计信息（用于调试和监控）
func (v *TrackValidator) GetStats(track []TrackPoint) map[string]interface{} {
	if len(track) < 2 {
		return map[string]interface{}{
			"points":   len(track),
			"duration": 0,
		}
	}

	duration := track[len(track)-1].Time - track[0].Time
	totalDistance := 0.0

	for i := 1; i < len(track); i++ {
		dx := track[i].X - track[i-1].X
		dy := track[i].Y - track[i-1].Y
		totalDistance += math.Sqrt(float64(dx*dx + dy*dy))
	}

	avgSpeed := 0.0
	if duration > 0 {
		avgSpeed = totalDistance / float64(duration) * 1000 // 像素/秒
	}

	return map[string]interface{}{
		"points":         len(track),
		"duration_ms":    duration,
		"total_distance": totalDistance,
		"avg_speed":      avgSpeed,
		"start_x":        track[0].X,
		"start_y":        track[0].Y,
		"end_x":          track[len(track)-1].X,
		"end_y":          track[len(track)-1].Y,
	}
}
