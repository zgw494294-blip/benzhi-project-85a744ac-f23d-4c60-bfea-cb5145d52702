package rigging

import "errors"

var (
	ErrNotFound        = errors.New("resource not found")
	ErrVersionConflict = errors.New("expected version conflict")
	ErrValidation      = errors.New("validation failed")
	ErrStateConflict   = errors.New("state transition conflict")
	ErrFrozen          = errors.New("approved plan is frozen")
	ErrIdempotency     = errors.New("idempotency key reused with another command")
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func Invalid(field, message string) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}
