package main

import (
	"fmt"
	"reflect"
)

// WithRetries takes a function f of any signature and an integer `retries`.
// It returns a new function of the same signature that will:
// 1) Retry calling f up to `retries` times if there is a panic or if f returns a non-nil error.
// 2) If f does not return an error, then only panic retries apply.
func WithRetries[T any](f T, retries int) T {
	fv := reflect.ValueOf(f)
	ft := fv.Type()

	if ft.Kind() != reflect.Func {
		panic("WithRetries expects a function")
	}

	// Check if the function returns an error as its last result.
	// We'll store the index of the error result (if any).
	var hasError bool
	var errorIndex int
	outCount := ft.NumOut()
	if outCount > 0 {
		lastOut := ft.Out(outCount - 1)
		// Compare with the type of 'error' (i.e. the type underlying `error` interface).
		hasError = (lastOut == reflect.TypeOf((*error)(nil)).Elem())
		if hasError {
			errorIndex = outCount - 1
		}
	}

	// MakeFunc allows us to create a new function of the same type as ft,
	// but we control how it is invoked internally.
	wrapper := reflect.MakeFunc(ft, func(in []reflect.Value) (out []reflect.Value) {
		for i := 0; i <= retries; i++ {
			callOut, panicVal := safeCall(fv, in)

			// If there was a panic, treat it like an error
			if panicVal != nil {
				if i < retries {
					// Retry
					continue
				}
				// No more retries left
				if hasError {
					// Fill the error return with the panic as an error
					panicErr := fmt.Errorf("panic recovered: %v", panicVal)
					callOut[errorIndex] = reflect.ValueOf(panicErr)
					return callOut
				}
				// If there's no error in the signature, re-panic
				panic(panicVal)
			}

			// No panic, check for error if the signature allows it
			if hasError {
				errVal := callOut[errorIndex]
				if !errVal.IsNil() {
					// We have a non-nil error
					if i < retries {
						// Retry
						continue
					}
				}
			}

			// Either there's no error, or we've exhausted retries
			return callOut
		}

		// Should never reach here because the loop returns on success or final attempt
		return nil
	})

	// Convert the newly built wrapper back to the original type T
	return wrapper.Interface().(T)
}

// safeCall invokes fn.Call(in) but wraps it in a recover to capture panics.
// Returns the results of the call plus any panic value as a separate interface{}.
func safeCall(fn reflect.Value, in []reflect.Value) (ret []reflect.Value, panicVal interface{}) {
	defer func() {
		if r := recover(); r != nil {
			panicVal = r
		}
	}()
	ret = fn.Call(in)
	return ret, nil
}
