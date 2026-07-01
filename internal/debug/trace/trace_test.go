package trace

import "testing"

// TestLogCompiles verifies Log() compiles and does not panic
// regardless of build tag. In debug mode it emits to slog.Debug;
// in release mode it is a no-op.
func TestLogCompiles(t *testing.T) {
	// Must compile and not panic.
	Log("test_compilation", "tag", "debug")
}

func TestLogVaritypes(t *testing.T) {
	// Verify variadic attrs accept mixed types without panic.
	Log("varitypes",
		"int", 42,
		"string", "hello",
		"bool", true,
		"float", 3.14,
	)
}

func TestLogEmptyAttrs(t *testing.T) {
	// Zero attrs — common for entry/exit markers.
	Log("no_attrs")
}
