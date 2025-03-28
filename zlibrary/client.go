package zlibrary

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url" // Added
	"time"
)

// --- Constants ---
const (
	BaseURL    = "https://z-library.sk" // WARNING: This domain might change/be blocked. Exported
	SearchPath = "/s/"                  // Exported
)

// --- Global HTTP Client ---
var httpClient *http.Client
var lastReferrer string

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Failed to create cookie jar: %v", err)
	}
	httpClient = &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second, // Increased timeout
	}
	// Initialize lastReferrer with the base URL or a sensible default
	baseURLParsed, err := url.Parse(BaseURL)
	if err != nil {
		log.Fatalf("Failed to parse base URL: %v", err) // Fail fast if base URL is invalid
	}
	lastReferrer = baseURLParsed.String()
}

// MakeRequest executes an HTTP GET request using the shared client and headers.
// It automatically handles the Referer header based on the last successful request.
// The caller is responsible for closing the response body.
func MakeRequest(urlStr string) (*http.Response, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", urlStr, err)
	}

	// Set common headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("DNT", "1") // Do Not Track
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if lastReferrer != "" {
		req.Header.Set("Referer", lastReferrer)
	}

	// Execute request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed for %s: %w", urlStr, err)
	}

	// Update lastReferrer based on the final URL after redirects
	finalURL := urlStr // Default to original URL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	// Update referrer only on success or redirection leading eventually to success/handled error
	// This prevents setting referrer to an error page URL in many cases
	if resp.StatusCode >= 200 && resp.StatusCode < 400 { // Consider 2xx and 3xx as "navigated"
		lastReferrer = finalURL
	} else {
		// Optionally log or handle non-2xx/3xx status codes where referrer might be sensitive
		// log.Printf("Request to %s resulted in status %d. Referrer not updated.", finalURL, resp.StatusCode)
	}

	return resp, nil // Caller MUST close resp.Body
}
