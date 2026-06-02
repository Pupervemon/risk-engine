package domain

import (
	"fmt"
	"math"
)

// TrackPoint is a single mouse-track point submitted by the client.
type TrackPoint struct {
	X    int
	Y    int
	Time int64
}

// TrackValidationResult is the result of mouse-track validation.
type TrackValidationResult struct {
	Valid   bool
	Code    string
	Message string
}

type TrackValidationPolicy struct {
	Enabled        bool
	MinPoints      int
	MinDurationMs  int64
	MaxDurationMs  int64
	PointTolerance int
}

func (p TrackValidationPolicy) Validate(track []TrackPoint, answer SliderAnswer) TrackValidationResult {
	if !p.Enabled {
		return TrackValidationResult{Valid: true, Code: "OK", Message: "track validation disabled"}
	}

	if len(track) < p.MinPoints {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SHORT",
			Message: fmt.Sprintf("track points too few: %d < %d", len(track), p.MinPoints),
		}
	}

	if len(track) < 2 {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID",
			Message: "track must have at least 2 points",
		}
	}

	duration := track[len(track)-1].Time - track[0].Time
	if duration < p.MinDurationMs {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_FAST",
			Message: fmt.Sprintf("track duration too short: %dms < %dms", duration, p.MinDurationMs),
		}
	}

	if duration > p.MaxDurationMs {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_TOO_SLOW",
			Message: fmt.Sprintf("track duration too long: %dms > %dms", duration, p.MaxDurationMs),
		}
	}

	startX := track[0].X
	if startX > p.PointTolerance*2 {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_START",
			Message: fmt.Sprintf("invalid start position: x=%d", startX),
		}
	}

	endX := track[len(track)-1].X
	distanceToTarget := absInt(endX - answer.DX)
	if distanceToTarget > p.PointTolerance {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_END",
			Message: fmt.Sprintf("end position mismatch: %d vs %d (distance=%d)", endX, answer.DX, distanceToTarget),
		}
	}

	if !trackYStaysNearTarget(track, answer.DY, p.PointTolerance) {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_Y",
			Message: fmt.Sprintf("track y is too far from target y=%d", answer.DY),
		}
	}

	if !trackIsContinuous(track) {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_DISCONTINUOUS",
			Message: "track has discontinuous jumps",
		}
	}

	if !trackDirectionIsValid(track, answer.DX) {
		return TrackValidationResult{
			Valid:   false,
			Code:    "TRACK_INVALID_DIRECTION",
			Message: "track direction is invalid",
		}
	}

	return TrackValidationResult{Valid: true, Code: "OK", Message: "track validation passed"}
}

func trackIsContinuous(track []TrackPoint) bool {
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

func trackYStaysNearTarget(track []TrackPoint, targetY int, tolerance int) bool {
	for _, point := range track {
		if absInt(point.Y-targetY) > tolerance {
			return false
		}
	}
	return true
}

func trackDirectionIsValid(track []TrackPoint, targetX int) bool {
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
