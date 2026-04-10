package ruleevaluator

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type tokenType int

const (
	tokEq tokenType = iota
	tokNe
	tokGt
	tokGe
	tokLt
	tokLe
	tokAnd
	tokOr
	tokLParen
	tokNot
	tokElse // :
	tokFunc
)

const (
	errMsgUnknownComparisonOp    = "unknown comparison operator: %v"
	errMsgInsufficientForTernary = "insufficient values for ternary operator"
	errMsgInsufficientForOp      = "insufficient values for operator: %d"
)

// EvaluatorFunc is the signature for custom functions registered with RegisterFunction.
type EvaluatorFunc func(args []any) (any, error)

// RuleEvaluator evaluates string expressions against a bound data object.
type RuleEvaluator struct {
	data  any
	funcs map[string]EvaluatorFunc
}

type funcMarker struct {
	fn EvaluatorFunc
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isIdentChar(b byte) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}
	switch b {
	case '_', '.', '$', '[', ']', '-':
		return true
	}
	return false
}

// NewRuleEvaluator creates a new RuleEvaluator bound to data.
func NewRuleEvaluator(data any) *RuleEvaluator {
	return &RuleEvaluator{
		data:  data,
		funcs: make(map[string]EvaluatorFunc),
	}
}

// RegisterFunction registers a named function callable from expressions.
func (e *RuleEvaluator) RegisterFunction(name string, fn EvaluatorFunc) {
	e.funcs[name] = fn
}

// Evaluate evaluates expression against the bound data with no variables.
func (e *RuleEvaluator) Evaluate(expression string) (any, error) {
	return e.EvaluateWithVars(expression, nil)
}

// --- comparison helpers ---

// compareNil handles nil operands — only == and != are permitted.
func compareNil(left, right any, op tokenType) (bool, error) {
	switch op {
	case tokEq:
		return left == nil && right == nil, nil
	case tokNe:
		return !(left == nil && right == nil), nil
	default:
		return false, fmt.Errorf("cannot use operator %v with nil", op)
	}
}

// compareBools applies a comparison operator to two bool values.
func compareBools(l, r bool, op tokenType) (bool, error) {
	switch op {
	case tokEq:
		return l == r, nil
	case tokNe:
		return l != r, nil
	case tokGt:
		return l && !r, nil
	case tokLt:
		return !l && r, nil
	case tokGe:
		return l == r || (l && !r), nil
	case tokLe:
		return l == r || (!l && r), nil
	default:
		return false, fmt.Errorf(errMsgUnknownComparisonOp, op)
	}
}

// applyOrderedOp applies a comparison operator to two ordered (int64, float64, or string) values.
func applyOrderedOp[T int64 | float64 | string](l, r T, op tokenType) (bool, error) {
	switch op {
	case tokEq:
		return l == r, nil
	case tokNe:
		return l != r, nil
	case tokGt:
		return l > r, nil
	case tokLt:
		return l < r, nil
	case tokGe:
		return l >= r, nil
	case tokLe:
		return l <= r, nil
	default:
		return false, fmt.Errorf(errMsgUnknownComparisonOp, op)
	}
}

func compare(left, right any, operator tokenType) (bool, error) {
	if left == nil || right == nil {
		return compareNil(left, right, operator)
	}

	lt, rt := reflect.TypeOf(left), reflect.TypeOf(right)
	if lt != rt {
		return false, fmt.Errorf("cannot compare different types: %v and %v", lt, rt)
	}

	switch l := left.(type) {
	case bool:
		return compareBools(l, right.(bool), operator)
	case string:
		return applyOrderedOp(l, right.(string), operator)
	case int64:
		return applyOrderedOp(l, right.(int64), operator)
	case float64:
		return applyOrderedOp(l, right.(float64), operator)
	default:
		return false, fmt.Errorf("type %v is not comparable", lt)
	}
}

// --- processOp helpers ---

func processUnaryNot(values *stack[any]) error {
	v, err := values.Pop()
	if err != nil {
		return fmt.Errorf("insufficient values for unary NOT")
	}
	bv, ok := v.(bool)
	if !ok {
		return fmt.Errorf("invalid type for unary NOT: %T", v)
	}
	values.Push(!bv)
	return nil
}

func processTernaryOp(values *stack[any]) error {
	fv, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForTernary)
	}
	tv, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForTernary)
	}
	cond, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForTernary)
	}
	b, ok := cond.(bool)
	if !ok {
		return fmt.Errorf("ternary condition must be bool, got %T", cond)
	}
	if b {
		values.Push(tv)
	} else {
		values.Push(fv)
	}
	return nil
}

