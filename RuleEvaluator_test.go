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
