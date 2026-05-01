package config

import "time"

const (
	DefaultNoVNCPath   = "/home/talon/novnc-static"
	DefaultFrontendURL = "https://getaced.io"
	DefaultOpenAIModel = "gpt-5.4-mini"

	MaxBookmarkFolderNameLen = 100
	MaxCustomPromptLen       = 500
	UsageLimit               = 10000

	AuthCodeExpiry   = 30 * time.Second
	OAuthStateExpiry = 10 * time.Minute
	VNCSessionExpiry = 5 * time.Minute
	JWTExpiry        = 7 * 24 * time.Hour

	HTTPTimeoutOpenAI = 60 * time.Second
	HTTPTimeoutGoogle = 10 * time.Second
)
