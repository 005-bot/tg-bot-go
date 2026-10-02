package webhook

// Response is the body returned when an update is accepted for processing.
type Response struct {
	OK bool `json:"ok"`
}

// ErrorResponse is the body returned when the update cannot be decoded.
type ErrorResponse struct {
	Error string `json:"error"`
}