func processLogicalOp(values *stack[any], op tokenType) error {
	right, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForOp, op)
	}
	left, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForOp, op)
	}
	lb, ok1 := left.(bool)
	rb, ok2 := right.(bool)
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid types for logical operator: %T and %T", left, right)
	}
	if op == tokAnd {
		values.Push(lb && rb)
	} else {
		values.Push(lb || rb)
	}
	return nil
}

func processComparisonOp(values *stack[any], op tokenType) error {
	right, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForOp, op)
	}
	left, err := values.Pop()
	if err != nil {
		return fmt.Errorf(errMsgInsufficientForOp, op)
	}
	b, err := compare(left, right, op)
	if err != nil {
		return err
	}
	values.Push(b)
	return nil
}

func processOp(values *stack[any], operator tokenType) error {
	switch operator {
	case tokNot:
		return processUnaryNot(values)
	case tokElse:
		return processTernaryOp(values)
	case tokFunc:
		return processFunc(values)
	case tokAnd, tokOr:
		return processLogicalOp(values, operator)
	case tokEq, tokNe, tokGt, tokGe, tokLt, tokLe:
		return processComparisonOp(values, operator)
	default:
		return fmt.Errorf("unknown operator: %v", operator)
	}
}

func processParen(values *stack[any], ops *stack[tokenType]) error {
	for ops.Len() > 0 {
		peek, _ := ops.Peek()
		if peek == tokLParen {
			break
		}
		op, _ := ops.Pop()
		if err := processOp(values, op); err != nil {
			return err
		}
	}

	peek, err := ops.Peek()
	if err != nil || peek != tokLParen {
		return fmt.Errorf("mismatched parentheses")
	}
	_, _ = ops.Pop()

	if ops.Len() > 0 {
		op, _ := ops.Peek()
		if op == tokNot || op == tokFunc {
			op, _ = ops.Pop()
			if err := processOp(values, op); err != nil {
				return err
			}
		}
	}

	return nil
}

func processFunc(values *stack[any]) error {
	if values.Len() == 0 {
		return fmt.Errorf("function call with empty value stack")
	}

	argsReversed := make([]any, 0)
	for values.Len() > 0 {
		v, _ := values.Pop()
		if fm, ok := v.(funcMarker); ok {
			for i, j := 0, len(argsReversed)-1; i < j; i, j = i+1, j-1 {
				argsReversed[i], argsReversed[j] = argsReversed[j], argsReversed[i]
			}
			res, err := fm.fn(argsReversed)
			if err != nil {
				return err
			}
			values.Push(res)
			return nil
		}
		argsReversed = append(argsReversed, v)
	}

	return fmt.Errorf("function call missing func marker")
}

// --- tokenize helpers ---

// scanTwoCharOp checks whether expr[i:i+2] is a recognised two-character operator.
func scanTwoCharOp(expr string, i int) (tok string, advance int, ok bool) {
	if i+1 >= len(expr) {
		return "", 0, false
	}
	switch expr[i : i+2] {
	case "==", "!=", ">=", "<=", "&&", "||":
		return expr[i : i+2], 2, true
	}
	return "", 0, false
}

// scanStringLiteral reads a single- or double-quoted string literal starting at i.
func scanStringLiteral(expr string, i int) (tok string, newI int, err error) {
	quote := expr[i]
	start := i
	i++
	for i < len(expr) {
		if expr[i] == '\\' {
			i += 2
			continue
		}
		if expr[i] == quote {
			i++
			return expr[start:i], i, nil
		}
		i++
	}
	return "", i, fmt.Errorf("unterminated string literal")
}

// scanWord reads an identifier/keyword/number token starting at i.
func scanWord(expr string, i int) (word string, newI int) {
	start := i
	for i < len(expr) && isIdentChar(expr[i]) {
		i++
	}
	return expr[start:i], i
}

// tryMergeIsNot checks whether the text at position i starts with "not" (as a word boundary),
// returning the position after "not" and true if so.
func tryMergeIsNot(expr string, i int) (newI int, merged bool) {
	j := i
	for j < len(expr) && isSpace(expr[j]) {
		j++
	}
	if j+3 > len(expr) || expr[j:j+3] != "not" {
		return i, false
	}
	k := j + 3
	if k < len(expr) && isIdentChar(expr[k]) {
		return i, false
	}
	return k, true
}

