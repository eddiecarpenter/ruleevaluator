# ruleevaluator

[![CI](https://github.com/eddiecarpenter/ruleevaluator/actions/workflows/sonarcloud.yml/badge.svg)](https://github.com/eddiecarpenter/ruleevaluator/actions/workflows/sonarcloud.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=eddiecarpenter_ruleevaluator&metric=alert_status&token=ec6fcfc05f08f5b08ba9c218aaeb7a6caadb7341)](https://sonarcloud.io/summary/new_code?id=eddiecarpenter_ruleevaluator)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=eddiecarpenter_ruleevaluator&metric=coverage)](https://sonarcloud.io/summary/new_code?id=eddiecarpenter_ruleevaluator)
[![Go Report Card](https://goreportcard.com/badge/github.com/eddiecarpenter/ruleevaluator)](https://goreportcard.com/report/github.com/eddiecarpenter/ruleevaluator)
[![Go Reference](https://pkg.go.dev/badge/github.com/eddiecarpenter/ruleevaluator.svg)](https://pkg.go.dev/github.com/eddiecarpenter/ruleevaluator)

A lightweight, zero-dependency Go library for evaluating dynamic expressions against arbitrary data structures. It supports nested field access on structs and maps, array indexing, runtime variables, custom functions, and a full set of comparison and logical operators — all parsed and evaluated at runtime with no code generation or reflection overhead beyond what Go's standard library provides.

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Real-world patterns](#real-world-patterns)
  - [Wrapping multiple data sources](#wrapping-multiple-data-sources)
  - [Expressions that return non-boolean values](#expressions-that-return-non-boolean-values)
  - [Reusing an evaluator across a loop with mutable data](#reusing-an-evaluator-across-a-loop-with-mutable-data)
  - [Functions that close over application context](#functions-that-close-over-application-context)
- [Expression syntax](#expression-syntax)
  - [Literals](#literals)
  - [Field access](#field-access)
  - [Comparison operators](#comparison-operators)
  - [Logical operators](#logical-operators)
  - [Ternary operator](#ternary-operator)
  - [Custom functions](#custom-functions)
  - [Variables](#variables)
- [Type system](#type-system)
- [Error handling](#error-handling)
- [API reference](#api-reference)

---

## Installation

```bash
go get github.com/eddiecarpenter/ruleevaluator
```

Requires Go 1.24 or later. No external dependencies.

---

## Quick start

```go
package main

import (
    "fmt"
    "log"

    "github.com/eddiecarpenter/ruleevaluator"
)

type Order struct {
    Status   string
    Amount   int64
    Priority int64
}

func main() {
    order := Order{Status: "PENDING", Amount: 150, Priority: 2}

    ev := ruleevaluator.NewRuleEvaluator(order)

    result, err := ev.Evaluate("status == 'PENDING' && amount > 100")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result) // true
}
```

---

## Real-world patterns

### Wrapping multiple data sources

A common need is evaluating expressions against several different objects simultaneously — for example, a request, a sub-object extracted from it, and a loop variable. The simplest approach is a thin wrapper struct that holds each source as a field, and passing a pointer to it so the evaluator always reads the current value.

```go
type EvaluatorData struct {
    Req  any
    Info any
    Unit any
}

data := EvaluatorData{
    Req:  request,
    Info: &chargingInfo,
    Unit: nil, // populated later inside the loop
}

ev := ruleevaluator.NewRuleEvaluator(&data)
```

Expressions then reach each source via the wrapper field name:

```
req.subscriberId == '12345'
info.roleOfNode == 'MO'
unit.ratingGroup == 100
```

---

### Expressions that return non-boolean values

`Evaluate` returns `any`, not just `bool`. Expressions can resolve to a string, number, or `nil` — useful for computing a value (e.g. a category or direction) rather than a filter condition.

```go
// Expression evaluates to a string
result, err := ev.Evaluate(serviceType.SourceType)
if err != nil {
    return fmt.Errorf("evaluating source type: %w", err)
}

sourceType, ok := result.(string)
if !ok {
    sourceType = defaultSourceType // fallback if expression returned nil
}
```

```go
// Expression evaluates to a domain type via a registered function
result, err := ev.Evaluate(serviceType.ServiceDirection)
if err != nil {
    return fmt.Errorf("evaluating service direction: %w", err)
}

direction, ok := result.(CallDirection)
if !ok {
    direction = MO // safe default
}
```

The pattern is: evaluate, type-assert the result, fall back to a default if the assertion fails or the result is `nil`.

---

### Reusing an evaluator across a loop with mutable data

Create the evaluator once before the loop. If one field of the data wrapper changes per iteration (e.g. the current unit of usage), update it in place — the evaluator reads through the pointer and always sees the current value.

```go
data := EvaluatorData{Req: request, Info: &info}
ev := ruleevaluator.NewRuleEvaluator(&data) // pointer — mutations are visible

ev.RegisterFunction("serviceCategory", categoryFunc)

for _, unit := range request.MultipleUnitUsage {
    data.Unit = &unit // update before each evaluation

    // All three Evaluate calls see the updated data.Unit
    if serviceType.ServiceTypeRule != "" {
        match, err := ev.Evaluate(serviceType.ServiceTypeRule)
        if err != nil {
            return fmt.Errorf("evaluating service type rule: %w", err)
        }
        if match == false {
            continue
        }
    }

    category, err := ev.Evaluate(serviceType.ServiceCategory)
    if err != nil {
        return fmt.Errorf("evaluating service category: %w", err)
    }

    // use category ...
}
```

Key points:
- Pass a **pointer** to the wrapper (`&data`) so mutations are reflected without re-creating the evaluator.
- Register all functions **once** before the loop — registration is not cheap to repeat.
- Guard optional rule expressions with an empty-string check before calling `Evaluate`.

---

### Functions that close over application context

Custom functions can close over anything available at registration time — database clients, caches, configuration, other services. This keeps expressions simple and human-readable while the function handles all the lookup logic.

A common use case is computing a discount for an order based on the customer's membership tier, looked up from a repository:

```go
type Order struct {
    CustomerID string
    Category   string
    Amount     float64
}

// discountFor(category) returns the percentage discount for the customer's tier
func discountFor(customerID string, repo MembershipRepo) ruleevaluator.EvaluatorFunc {
    return func(args []any) (any, error) {
        if len(args) != 1 {
            return nil, fmt.Errorf("discountFor expects 1 arg (category)")
        }
        category, ok := args[0].(string)
        if !ok {
            return nil, fmt.Errorf("discountFor: category must be a string")
        }
        tier, err := repo.GetMembershipTier(customerID)
        if err != nil {
            return float64(0), nil // no discount if lookup fails
        }
        return repo.GetDiscount(tier, category), nil // returns float64
    }
}

order := Order{CustomerID: "u-42", Category: "electronics", Amount: 299.99}

ev := ruleevaluator.NewRuleEvaluator(order)
ev.RegisterFunction("discountFor", discountFor(order.CustomerID, membershipRepo))

// Eligibility rule stored in config — no code change needed to adjust business logic
eligible, err := ev.Evaluate("amount > 50 && discountFor(category) > 0.1")
```

The expression stored in your database or config file stays readable by non-engineers:

```
amount > 50 && discountFor(category) > 0.1
```

The same pattern applies to any runtime lookup: feature flag checks, geo-based shipping zone resolution, role/permission checks, product catalogue lookups, tax rate tables, and so on.

---

## Expression syntax

### Literals

| Type    | Syntax              | Go type   |
|---------|---------------------|-----------|
| Boolean | `true`, `false`     | `bool`    |
| Null    | `null`, `nil`       | `nil`     |
| Integer | `123`, `-42`        | `int64`   |
| Float   | `1.5`, `3.14`       | `float64` |
| String  | `'hello'`, `"world"`| `string`  |

Strings may use either single or double quotes. Backslash escaping is supported inside string literals.

```
true
null
42
3.14
'hello world'
"hello world"
```

---

### Field access

Fields are accessed using dot notation. The evaluator walks the path segment by segment against the data passed to `NewRuleEvaluator`.

**Structs** — exported fields are matched by name. lowerCamelCase field names in expressions are automatically tried as UpperCamelCase against the struct (e.g. `roleOfNode` matches exported field `RoleOfNode`).

**Maps** — only `map[string]any` (or any map with a `string` key) is supported. The segment name is used as the key directly.

**Missing fields or keys** return `nil` rather than an error. This mirrors the behaviour of dynamic languages and avoids defensive boilerplate in expressions.

```
# Struct field
chargingInformation.roleOfNode

# Nested struct
chargingInformation.recipientInfo[0].address

# Map key
order.metadata.source

# Root-level array index
items[0]
```

**Array indexing** uses `[n]` syntax where `n` is either a literal integer or a variable (see [Variables](#variables) below).

```
recipientInfo[0].address
recipientInfo[1].address
```

Out-of-bounds indices return `nil` — no panic, no error.

---

### Comparison operators

| Operator  | Meaning                     |
|-----------|-----------------------------|
| `==`      | Equal                       |
| `!=`      | Not equal                   |
| `>`       | Greater than                |
| `>=`      | Greater than or equal       |
| `<`       | Less than                   |
| `<=`      | Less than or equal          |
| `is`      | Alias for `==`              |
| `is not`  | Alias for `!=`              |

```
status == 'ACTIVE'
amount >= 100
priority != 0
roleOfNode is 'MO'
roleOfNode is not 'MT'
```

> **Important — type strictness.** Comparisons require both sides to be the same Go type. Integer literals parse as `int64` and float literals as `float64`. A struct field typed as `int` compared to a literal `100` (which is `int64`) will return an error. See [Type system](#type-system) for details.

**Nil comparisons** support `==` and `!=` only. Using `>`, `<`, `>=`, or `<=` with a `nil` operand returns an error.

```
missingField == null     // true if the field is missing
missingField != null     // false if the field is missing
```

---

### Logical operators

| Operator      | Meaning          |
|---------------|------------------|
| `&&`          | Logical AND      |
| `\|\|`        | Logical OR       |
| `!`           | Logical NOT      |
| `not`         | Alias for `!`    |

Both operands of `&&` and `||` must evaluate to `bool`. The operand of `!` / `not` must also be `bool`. Passing a non-bool value returns an error.

```
status == 'ACTIVE' && amount > 0
isPremium || level >= 5
!isBlocked
not isExpired
```

**Parentheses** control evaluation order:

```
(status == 'PENDING' || status == 'ACTIVE') && amount > 0
```

---

### Ternary operator

```
condition ? trueValue : falseValue
```

The condition must evaluate to `bool`. The true and false branches can be any expression, including field access, literals, or function calls.

```
status == 'ACTIVE' ? 'allowed' : 'denied'
amount > 1000 ? 'high' : 'standard'
```

---

### Custom functions

Register named functions before evaluating. A function receives its arguments as `[]any` and returns `(any, error)`.

```go
ev := ruleevaluator.NewRuleEvaluator(data)

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

result, err := ev.Evaluate("startsWith(address, 'tel:')")
```

Functions are called using standard call syntax with comma-separated arguments:

```
startsWith(address, 'tel:')
concat(firstName, lastName)
toUpperCase(status)
```

Arguments can be any expression — literals, field paths, variables, or nested function calls.

---

### Variables

Variables are runtime values injected alongside an expression. They are prefixed with `$` and passed as a `map[string]any` to `EvaluateWithVars`.

```go
vars := map[string]any{
    "$index": int64(1),
    "$prefix": "tel:",
}

result, err := ev.EvaluateWithVars(
    "startsWith(recipientInfo[$index].address, $prefix)",
    vars,
)
```

Variables can be used:

- **As a value in an expression:** `$prefix == 'tel:'`
- **As an array index:** `items[$index]`
- **As the root of a path:** `$obj.fieldName`

A missing variable returns `nil` rather than an error.

---

## Type system

The evaluator enforces strict type equality on comparisons. Both operands must be the same Go type — no implicit coercion is performed.

**Numeric literals:**

| Literal form | Parsed as  |
|--------------|------------|
| `123`        | `int64`    |
| `1.5`        | `float64`  |

**Common pitfall — `int` fields vs integer literals:**

If a struct field is declared as `int` (not `int64`), comparing it to a numeric literal will fail because the literal is `int64`:

```go
type Item struct {
    Count int // not int64
}

ev := ruleevaluator.NewRuleEvaluator(Item{Count: 5})
_, err := ev.Evaluate("count == 5") // ERROR: cannot compare int and int64
```

**Fix:** Declare numeric fields as `int64` (or `float64`) to match how the evaluator parses numeric literals.

```go
type Item struct {
    Count int64 // matches int64 literals
}
```

**String comparisons** use lexicographic ordering for `>`, `<`, `>=`, `<=`.

**Boolean comparisons** support all six operators. `>` is interpreted as "true is greater than false".

---

## Error handling

All errors returned by `Evaluate` and `EvaluateWithVars` are either plain `error` values (for syntax errors) or typed `*EvaluatorError` values. Use `errors.As` to inspect them:

```go
import "errors"

result, err := ev.Evaluate(expr)
if err != nil {
    var evalErr *ruleevaluator.EvaluatorError
    if errors.As(err, &evalErr) {
        fmt.Println("code:", evalErr.Code)
        fmt.Println("message:", evalErr.Message)
    } else {
        fmt.Println("syntax error:", err)
    }
}
```

**Error codes:**

| Code          | Meaning                                      |
|---------------|----------------------------------------------|
| `EMPTY_STACK` | Internal stack underflow — malformed expression |

Errors returned directly (not wrapped in `EvaluatorError`) include syntax errors such as unterminated string literals, mismatched parentheses, and unexpected characters.

---

## API reference

### `NewRuleEvaluator(data any) *RuleEvaluator`

Creates a new evaluator bound to `data`. The data can be any Go value — struct, map, slice, pointer, or primitive. Field paths in expressions are resolved against this value.

```go
ev := ruleevaluator.NewRuleEvaluator(myStruct)
```

---

### `(*RuleEvaluator) RegisterFunction(name string, fn EvaluatorFunc)`

Registers a named function that can be called from expressions. Must be called before `Evaluate` or `EvaluateWithVars`.

```go
type EvaluatorFunc func(args []any) (any, error)

ev.RegisterFunction("double", func(args []any) (any, error) {
    n := args[0].(int64)
    return n * 2, nil
})
```

---

### `(*RuleEvaluator) Evaluate(expression string) (any, error)`

Evaluates an expression against the bound data with no variables. Returns the result as `any` — typically `bool`, `string`, `int64`, `float64`, or `nil`.

```go
result, err := ev.Evaluate("status == 'ACTIVE' && amount > 0")
if err != nil {
    // handle
}
approved := result.(bool)
```

---

### `(*RuleEvaluator) EvaluateWithVars(expression string, vars map[string]any) (any, error)`

Same as `Evaluate` but accepts a variable map. Variables are referenced in expressions with a `$` prefix.

```go
result, err := ev.EvaluateWithVars(
    "items[$i].status == 'READY'",
    map[string]any{"$i": int64(0)},
)
```

Pass `nil` for `vars` if no variables are needed (equivalent to calling `Evaluate`).

---

## License

See [LICENSE](LICENSE).
