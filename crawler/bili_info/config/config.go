package config

import "time"

const (
	Workers        = 5
	MaxRetries     = 3
	RetryBaseDelay = 2 * time.Second
	CookieFile     = "cookie.txt"
	ImageBaseDir   = "images"
)

type Config struct {
	Output   string
	BVIDs    string
	Input    string
	ImageDir string
	NoCover  bool
	Help     bool
}
