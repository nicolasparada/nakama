package errs

import "fmt"

var (
	Unauthenticated = NewUnauthenticatedError("unauthenticated")
)

type Error struct {
	Kind    Kind
	Message string
	Field   *string
}

type Kind string

const (
	KindInvalidArgument  Kind = "invalid_argument"
	KindNotFound         Kind = "not_found"
	KindAlreadyExists    Kind = "already_exists"
	KindPermissionDenied Kind = "permission_denied"
	KindUnauthenticated  Kind = "unauthenticated"
)

func NewInvalidArgumentError(field, message string) *Error {
	return &Error{
		Kind:    KindInvalidArgument,
		Message: message,
		Field:   &field,
	}
}

func NewNotFoundError(message string) *Error {
	return &Error{
		Kind:    KindNotFound,
		Message: message,
	}
}

func NewAlreadyExistsError(field, message string) *Error {
	return &Error{
		Kind:    KindAlreadyExists,
		Message: message,
		Field:   &field,
	}
}

func NewPermissionDeniedError(message string) *Error {
	return &Error{
		Kind:    KindPermissionDenied,
		Message: message,
	}
}

func NewUnauthenticatedError(message string) *Error {
	return &Error{
		Kind:    KindUnauthenticated,
		Message: message,
	}
}

func (e *Error) Error() string {
	if e.Field != nil {
		return fmt.Sprintf("%s (field: %s): %s", e.Kind, *e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}
