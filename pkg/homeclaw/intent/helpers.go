package intent

// entityString extracts a string entity value from an entities map.
// Returns "" if the key is absent or the value is not a string.
func entityString(entities map[string]interface{}, key string) string {
	if entities == nil {
		return ""
	}
	v, ok := entities[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// errResponse builds an IntentResponse for an operation that produced an error
// but still counts as "handled" (we reply with an error message rather than
// falling through to the large model).
func errResponse(msg string, err error) IntentResponse {
	return IntentResponse{
		Handled:  true,
		Response: msg,
		Error:    err,
	}
}
