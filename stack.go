package ruleevaluator


type stack[T any] struct {
	data []T
}

func newStack[T any]() *stack[T] {
	return &stack[T]{
		data: make([]T, 0),
	}
}

func (s *stack[T]) Push(v T) {
	s.data = append(s.data, v)
}

func (s *stack[T]) Pop() (T, error) {
	if len(s.data) == 0 {
		var zero T
		return zero, newEmptyStackError()
	}
	v := s.data[len(s.data)-1]
	s.data = s.data[:len(s.data)-1]
	return v, nil
}

func (s *stack[T]) Peek() (T, error) {
	if len(s.data) == 0 {
		var zero T
		return zero, newEmptyStackError()
	}
	return s.data[len(s.data)-1], nil
}

func (s *stack[T]) Len() int {
	return len(s.data)
}
