package storage

// Filter represents a user's street subscription filter.
//
// The JSON encoding MUST match the Python pydantic Filter model byte for byte:
// {"street": "..."} or {"street": null}. The pointer field has no omitempty so
// a nil street marshals to explicit null.
type Filter struct {
	Street *string `json:"street"`
}