func tokenize(expression string) ([]string, error) {
	tokens := make([]string, 0, 32)

	for i := 0; i < len(expression); {
		if isSpace(expression[i]) {
			i++
			continue
		}

		if tok, adv, ok := scanTwoCharOp(expression, i); ok {
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		ch := expression[i]
		switch ch {
		case '(', ')', '>', '<', '!', '?', ':':
			tokens = append(tokens, string(ch))
			i++
			continue
		case ',':
			i++
			continue
		case '\'', '"':
			tok, newI, err := scanStringLiteral(expression, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = newI
			continue
		}

		word, newI := scanWord(expression, i)
		if newI == i {
			return nil, fmt.Errorf("unexpected character: %q", expression[i])
		}
		i = newI

		if word == "is" {
			if mergedI, ok := tryMergeIsNot(expression, i); ok {
				tokens = append(tokens, "is not")
				i = mergedI
				continue
			}
		}

		tokens = append(tokens, word)
	}

	return tokens, nil
}

// --- getFieldValue helpers ---

// deref unwraps pointer and interface values until a concrete value or nil is reached.
func deref(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// parsePart splits a path segment like "foo[0]" into its name and index components.
func parsePart(part string) (name string, hasIdx bool, idxSpec string, err error) {
	lb := strings.IndexByte(part, '[')
	if lb < 0 {
		return part, false, "", nil
	}
	rb := strings.LastIndexByte(part, ']')
	if rb < 0 || rb < lb {
		return "", false, "", fmt.Errorf("invalid index syntax in %q", part)
	}
	idxSpec = part[lb+1 : rb]
	if idxSpec == "" {
		return "", false, "", fmt.Errorf("empty index in %q", part)
	}
	return part[:lb], true, idxSpec, nil
}

// varToInt converts a variable value to an int for use as an array index.
func varToInt(spec string, v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int8:
		return int(n), nil
	case int16:
		return int(n), nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case uint:
		return int(n), nil
	case uint8:
		return int(n), nil
	case uint16:
		return int(n), nil
	case uint32:
		return int(n), nil
	case uint64:
		return int(n), nil
	case float32:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("index variable %q is not an int: %v", spec, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("index variable %q has unsupported type %T", spec, v)
	}
}

// toIndex converts an index specifier (literal integer or $variable) to an int.
func toIndex(spec string, vars map[string]any) (int, error) {
	if len(spec) > 0 && spec[0] == '$' {
		if vars == nil {
			return 0, fmt.Errorf("index variable %q not provided", spec)
		}
		v, ok := vars[spec]
		if !ok {
			return 0, fmt.Errorf("index variable %q not found", spec)
		}
		return varToInt(spec, v)
	}
	i, err := strconv.Atoi(spec)
	if err != nil {
		return 0, fmt.Errorf("invalid index %q: %v", spec, err)
	}
	return i, nil
}

// resolveNamedField looks up a named field on a struct or a string-keyed map.
func resolveNamedField(cv reflect.Value, name string) (any, bool) {
	switch cv.Kind() {
	case reflect.Map:
		if cv.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		mv := cv.MapIndex(reflect.ValueOf(name))
		if !mv.IsValid() {
			return nil, false
		}
		return mv.Interface(), true
	case reflect.Struct:
		fv := cv.FieldByName(name)
		if !fv.IsValid() && len(name) > 0 {
			// Try Title-cased name (lowerCamel in expression → UpperCamel in struct).
			fv = cv.FieldByName(strings.ToUpper(name[:1]) + name[1:])
		}
		if !fv.IsValid() || !fv.CanInterface() {
			return nil, false
		}
		return fv.Interface(), true
	default:
		return nil, false
	}
}

// applySliceIndex returns the element at idx in a slice or array, or (nil, false) if out of bounds.
func applySliceIndex(cur any, idx int) (any, bool) {
	iv := deref(reflect.ValueOf(cur))
	if !iv.IsValid() {
		return nil, false
	}
	switch iv.Kind() {
	case reflect.Slice, reflect.Array:
		if idx < 0 || idx >= iv.Len() {
			return nil, false
		}
		el := iv.Index(idx)
		if !el.IsValid() || !el.CanInterface() {
			return nil, false
		}
		return el.Interface(), true
	default:
		return nil, false
	}
}

// walkPathStep resolves a single dot-separated path segment against cur.
// Returns (nil, nil) for any missing field, key, or out-of-bounds index — never an error.
func walkPathStep(cur any, rawPart string, vars map[string]any) (any, error) {
	// $var part: substitute the variable as the new current object.
	if len(rawPart) > 0 && rawPart[0] == '$' {
		if vars == nil {
			return nil, nil
		}
		v, ok := vars[rawPart]
		if !ok {
			return nil, nil
		}
		return v, nil
	}

	name, hasIdx, idxSpec, err := parsePart(rawPart)
	if err != nil {
		return nil, err
	}

	// Resolve named field or map key.
	if name != "" {
		cv := deref(reflect.ValueOf(cur))
		if !cv.IsValid() {
			return nil, nil
		}
		val, ok := resolveNamedField(cv, name)
		if !ok {
			return nil, nil
		}
		cur = val
	}

	// Apply array/slice index if present.
	if hasIdx {
		idx, err := toIndex(idxSpec, vars)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
		val, ok := applySliceIndex(cur, idx)
		if !ok {
			return nil, nil
		}
		cur = val
	}

	return cur, nil
}

func (e *RuleEvaluator) getFieldValue(field string, vars map[string]any) (any, error) {
	if field == "" {
		return nil, fmt.Errorf("field cannot be empty")
	}

	var cur any = e.data

	for _, rawPart := range strings.Split(field, ".") {
		if rawPart == "" {
			return nil, nil
		}
		var err error
		cur, err = walkPathStep(cur, rawPart, vars)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, nil
		}
	}

	return cur, nil
}

// --- resolveValue helpers ---

// parseStringLiteral strips surrounding single or double quotes from a string token.
func parseStringLiteral(token string) (string, bool) {
	if len(token) < 2 {
		return "", false
	}
	first, last := token[0], token[len(token)-1]
	if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
		return token[1 : len(token)-1], true
	}
	return "", false
}

// derefSimplePointer unwraps a pointer to a primitive value for comparison purposes.
func derefSimplePointer(val any) any {
	if val == nil {
		return val
	}
	v := reflect.ValueOf(val)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return val
	}
	elem := v.Elem()
	switch elem.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return elem.Interface()
	}
	return val
}

