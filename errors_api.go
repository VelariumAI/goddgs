package goddgs

import (
	"errors"
	"fmt"
)

// ErrorKind is a stable machine-readable error category.
type ErrorKind string

const (
	ErrKindBlocked             ErrorKind = "blocked"
	ErrKindRateLimited         ErrorKind = "rate_limited"
	ErrKindProviderUnavailable ErrorKind = "provider_unavailable"
	ErrKindParse               ErrorKind = "parse"
	ErrKindInvalidInput        ErrorKind = "invalid_input"
	ErrKindNoResults           ErrorKind = "no_results"
	ErrKindInternal            ErrorKind = "internal"
)

// SearchError is a structured error returned by providers/engine.
type SearchError struct {
	Kind      ErrorKind
	Provider  string
	Temporary bool
	Cause     error
	Details   map[string]string
}

func (e *SearchError) Error() string {
	if e == nil {
		return "goddgs: <nil>"
	}
	if e.Provider == "" {
		if e.Cause != nil {
			return fmt.Sprintf("goddgs: %s: %v", e.Kind, e.Cause)
		}
		return fmt.Sprintf("goddgs: %s", e.Kind)
	}
	if e.Cause != nil {
		return fmt.Sprintf("goddgs: provider=%s kind=%s: %v", e.Provider, e.Kind, e.Cause)
	}
	return fmt.Sprintf("goddgs: provider=%s kind=%s", e.Provider, e.Kind)
}

func (e *SearchError) Unwrap() error { return e.Cause }

func classifyError(provider string, err error) *SearchError {
	if err == nil {
		return nil
	}
	var se *SearchError
	if errors.As(err, &se) {
		if se.Provider == "" {
			se.Provider = provider
		}
		return se
	}
	if IsBlocked(err) {
		out := &SearchError{Kind: ErrKindBlocked, Provider: provider, Temporary: true, Cause: err}
		var be *BlockedError
		if errors.As(err, &be) {
			out.Details = map[string]string{"detector": be.Event.Detector}
		}
		return out
	}
	if errors.Is(err, ErrNoResults) {
		return &SearchError{Kind: ErrKindNoResults, Provider: provider, Temporary: false, Cause: err}
	}
	if errors.Is(err, ErrNoVQD) || errors.Is(err, ErrUnexpectedBody) {
		return &SearchError{Kind: ErrKindParse, Provider: provider, Temporary: true, Cause: err}
	}
	return &SearchError{Kind: ErrKindProviderUnavailable, Provider: provider, Temporary: true, Cause: err}
}
