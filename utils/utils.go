package utils

import (
	"fmt"
	"os"            // Added for UserHomeDir
	"path/filepath" // Added for Join
	"regexp"
	"strings"
)

// Moved from main
var home, _ = os.UserHomeDir()                 // Ignoring error here for simplicity, consider handling
var DownloadDir = filepath.Join(home, "books") // Exported

// SanitizeFilename removes characters invalid for filenames and truncates.
func SanitizeFilename(filename string) string {
	sanitized := regexp.MustCompile(`[\\/*?:"<>|]`).ReplaceAllString(filename, "")
	sanitized = regexp.MustCompile(`\.{2,}`).ReplaceAllString(sanitized, ".")
	sanitized = regexp.MustCompile(`\s{2,}`).ReplaceAllString(sanitized, " ")
	sanitized = strings.Trim(sanitized, ". ")
	maxLen := 200
	if len(sanitized) > maxLen {
		lastSpace := strings.LastIndex(sanitized[:maxLen], " ")
		if lastSpace != -1 {
			sanitized = sanitized[:lastSpace]
		} else {
			sanitized = sanitized[:maxLen]
		}
	}
	if sanitized == "" {
		return "downloaded_book"
	}
	return sanitized
}

// PtrStr returns a pointer to a string, or nil if the string is empty.
func PtrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// FormatBytes converts bytes into a human-readable string (KB, MB, GB).
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// MinInt returns the smaller of two integers.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
