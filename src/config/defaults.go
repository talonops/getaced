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
)
