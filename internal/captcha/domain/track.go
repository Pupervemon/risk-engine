package domain

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
