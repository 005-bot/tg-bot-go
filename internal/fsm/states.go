package fsm

// aiogram-parity conversation state names (StatesGroup names as stored in
// Redis, e.g. class Filter with state filter -> "Filter:filter").
const (
	// FilterState is set while the user picks or confirms a street filter.
	FilterState = "Filter:filter"
	// FeedbackState is set while the user writes feedback.
	FeedbackState = "FeedbackState:feedback"
)
