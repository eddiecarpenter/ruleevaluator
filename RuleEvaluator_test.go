package ruleevaluator

import (
	"errors"
	"strings"
	"testing"
)

type Recipient struct {
	Address string
}

type ChargingInformation struct {
	RoleOfNode    string
	RecipientInfo []Recipient
	RatingGroup   int // IMPORTANT: int (not int64) to show the type mismatch behaviour
}

type Root struct {
	ChargingInformation ChargingInformation
	M                   map[string]any
	Arr                 []any
}

func newEvaluatorFixture() *RuleEvaluator {
	root := Root{
		ChargingInformation: ChargingInformation{
			RoleOfNode: "MO",
			RecipientInfo: []Recipient{
				{Address: "tel:+123"},
				{Address: "tel:+999"},
			},
			RatingGroup: 100, // int
		},
		M: map[string]any{
			"foo": "bar",
			"n":   int64(7),
			"i":   int(7),
			"f":   float64(1.5),
			"b":   true,
			"arr": []any{"x", "y"},
			"obj": map[string]any{"k": "v"},
		},
		Arr: []any{"a0", "a1"},
	}

	ev := NewRuleEvaluator(root)

	// startsWith(s, prefix) -> bool
	ev.RegisterFunction("startsWith", func(args []any) (any, error) {
		if len(args) != 2 {
			return nil, errors.New("startsWith expects 2 args")
		}
		s, ok1 := args[0].(string)
		p, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return nil, errors.New("startsWith args must be strings")
		}
		return strings.HasPrefix(s, p), nil
	})

	// concat(a, b) -> string
	ev.RegisterFunction("concat", func(args []any) (any, error) {
		if len(args) != 2 {
			return nil, errors.New("concat expects 2 args")
		}
		a, ok1 := args[0].(string)
		b, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return nil, errors.New("concat args must be strings")
		}
		return a + b, nil
	})

	// identity(x) -> x
	ev.RegisterFunction("identity", func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("identity expects 1 arg")
		}
		return args[0], nil
	})

	return ev
}

