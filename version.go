package variably

// Version information
const (
	// Version is the current SDK version
	Version = "1.0.0"
	
	// UserAgent is the HTTP User-Agent header sent with requests
	UserAgent = "Variably-Go-SDK/" + Version
)

// GetVersion returns the current SDK version
func GetVersion() string {
	return Version
}

// GetUserAgent returns the HTTP User-Agent string
func GetUserAgent() string {
	return UserAgent
}