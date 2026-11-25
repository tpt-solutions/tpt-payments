package main

import "fmt"

// typed asserts the dynamic type of a decoded JSON value. A mismatch panics, which fails the test.
func typed[T any](v any) T {
	t, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("unexpected JSON type %T", v))
	}
	return t
}
