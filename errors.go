package cargo

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CargoError represents a framework error with context
type CargoError struct {
	Code    codes.Code
	Message string
	Cause   error
}

func (e *CargoError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *CargoError) GRPCStatus() *status.Status {
	return status.New(e.Code, e.Message)
}

// Error constructors
func NewValidationError(message string) *CargoError {
	return &CargoError{
		Code:    codes.InvalidArgument,
		Message: message,
	}
}

func NewNotFoundError(message string) *CargoError {
	return &CargoError{
		Code:    codes.NotFound,
		Message: message,
	}
}

func NewAuthenticationError(message string) *CargoError {
	return &CargoError{
		Code:    codes.Unauthenticated,
		Message: message,
	}
}

func NewAuthorizationError(message string) *CargoError {
	return &CargoError{
		Code:    codes.PermissionDenied,
		Message: message,
	}
}

func NewInternalError(message string) *CargoError {
	return &CargoError{
		Code:    codes.Internal,
		Message: message,
	}
}

func NewUnimplementedError(message string) *CargoError {
	return &CargoError{
		Code:    codes.Unimplemented,
		Message: message,
	}
}

// WrapError wraps an existing error with additional context
func WrapError(err error, code codes.Code, message string) *CargoError {
	return &CargoError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// ToGRPCError converts any error to a gRPC status error
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if cargoErr, ok := err.(*CargoError); ok {
		return cargoErr.GRPCStatus().Err()
	}

	// Default to internal error for unknown errors
	return status.Error(codes.Internal, err.Error())
}
