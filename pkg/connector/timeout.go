package connector

import "time"

// GetTimeoutDuration converts a timeout in milliseconds to a time.Duration.
// If timeoutMs is 0 or negative, returns the provided default.
func GetTimeoutDuration(timeoutMs int, defaultTimeout time.Duration) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return defaultTimeout
}
