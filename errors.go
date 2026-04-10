package ruleevaluator

// ErrorCode identifies the category of a rule evaluator error.
type ErrorCode string

const (
	// CodeEmptyStack is returned when a Pop or Peek is attempted on an empty stack.
	CodeEmptyStack ErrorCode = "EMPTY_STACK"
)

// EvaluatorError is the typed error for rule evaluator failures.
// Callers should use errors.As to inspect the Code and act accordingly.
type EvaluatorError struct {
	Code    ErrorCode
	Message string
}

// Error implements the error interface.
func (e *EvaluatorError) Error() string {
	return string(e.Code) + ": " + e.Message
}

func newEvaluatorError(code ErrorCode, msg string) *EvaluatorError {
	return &EvaluatorError{Code: code, Message: msg}
}

// newEmptyStackError returns an EvaluatorError indicating an operation was
// attempted on an empty stack.
func newEmptyStackError() *EvaluatorError {
	return newEvaluatorError(CodeEmptyStack, "empty stack")
}
