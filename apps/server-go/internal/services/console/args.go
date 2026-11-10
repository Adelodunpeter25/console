package console

import (
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

func marshalResult(value any) (*jskernel.Result, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: data}, nil
}

func stringArg(args map[string]any, name string) (string, error) {
	value, ok := args[name].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

// optionalString reads a string argument. A value of any other type is an
// error rather than being treated as absent.
func optionalString(args map[string]any, name string) (string, error) {
	value, present := args[name]
	if !present {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %v", name, value)
	}
	return text, nil
}

// intValue reads a whole number from a JavaScript argument. The engine hands
// whole numbers over as int64 and fractional ones as float64, so every Go
// integer type is accepted; a number with a fractional part is not an integer.
func intValue(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int64(n)) {
			return int(n), true
		}
	}
	return 0, false
}

// optionalInt returns the named integer argument. ok is false when the
// argument is absent; a present value that is not a whole number is an error,
// so a wrong type fails loudly instead of being ignored.
func optionalInt(args map[string]any, name string) (n int, ok bool, err error) {
	value, present := args[name]
	if !present {
		return 0, false, nil
	}
	n, isInt := intValue(value)
	if !isInt {
		return 0, false, fmt.Errorf("%s must be a whole number, got %v", name, value)
	}
	return n, true, nil
}
