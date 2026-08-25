// Package boterr defines user-facing error types for the Telegram bot.
// Handlers wrap domain failures with these types; the global error
// middleware maps them to verbatim Russian user messages (aiogram
// error-handler parity).
package boterr

import "fmt"

// UserInputError marks errors caused by invalid user input. The middleware
// replies with "⚠️ Ошибка ввода: {message}".
type UserInputError struct {
	err error
}

// Error implements the error interface.
func (e UserInputError) Error() string {
	if e.err == nil {
		return "user input error"
	}
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e UserInputError) Unwrap() error {
	return e.err
}

// NewUserInputError wraps err as a UserInputError.
func NewUserInputError(err error) error {
	return UserInputError{err: err}
}

// NewUserInputErrorf builds a UserInputError from a format string.
func NewUserInputErrorf(format string, args ...any) error {
	return UserInputError{err: fmt.Errorf(format, args...)}
}

// APIError marks temporary service or API failures. The middleware replies
// with "🔧 Временная проблема с сервисом, пожалуйста, попробуйте позже".
type APIError struct {
	err error
}

// Error implements the error interface.
func (e APIError) Error() string {
	if e.err == nil {
		return "api error"
	}
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e APIError) Unwrap() error {
	return e.err
}

// NewAPIError wraps err as an APIError.
func NewAPIError(err error) error {
	return APIError{err: err}
}
