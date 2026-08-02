package cmd

import (
	"errors"
	"fmt"
)

// Process exit statuses. run documents these in its help; keep the two in sync.
const (
	exitOK               = 0
	exitError            = 1
	exitUsage            = 2
	exitAPIError         = 3
	exitPermissionDenied = 4
	exitMaxSteps         = 5
	exitInterrupted      = 130
)

// exitOverride lets a command request a non-zero exit status on an otherwise
// successful (nil-error) run — e.g. run completing with tool calls blocked by
// the permission gate.
var exitOverride int

// codedError carries an explicit process exit status alongside an error.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// coded wraps err with an exit status, returning nil when err is nil.
func coded(code int, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, err: err}
}

// codedf builds a coded error from a format string.
func codedf(code int, format string, a ...any) error {
	return &codedError{code: code, err: fmt.Errorf(format, a...)}
}

// ExitCode maps the result of Execute to a process exit status: the status
// carried by a codedError, exitError for any other error, or the exitOverride
// (default 0) when there is no error.
func ExitCode(err error) int {
	if err == nil {
		return exitOverride
	}
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return exitError
}