func TestRuleEvaluator_Literals(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name string
		expr string
		want any
	}{
		{"true", "true", true},
		{"false", "false", false},
		{"null", "null", nil},
		{"int64", "123", int64(123)},
		{"float64", "1.25", float64(1.25)},
		{"single-quoted", "'abc'", "abc"},
		{"double-quoted", "\"abc\"", "abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_FieldResolution_StructMapIndexing(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name string
		expr string
		want any
	}{
		// Struct fields (lowerCamel in expression, UpperCamel in struct)
		{"struct field (title-case fallback)", "chargingInformation.roleOfNode", "MO"},
		{"struct slice index + nested field", "chargingInformation.recipientInfo[0].address", "tel:+123"},
		{"struct slice index 2", "chargingInformation.recipientInfo[1].address", "tel:+999"},

		// Root slice
		{"root array index", "arr[1]", "a1"},

		// Map
		{"map string", "m.foo", "bar"},
		{"map int64", "m.n", int64(7)},
		{"map int", "m.i", int(7)},
		{"map float64", "m.f", float64(1.5)},
		{"map bool", "m.b", true},
		{"map slice index", "m.arr[1]", "y"},
		{"nested map", "m.obj.k", "v"},

		// Missing returns nil (Java-like)
		{"missing map key -> nil", "m.missingKey", nil},
		{"missing index -> nil", "m.arr[99]", nil},
		{"missing nested field -> nil", "chargingInformation.missingField", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_Variables_PathAndIndex(t *testing.T) {
	ev := newEvaluatorFixture()

	vars := map[string]any{
		"$i": int64(1),
		"$x": map[string]any{"k": "v2"},
	}

	cases := []struct {
		name string
		expr string
		want any
	}{
		{"index via $i", "chargingInformation.recipientInfo[$i].address", "tel:+999"},
		{"$var replaces current object", "$x.k", "v2"},
		{"missing $var -> nil", "$missing.k", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.EvaluateWithVars(tc.expr, vars)
			if err != nil {
				t.Fatalf("EvaluateWithVars(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("EvaluateWithVars(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_Comparisons(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"int eq true", "1 == 1", true, false},
		{"int eq false", "1 == 2", false, false},
		{"int gt true", "2 > 1", true, false},
		{"string lt true", "'a' < 'b'", true, false},
		{"bool eq true", "true == true", true, false},

		// nil comparisons: only == and != allowed
		{"nil eq true", "null == null", true, false},
		{"nil ne false", "null != null", false, false},
		{"nil gt error", "null > 1", nil, true},

		// strict type mismatch (this is the behaviour you are worried about)
		{"type mismatch literal int vs float", "1 == 1.0", nil, true},
		{"type mismatch string vs int", "'1' == 1", nil, true},

		// strict type mismatch from struct field type vs numeric literal:
		// chargingInformation.ratingGroup is int, literal 100 parses as int64.
		{"type mismatch int field vs int64 literal", "chargingInformation.ratingGroup == 100", nil, true},

		// but this one should work: map contains int64(7) and literal 7 is int64
		{"int64 field vs int64 literal ok", "m.n == 7", true, false},

		// and this should fail: map contains int(7) but literal 7 is int64
		{"int field vs int64 literal fails", "m.i == 7", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%#v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_LogicalOps_AndOrNot_Parentheses(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"and", "true && false", false, false},
		{"or", "true || false", true, false},
		{"not bang", "!false", true, false},
		{"not keyword", "not false", true, false},
		{"grouped parentheses", "(true && false) || true", true, false},

		// Type enforcement: logical ops require bool
		{"and with non-bool error", "true && 1", nil, true},
		{"not with non-bool error", "!1", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%#v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_Ternary(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"true branch", "true ? 'A' : 'B'", "A", false},
		{"false branch", "false ? 'A' : 'B'", "B", false},
		{"condition must be bool", "1 ? 'A' : 'B'", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%#v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_FunctionCalls_WithArgs(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{
			name: "startsWith true",
			expr: "startsWith('hello', 'he')",
			want: true,
		},
		{
			name: "startsWith false",
			expr: "startsWith('hello', 'xx')",
			want: false,
		},
		{
			name: "concat literals",
			expr: "concat('a', 'b')",
			want: "ab",
		},
		{
			name: "concat with field",
			expr: "concat(chargingInformation.roleOfNode, 'X')",
			want: "MOX",
		},
		{
			name: "identity on number",
			expr: "identity(123)",
			want: int64(123),
		},
		{
			name:    "wrong arg count",
			expr:    "identity(1, 2)",
			wantErr: true,
		},
		{
			name:    "wrong arg type",
			expr:    "startsWith(1, 'a')",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%#v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_ErrorCases_Syntax(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"mismatched parens", "(1 == 1", true},
		{"unterminated string", "'abc", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ev.Evaluate(tc.expr)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.expr)
			}
		})
	}
}

func TestRuleEvaluator_IsIsNot(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name string
		expr string
		want any
	}{
		{"is null true", "null is null", true},
		{"is null false", "'x' is null", false},
		{"is not null true", "'x' is not null", true},
		{"is not null false", "null is not null", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_BoolComparisons(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"bool gt (true > false)", "true > false", true, false},
		{"bool lt (false < true)", "false < true", true, false},
		{"bool ge (true >= true)", "true >= true", true, false},
		{"bool le (false <= false)", "false <= false", true, false},
		{"bool gt false (false > true)", "false > true", false, false},
		{"bool ne (true != false)", "true != false", true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_Variables_IntTypes(t *testing.T) {
	type Container struct {
		Items []string
	}
	container := Container{Items: []string{"zero", "one", "two"}}
	ev := NewRuleEvaluator(container)

	cases := []struct {
		name  string
		index any
		want  any
	}{
		{"int index", int(1), "one"},
		{"int8 index", int8(2), "two"},
		{"int16 index", int16(0), "zero"},
		{"int32 index", int32(1), "one"},
		{"uint index", uint(2), "two"},
		{"uint8 index", uint8(0), "zero"},
		{"uint16 index", uint16(1), "one"},
		{"uint32 index", uint32(2), "two"},
		{"uint64 index", uint64(0), "zero"},
		{"float32 index", float32(1), "one"},
		{"float64 index", float64(2), "two"},
		{"string index", "0", "zero"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.EvaluateWithVars("items[$i]", map[string]any{"$i": tc.index})
			if err != nil {
				t.Fatalf("EvaluateWithVars err=%v", err)
			}
			if got != tc.want {
				t.Fatalf("got=%#v, want=%#v", got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_PointerFields(t *testing.T) {
	type WithPtr struct {
		Name *string
		Age  *int
		Flag *bool
	}

	name := "Alice"
	age := 42
	flag := true

	ev := NewRuleEvaluator(WithPtr{Name: &name, Age: &age, Flag: &flag})

	cases := []struct {
		name string
		expr string
		want any
	}{
		{"pointer string field", "name", "Alice"},
		{"pointer bool field eq", "flag == true", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestRuleEvaluator_ErrorPaths(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name string
		expr string
	}{
		// compare: incomparable type (int vs bool)
		{"compare incompatible types", "m.b == m.i"},
		// processUnaryNot: insufficient values
		{"not insufficient", "!"},
		// parsePart: missing closing bracket
		{"missing closing bracket", "arr[0"},
		// parsePart: empty index
		{"empty index", "arr[]"},
		// toIndex: invalid literal
		{"invalid literal index", "arr[x]"},
		// toIndex: $var not found
		{"var index not found", "arr[$missing]"},
		// getFieldValue: empty expression
		{"empty expression", ""},
		// applySliceIndex on non-slice — returns nil, not error (covered separately)
		// unterminated double-quoted string
		{"unterminated double-quote string", `"abc`},
		// processFunc: function missing marker (stack exhausted before finding marker)
		{"func marker missing", "startsWith('a')"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ev.Evaluate(tc.expr)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.expr)
			}
		})
	}
}

func TestRuleEvaluator_NilPointerDeref(t *testing.T) {
	type Inner struct {
		Value string
	}
	type WithNilPtr struct {
		Inner *Inner
	}
	// nil pointer to a struct — accessing a field beneath it should return nil
	ev := NewRuleEvaluator(WithNilPtr{Inner: nil})

	got, err := ev.Evaluate("inner.value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for field access through nil pointer, got %#v", got)
	}
}

func TestRuleEvaluator_ApplySliceIndex_NonSlice(t *testing.T) {
	ev := newEvaluatorFixture()
	// m.foo is a string — indexing it returns nil, not an error
	got, err := ev.Evaluate("m.foo[0]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for index on non-slice, got %#v", got)
	}
}

func TestRuleEvaluator_VarIndexErrors(t *testing.T) {
	type Container struct {
		Items []string
	}
	ev := NewRuleEvaluator(Container{Items: []string{"a", "b"}})

	// $var with nil vars map
	_, err := ev.EvaluateWithVars("items[$i]", nil)
	if err == nil {
		t.Fatal("expected error for nil vars with $var index")
	}

	// varToInt with unsupported type
	_, err = ev.EvaluateWithVars("items[$i]", map[string]any{"$i": struct{}{}})
	if err == nil {
		t.Fatal("expected error for unsupported index variable type")
	}

	// varToInt with non-numeric string
	_, err = ev.EvaluateWithVars("items[$i]", map[string]any{"$i": "abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric string index variable")
	}
}

func TestRuleEvaluator_OrderedOp_FloatAndString(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name string
		expr string
		want any
	}{
		{"float gt", "2.0 > 1.0", true},
		{"float lt", "0.5 < 1.0", true},
		{"float ge", "1.0 >= 1.0", true},
		{"float le", "0.5 <= 1.0", true},
		{"float ne", "1.0 != 2.0", true},
		{"string gt", "'b' > 'a'", true},
		{"string ge", "'a' >= 'a'", true},
		{"string ne", "'a' != 'b'", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

// TestRuleEvaluator_CompareNil covers the nil > / < / >= / <= error path.
func TestRuleEvaluator_CompareNil_OrderedError(t *testing.T) {
	ev := newEvaluatorFixture()
	for _, expr := range []string{"null > 1", "null < 1", "null >= 1", "null <= 1"} {
		_, err := ev.Evaluate(expr)
		if err == nil {
			t.Fatalf("expected error for %q", expr)
		}
	}
}

// TestRuleEvaluator_Compare_NilRight covers compare when right operand is nil.
func TestRuleEvaluator_Compare_NilRight(t *testing.T) {
	ev := newEvaluatorFixture()
	got, err := ev.Evaluate("null == null")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Fatalf("expected true, got %#v", got)
	}
	got, err = ev.Evaluate("null != null")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != false {
		t.Fatalf("expected false, got %#v", got)
	}
}

// TestRuleEvaluator_Compare_UncomparableType covers the default case in compare.
func TestRuleEvaluator_Compare_UncomparableType(t *testing.T) {
	type Custom struct{ V int }
	ev := NewRuleEvaluator(struct{ A, B Custom }{A: Custom{1}, B: Custom{1}})
	_, err := ev.Evaluate("a == b")
	if err == nil {
		t.Fatal("expected error comparing uncomparable type")
	}
}

// TestRuleEvaluator_ProcessTernary_NonBoolCondition covers the non-bool condition error.
func TestRuleEvaluator_ProcessTernary_NonBoolCondition(t *testing.T) {
	ev := newEvaluatorFixture()
	_, err := ev.Evaluate("1 ? 'a' : 'b'")
	if err == nil {
		t.Fatal("expected error for non-bool ternary condition")
	}
}

// TestRuleEvaluator_ProcessLogicalOp_InsufficientValues covers logical op error path.
func TestRuleEvaluator_ProcessLogicalOp_InsufficientValues(t *testing.T) {
	ev := newEvaluatorFixture()
	_, err := ev.Evaluate("&& true")
	if err == nil {
		t.Fatal("expected error for insufficient values in logical op")
	}
}

// TestRuleEvaluator_ScanStringLiteral_Escape covers the backslash escape path.
func TestRuleEvaluator_ScanStringLiteral_Escape(t *testing.T) {
	ev := newEvaluatorFixture()
	got, err := ev.Evaluate(`'it\'s'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `it\'s` {
		t.Fatalf("got %#v", got)
	}
}

// TestRuleEvaluator_ResolveNamedField_NonStringMap covers map with non-string key.
func TestRuleEvaluator_ResolveNamedField_NonStringMap(t *testing.T) {
	ev := NewRuleEvaluator(struct{ M map[int]string }{M: map[int]string{1: "a"}})
	got, err := ev.Evaluate("m.foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for non-string-keyed map, got %#v", got)
	}
}

// TestRuleEvaluator_ResolveValue_UnknownToken covers the field-not-found path in resolveValue.
func TestRuleEvaluator_ResolveValue_Tokens(t *testing.T) {
	ev := newEvaluatorFixture()

	cases := []struct {
		name string
		expr string
		want any
	}{
		// resolveValue: is / is not push eq/ne operators
		{"is null via field", "m.missingKey is null", true},
		{"is not null via field", "m.foo is not null", true},
		// processToken: || operator
		{"or operator", "false || true", true},
		// processToken: >= operator
		{"ge operator", "2 >= 2", true},
		// processToken: <= operator
		{"le operator", "1 <= 2", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.Evaluate(tc.expr)
			if err != nil {
				t.Fatalf("Evaluate(%q) err=%v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q)=%#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

// TestRuleEvaluator_VarToInt_AllTypes exercises the remaining varToInt type branches via EvaluateWithVars.
func TestRuleEvaluator_VarToInt_AllTypes(t *testing.T) {
	type C struct{ Items []string }
	ev := NewRuleEvaluator(C{Items: []string{"zero", "one", "two"}})

	cases := []struct {
		name  string
		index any
		want  string
	}{
		{"int32", int32(1), "one"},
		{"uint32", uint32(2), "two"},
		{"float32", float32(0), "zero"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ev.EvaluateWithVars("items[$i]", map[string]any{"$i": tc.index})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got=%#v want=%#v", got, tc.want)
			}
		})
	}
}
