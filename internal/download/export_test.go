package download

// SetLink swaps the link function and returns a restore func.
func SetLink(f func(string, string) error) func() {
	old := linkFile
	linkFile = f
	return func() { linkFile = old }
}
