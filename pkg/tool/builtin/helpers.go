package builtin

// stringFromInput extracts a string value from the input map.
// Returns ("", false) if the key is missing or not a string.
func stringFromInput(input map[string]interface{}, key string) (string, bool) {
	v, ok := input[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// intFromInput extracts an integer value from the input map, handling JSON float64 conversion.
// JSON numbers are decoded as float64, so this handles the type conversion.
// Returns defaultVal if the key is missing or not a number.
func intFromInput(input map[string]interface{}, key string, defaultVal int) int {
	v, ok := input[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return defaultVal
	}
}
