package config

import "time"

const (
	DefaultNoVNCPath   = "/home/talon/novnc-static"
	DefaultFrontendURL = "https://getaced.io"
	DefaultOpenAIModel = "gpt-4o"

	MaxBookmarkFolderNameLen = 100
	MaxCustomPromptLen       = 500
	UsageLimit               = 30

	AuthCodeExpiry  = 30 * time.Second
	OAuthStateExpiry = 10 * time.Minute
	VNCSessionExpiry = 5 * time.Minute
	JWTExpiry        = 7 * 24 * time.Hour

	HTTPTimeoutOpenAI = 30 * time.Second
	HTTPTimeoutCreem  = 15 * time.Second
	HTTPTimeoutGoogle = 10 * time.Second

	// Creem product IDs
	MonthlyProductID  = "prod_5EZyT4sixyBhWmVYxf1nwl"
	QuarterlyProductID = "prod_4iDmKqfGpdNhoLsDEHbw1K"

	// Plan pricing in cents
	MonthlyPriceCents   = 499 // $4.99
	QuarterlyPriceCents = 999 // $9.99
	QuarterlyMRRCents   = 333 // $9.99 / 3 months
)
