package ruleevaluator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluatorError_Error(t *testing.T) {
	err := &EvaluatorError{Code: CodeEmptyStack, Message: "empty stack"}
	assert.Equal(t, "EMPTY_STACK: empty stack", err.Error())
}

func TestEvaluatorError_ErrorsAs(t *testing.T) {
	err := newEmptyStackError()
	var ee *EvaluatorError
	require.True(t, errors.As(err, &ee))
	assert.Equal(t, CodeEmptyStack, ee.Code)
	assert.Equal(t, "empty stack", ee.Message)
}

func TestNewEmptyStackError_Fields(t *testing.T) {
	err := newEmptyStackError()
	assert.Equal(t, CodeEmptyStack, err.Code)
	assert.Equal(t, "empty stack", err.Message)
	assert.Equal(t, "EMPTY_STACK: empty stack", err.Error())
}
