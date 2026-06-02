package domain

// SliderChallenge is the captcha challenge returned to a client.
type SliderChallenge struct {
	CaptchaID         string
	MasterImage       string
	TileImage         string
	TargetY           int
	ExpiresIn         int
	RequireMouseTrack bool
}

// SliderAnswer is the stored answer for a slider captcha.
type SliderAnswer struct {
	DX int
	DY int
}

func (a SliderAnswer) Matches(pointX, pointY, tolerance int) bool {
	rangeSize := tolerance * 2
	minX := a.DX - tolerance
	minY := a.DY - tolerance

	return pointX >= minX &&
		pointX <= minX+rangeSize &&
		pointY >= minY &&
		pointY <= minY+rangeSize
}

// GeneratedSlider is the normalized output of a slider generator adapter.
type GeneratedSlider struct {
	MasterImage string
	TileImage   string
	Answer      SliderAnswer
	TargetY     int
}
