package application

import (
	"fmt"
	"math"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

func validateTrack(opts TrackValidationOptions, track []domain.TrackPoint, targetX int, targetY int) domain.TrackValidationResult {
	if !opts.Enabled {
		return domain.TrackValidationResult{Valid: true, Code: "OK", Message: "track validation disabled"}
	}

	if len(track) < opts.MinPoints {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SHORT",
			Message: fmt.Sprintf("track points too few: %d < %d", len(track), opts.MinPoints),
		}
	}

	if len(track) < 2 {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID",
			Message: "track must have at least 2 points",
		}
	}

	duration := track[len(track)-1].Time - track[0].Time
	if duration < opts.MinDurationMs {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_FAST",
			Message: fmt.Sprintf("track duration too short: %dms < %dms", duration, opts.MinDurationMs),
		}
	}

	if duration > opts.MaxDurationMs {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SLOW",
			Message: fmt.Sprintf("track duration too long: %dms > %dms", duration, opts.MaxDurationMs),
		}
	}

	startX := track[0].X
	if startX > opts.PointTolerance*2 { //起点位置检测
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_START",
			Message: fmt.Sprintf("invalid start position: x=%d", startX),
		}
	}

	endX := track[len(track)-1].X
	distanceToTarget := absInt(endX - targetX)
	if distanceToTarget > opts.PointTolerance {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_END",
			Message: fmt.Sprintf("end position mismatch: %d vs %d (distance=%d)", endX, targetX, distanceToTarget),
		}
	}

	if !trackYStaysNearTarget(track, targetY, opts.PointTolerance) {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_Y",
			Message: fmt.Sprintf("track y is too far from target y=%d", targetY),
		}
	}

	if !trackIsContinuous(track) {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_DISCONTINUOUS",
			Message: "track has discontinuous jumps",
		}
	}

	if !trackDirectionIsValid(track, targetX) {
		return domain.TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_DIRECTION",
			Message: "track direction is invalid",
		}
	}

	return domain.TrackValidationResult{Valid: true, Code: "OK", Message: "track validation passed"}
}

func trackIsContinuous(track []domain.TrackPoint) bool {
	const maxJump = 100

	for i := 1; i < len(track); i++ {
		dx := track[i].X - track[i-1].X
		dy := track[i].Y - track[i-1].Y
		distance := math.Sqrt(float64(dx*dx + dy*dy))
		if distance > maxJump {
			return false
		}
		if track[i].Time < track[i-1].Time {
			return false
		}
	}

	return true
}

func trackYStaysNearTarget(track []domain.TrackPoint, targetY int, tolerance int) bool {
	for _, point := range track {
		if absInt(point.Y-targetY) > tolerance {
			return false
		}
	}
	return true
}

func trackDirectionIsValid(track []domain.TrackPoint, targetX int) bool {
	rightwardDistance := 0
	leftwardDistance := 0

	for i := 1; i < len(track); i++ {
		dx := track[i].X - track[i-1].X
		if dx > 0 {
			rightwardDistance += dx
		} else {
			leftwardDistance += absInt(dx)
		}
	}

	if rightwardDistance < targetX/2 {
		return false
	}
	if leftwardDistance > rightwardDistance/3 {
		return false
	}

	return true
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
