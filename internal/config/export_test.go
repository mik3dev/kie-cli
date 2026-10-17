package config

// SetGOOS overrides the detected OS and returns a func restoring it.
func SetGOOS(v string) (restore func()) {
	old := goos
	goos = v
	return func() { goos = old }
}
