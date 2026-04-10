package ruleevaluator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStack_Push(t *testing.T) {
	s := newStack[int]()
	s.Push(1)
	s.Push(2)

	assert.Equal(t, 2, s.Len())
	val, err := s.Peek()
	assert.NoError(t, err)
	assert.Equal(t, 2, val)
}

func TestStack_Pop(t *testing.T) {
	s := newStack[string]()
	s.Push("first")
	s.Push("second")

	val, err := s.Pop()
	assert.NoError(t, err)
	assert.Equal(t, "second", val)
	assert.Equal(t, 1, s.Len())

	val, err = s.Pop()
	assert.NoError(t, err)
	assert.Equal(t, "first", val)
	assert.Equal(t, 0, s.Len())

	// Empty stack pop — expect a typed EvaluatorError with CodeEmptyStack
	val, err = s.Pop()
	assert.Error(t, err)
	var evalErr *EvaluatorError
	assert.True(t, errors.As(err, &evalErr))
	assert.Equal(t, CodeEmptyStack, evalErr.Code)
	assert.Equal(t, "", val)
}

func TestStack_Peek(t *testing.T) {
	s := newStack[float64]()

	// Empty stack peek — expect a typed EvaluatorError with CodeEmptyStack
	val, err := s.Peek()
	assert.Error(t, err)
	var evalErr *EvaluatorError
	assert.True(t, errors.As(err, &evalErr))
	assert.Equal(t, CodeEmptyStack, evalErr.Code)
	assert.Equal(t, 0.0, val)

	s.Push(1.5)
	val, err = s.Peek()
	assert.NoError(t, err)
	assert.Equal(t, 1.5, val)
	assert.Equal(t, 1, s.Len())
}

func TestStack_Len(t *testing.T) {
	s := newStack[int]()
	assert.Equal(t, 0, s.Len())

	s.Push(10)
	assert.Equal(t, 1, s.Len())

	s.Pop()
	assert.Equal(t, 0, s.Len())
}
