package util

import (
	"errors"
	"fmt"
	"testing"
)

// TestWithRetries_NoError tests the WithRetries function with a function that doesn't return an error.
func TestWithRetries_NoError(t *testing.T) {
	// Create a simple function that returns a string
	count := 0
	fn := func() string {
		count++
		return "success"
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function
	result := wrappedFn()

	// Check that the function was called only once
	if count != 1 {
		t.Errorf("Expected function to be called once, got %d calls", count)
	}

	// Check that the result is correct
	if result != "success" {
		t.Errorf("Expected result to be 'success', got '%s'", result)
	}
}

// TestWithRetries_WithError tests the WithRetries function with a function that returns an error.
func TestWithRetries_WithError(t *testing.T) {
	// Create a function that returns an error on the first two calls
	count := 0
	fn := func() (string, error) {
		count++
		if count < 3 {
			return "", errors.New("test error")
		}
		return "success", nil
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function
	result, err := wrappedFn()

	// Check that the function was called three times
	if count != 3 {
		t.Errorf("Expected function to be called 3 times, got %d calls", count)
	}

	// Check that there's no error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the result is correct
	if result != "success" {
		t.Errorf("Expected result to be 'success', got '%s'", result)
	}
}

// TestWithRetries_ExhaustedRetries tests the WithRetries function when retries are exhausted.
func TestWithRetries_ExhaustedRetries(t *testing.T) {
	// Create a function that always returns an error
	count := 0
	fn := func() (string, error) {
		count++
		return "", fmt.Errorf("test error %d", count)
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function
	result, err := wrappedFn()

	// Check that the function was called the maximum number of times
	if count != 4 { // Initial call + 3 retries = 4 calls
		t.Errorf("Expected function to be called 4 times, got %d calls", count)
	}

	// Check that there's an error
	if err == nil {
		t.Errorf("Expected an error, got nil")
	}

	// Check that the error message contains the last attempt
	expectedError := "test error 4"
	if err.Error() != expectedError {
		t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
	}

	// Check that the result is empty
	if result != "" {
		t.Errorf("Expected result to be empty, got '%s'", result)
	}
}

// TestWithRetries_WithPanic tests the WithRetries function with a function that panics.
func TestWithRetries_WithPanic(t *testing.T) {
	// Create a function that panics on the first two calls
	count := 0
	fn := func() (string, error) {
		count++
		if count < 3 {
			panic(fmt.Sprintf("test panic %d", count))
		}
		return "success", nil
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function
	result, err := wrappedFn()

	// Check that the function was called three times
	if count != 3 {
		t.Errorf("Expected function to be called 3 times, got %d calls", count)
	}

	// Check that there's no error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the result is correct
	if result != "success" {
		t.Errorf("Expected result to be 'success', got '%s'", result)
	}
}

// TestWithRetries_ExhaustedPanicRetries tests the WithRetries function when panic retries are exhausted.
func TestWithRetries_ExhaustedPanicRetries(t *testing.T) {
	// Create a function that always panics
	count := 0
	fn := func() (string, error) {
		count++
		panic(fmt.Sprintf("test panic %d", count))
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function
	_, err := wrappedFn()

	// Check that the function was called the maximum number of times
	if count != 4 { // Initial call + 3 retries = 4 calls
		t.Errorf("Expected function to be called 4 times, got %d calls", count)
	}

	// Check that there's an error that contains "panic recovered"
	if err == nil {
		t.Errorf("Expected an error, got nil")
	}
	if err != nil {
		if msg := err.Error(); len(msg) < 15 || msg[:15] != "panic recovered" {
			t.Errorf("Expected error to start with 'panic recovered', got '%s'", msg)
		}
	}
}

// TestWithRetries_NoPanicHandling tests that panics are re-panicked when the function doesn't return an error.
func TestWithRetries_NoPanicHandling(t *testing.T) {
	// Create a function that always panics but doesn't return an error
	count := 0
	fn := func() string {
		count++
		panic(fmt.Sprintf("test panic %d", count))
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function, expecting a panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected a panic, but none occurred")
		} else {
			// Check that the function was called the maximum number of times
			if count != 4 { // Initial call + 3 retries = 4 calls
				t.Errorf("Expected function to be called 4 times, got %d calls", count)
			}
		}
	}()
	
	wrappedFn()
}

// TestWithRetries_MultipleParams tests the WithRetries function with a function that takes multiple parameters.
func TestWithRetries_MultipleParams(t *testing.T) {
	// Create a function that takes multiple parameters and returns multiple values
	count := 0
	fn := func(a int, b string) (int, string, error) {
		count++
		if count < 2 {
			return 0, "", errors.New("test error")
		}
		return a + 1, b + "!", nil
	}

	// Wrap it with retries
	wrappedFn := WithRetries(fn, 3)

	// Call the wrapped function
	resultInt, resultString, err := wrappedFn(42, "hello")

	// Check that the function was called twice
	if count != 2 {
		t.Errorf("Expected function to be called 2 times, got %d calls", count)
	}

	// Check that there's no error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the results are correct
	if resultInt != 43 {
		t.Errorf("Expected int result to be 43, got %d", resultInt)
	}
	if resultString != "hello!" {
		t.Errorf("Expected string result to be 'hello!', got '%s'", resultString)
	}
}