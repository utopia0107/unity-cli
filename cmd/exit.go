package cmd

import (
	"errors"

	"github.com/youngwoocho02/unity-cli/internal/client"
)

// ExitCode classifies why the CLI exited. Documented in README (Exit Codes).
type ExitCode int

const (
	ExitOK              ExitCode = 0 // success
	ExitCommandFailed   ExitCode = 1 // Unity executed the command but reported failure
	ExitUsage           ExitCode = 2 // invalid flags/arguments, before contacting Unity
	ExitConnection      ExitCode = 3 // Unity not running / unreachable / ambiguous instance
	ExitVersionMismatch ExitCode = 4 // CLI and connector versions differ
	ExitTimeout         ExitCode = 5 // deadline exceeded while waiting or polling
)

// CodedError attaches an ExitCode to an error.
type CodedError struct {
	Code ExitCode
	Err  error
}

func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

// withCode tags err with an exit code; nil stays nil.
func withCode(code ExitCode, err error) error {
	if err == nil {
		return nil
	}
	return &CodedError{Code: code, Err: err}
}

// errReported marks errors whose message was already printed to the user
// (e.g. by printResponse), so main must not print it again.
var errReported = errors.New("error already reported")

// IsReported reports whether the error output was already emitted.
func IsReported(err error) bool { return errors.Is(err, errReported) }

// ExitCodeFor maps an error to the process exit code. The outermost
// CodedError wins; untagged connection errors classify as ExitConnection.
func ExitCodeFor(err error) ExitCode {
	if err == nil {
		return ExitOK
	}
	var coded *CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}
	if errors.Is(err, client.ErrConnection) {
		return ExitConnection
	}
	return ExitCommandFailed
}