func (e *RuleEvaluator) resolveValue(token string, ops *stack[tokenType], vars map[string]any) (any, error) {
	switch token {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null", "nil":
		return nil, nil
	}

	if v, err := strconv.ParseInt(token, 10, 64); err == nil {
		return v, nil
	}
	if v, err := strconv.ParseFloat(token, 64); err == nil {
		return v, nil
	}

	if s, ok := parseStringLiteral(token); ok {
		return s, nil
	}

	if e.funcs[token] != nil {
		ops.Push(tokFunc)
		return funcMarker{e.funcs[token]}, nil
	}

	val, err := e.getFieldValue(token, vars)
	if err != nil {
		return nil, err
	}

	return derefSimplePointer(val), nil
}

// --- EvaluateWithVars ---

func (e *RuleEvaluator) processToken(t string, values *stack[any], ops *stack[tokenType], vars map[string]any) error {
	switch t {
	case "==":
		ops.Push(tokEq)
	case "!=":
		ops.Push(tokNe)
	case ">":
		ops.Push(tokGt)
	case ">=":
		ops.Push(tokGe)
	case "<":
		ops.Push(tokLt)
	case "<=":
		ops.Push(tokLe)
	case "&&":
		ops.Push(tokAnd)
	case "||":
		ops.Push(tokOr)
	case "!", "not":
		ops.Push(tokNot)
	case "is":
		ops.Push(tokEq)
	case "is not":
		ops.Push(tokNe)
	case "(":
		ops.Push(tokLParen)
	case ")":
		return processParen(values, ops)
	case ":":
		ops.Push(tokElse)
	case "?":
		if ops.Len() > 0 {
			op, _ := ops.Pop()
			return processOp(values, op)
		}
	default:
		v, err := e.resolveValue(t, ops, vars)
		if err != nil {
			return err
		}
		values.Push(v)
	}
	return nil
}

// EvaluateWithVars evaluates expression against the bound data with the provided variables.
func (e *RuleEvaluator) EvaluateWithVars(expression string, vars map[string]any) (any, error) {
	tokens, err := tokenize(expression)
	if err != nil {
		return nil, err
	}

	values := newStack[any]()
	ops := newStack[tokenType]()

	for _, t := range tokens {
		if err := e.processToken(t, values, ops, vars); err != nil {
			return nil, err
		}
	}

	for ops.Len() > 0 {
		op, _ := ops.Pop()
		if err := processOp(values, op); err != nil {
			return nil, err
		}
	}

	return values.Pop()
}
