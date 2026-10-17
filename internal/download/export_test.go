package download

import (
	"net/http"
	"time"
)

// SetLink swaps the link function and returns a restore func.
func SetLink(f func(string, string) error) func() {
	old := linkFile
	linkFile = f
	return func() { linkFile = old }
}

// SetHeaderTimeout swaps the response-header timeout and returns a restore func.
func SetHeaderTimeout(d time.Duration) func() {
	old := headerTimeout
	headerTimeout = d
	return func() { headerTimeout = old }
}

// NewDownloadClient exposes downloadClient.
func NewDownloadClient(hc *http.Client) *http.Client { return downloadClient(hc) }
