package utils

import (
	"path/filepath"
	"strings"
)

func IsImagePath(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".png"
}

func IsHtmlPath(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".html"
}
