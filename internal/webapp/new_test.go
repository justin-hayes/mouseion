package webapp

// New builds the Handler for tests and panics when construction fails.
func New(s Services) *Handler {
	h, err := NewWithError(s)
	if err != nil {
		panic(err)
	}
	return h
}
