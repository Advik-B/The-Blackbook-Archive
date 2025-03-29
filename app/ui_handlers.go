package app

import (
	"fmt"
	"io" // Added
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings" // Added

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	// Adjust import paths
	"The.Blackbook.Archive/download"
	"The.Blackbook.Archive/utils"
	zlib "The.Blackbook.Archive/zlibrary"
)

// performSearch handles the search button click and entry submission.
func (g *guiApp) performSearch(query string) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		g.setStatus("Please enter a search query.")
		return
	}

	g.setStatus(fmt.Sprintf("Searching for '%s'...", trimmedQuery))
	g.searchButton.Disable()
	g.searchEntry.Disable()
	g.resultsList.UnselectAll()
	g.searchResults = nil // Clear previous results
	g.resultsList.Refresh()
	g.clearDetails() // Clear details pane

	go func() {
		results, finalURL, err := zlib.SearchZLibrary(trimmedQuery)
		searchErrStr := ""
		if err != nil {
			searchErrStr = fmt.Sprintf("Search failed: %v", err) // Include error message
			log.Printf("Search Error (URL: %s): %v\n", finalURL, err)
		}

		// Update UI back on the main Fyne thread
		// Fyne internally ensures thread safety for widget updates.
		// No need for fyne.CurrentApp().Driver().RunChecked() typically.
		g.searchButton.Enable()
		g.searchEntry.Enable()

		if searchErrStr != "" {
			g.setStatus(fmt.Sprintf("Search completed with errors (URL: %s).", finalURL))
			dialog.ShowError(fmt.Errorf(searchErrStr), g.mainWin) // Show error dialog
			// Still display partial results if any were found before the error
			g.searchResults = results
			g.resultsList.Refresh()
		} else {
			g.setStatus(fmt.Sprintf("Found %d results for '%s'. Select one.", len(results), trimmedQuery))
			g.searchResults = results
			if len(results) == 0 {
				g.setStatus(fmt.Sprintf("No results found for '%s'.", trimmedQuery))
			}
			g.resultsList.Refresh() // Refresh list with new data
		}
	}()
}

// fetchAndDisplayDetails handles the selection of a book from the results list.
func (g *guiApp) fetchAndDisplayDetails(bookURL string) {
	g.setStatus(fmt.Sprintf("Fetching details for %s...", bookURL))
	g.clearDetails()           // Clear previous details immediately
	g.downloadButton.Disable() // Disable download until details are loaded

	go func() {
		details, finalURL, err := zlib.GetBookDetails(bookURL)
		var fetchErr error
		if err != nil {
			fetchErr = fmt.Errorf("failed to get details from %s: %w", finalURL, err)
			log.Printf("Detail Fetch Error (URL: %s): %v\n", finalURL, err)
		}

		var coverResource fyne.Resource
		var coverErr error
		if fetchErr == nil && details != nil && details.CoverURL != nil {
			// Check cache first
			g.cacheMutex.RLock()
			cachedRes, found := g.imageCache[*details.CoverURL]
			g.cacheMutex.RUnlock()

			if found {
				coverResource = cachedRes
			} else {
				coverResource, coverErr = g.fetchImageResource(*details.CoverURL) // Use helper
				if coverErr == nil && coverResource != nil {
					g.cacheMutex.Lock()
					g.imageCache[*details.CoverURL] = coverResource // Cache fetched resource
					g.cacheMutex.Unlock()
				} else if coverErr != nil {
					log.Printf("Failed to fetch cover image %s: %v\n", *details.CoverURL, coverErr)
				}
			}
		}

		// Update UI on main thread
		if fetchErr != nil {
			g.setStatus(fmt.Sprintf("Error fetching details (URL: %s).", finalURL))
			dialog.ShowError(fetchErr, g.mainWin)
			g.selectedBook = nil // Ensure selected book is nil on error
			// Do not enable download button
			return
		}

		// Successfully fetched details
		g.selectedBook = details // Store the fetched details

		// Update Cover Image
		if coverErr != nil {
			g.setStatus(fmt.Sprintf("Details loaded, but failed to load cover: %v", coverErr))
			if g.coverImage != nil {
				g.coverImage.Resource = nil
			}
		} else if coverResource != nil {
			g.setStatus("Details loaded.")
			if g.coverImage != nil {
				g.coverImage.Resource = coverResource
			}
		} else {
			g.setStatus("Details loaded. No cover image found.")
			if g.coverImage != nil {
				g.coverImage.Resource = nil
			}
		}
		if g.coverImage != nil {
			g.coverImage.Refresh()
		}

		// Populate Text Details
		g.populateDetailsArea(details) // Use helper

		// Update Download Button
		if details.DownloadURL != nil && *details.DownloadURL != "" && *details.DownloadURL != "CONVERSION_NEEDED" {
			g.downloadButton.Enable()
			buttonText := "Download"
			if details.FileFormat != nil {
				buttonText += fmt.Sprintf(" (%s", *details.FileFormat)
				if details.FileSize != nil {
					buttonText += fmt.Sprintf(", %s", *details.FileSize)
				}
				buttonText += ")"
			}
			g.downloadButton.SetText(buttonText)
		} else {
			g.downloadButton.Disable()
			if details.DownloadURL != nil && *details.DownloadURL == "CONVERSION_NEEDED" {
				g.downloadButton.SetText("Download (Conversion)")
			} else {
				g.downloadButton.SetText("Download (N/A)")
			}
		}

	}()
}

// performDownload handles the download button click.
func (g *guiApp) performDownload() { // Takes no arguments, uses g.selectedBook
	if g.selectedBook == nil {
		dialog.ShowError(fmt.Errorf("no book selected for download"), g.mainWin)
		return
	}
	details := g.selectedBook // Use the currently selected book

	if details.DownloadURL == nil || *details.DownloadURL == "" {
		dialog.ShowError(fmt.Errorf("no primary download URL found for the selected book"), g.mainWin)
		return
	}
	if *details.DownloadURL == "CONVERSION_NEEDED" {
		dialog.ShowError(fmt.Errorf("primary download URL indicates conversion needed, cannot download directly"), g.mainWin)
		return
	}

	// Prepare filename and path
	if err := os.MkdirAll(utils.DownloadDir, 0755); err != nil {
		dialog.ShowError(fmt.Errorf("failed to create download directory '%s': %w", utils.DownloadDir, err), g.mainWin)
		return
	}
	author := "Unknown Author"
	if details.Author != nil && *details.Author != "" {
		author = *details.Author
	}
	title := "Unknown Title"
	if details.Title != nil && *details.Title != "" {
		title = *details.Title
	}
	ext := "bin" // Default extension if not found
	if details.FileFormat != nil && *details.FileFormat != "" {
		ext = strings.ToLower(*details.FileFormat) // Ensure lowercase extension
	}

	baseFilename := fmt.Sprintf("%s - %s", author, title)
	sanitizedBase := utils.SanitizeFilename(baseFilename)
	filename := fmt.Sprintf("%s.%s", sanitizedBase, ext)
	filePath := filepath.Join(utils.DownloadDir, filename)

	// Check if file exists and ask user
	if _, err := os.Stat(filePath); err == nil {
		dialog.ShowConfirm("File Exists", fmt.Sprintf("'%s' already exists.\nOverwrite?", filename), func(overwrite bool) {
			if overwrite {
				g.startActualDownload(details, filePath, filename)
			} else {
				g.setStatus("Download cancelled.")
			}
		}, g.mainWin)
	} else if !os.IsNotExist(err) {
		// Handle other stat errors (like permission issues)
		dialog.ShowError(fmt.Errorf("could not check for existing file '%s': %w", filePath, err), g.mainWin)
	} else {
		// File does not exist, proceed directly
		g.startActualDownload(details, filePath, filename)
	}
}

// startActualDownload performs the network request and file writing part of the download.
func (g *guiApp) startActualDownload(details *zlib.BookDetails, filePath, filename string) {

	g.setStatus(fmt.Sprintf("Starting download: %s", filename))
	g.downloadButton.Disable() // Disable while downloading
	g.progressBar.SetValue(0)
	g.progressBar.Show()

	progCh := make(chan download.DownloadProgress, 5) // Buffered channel

	go func() {
		defer close(progCh) // Ensure channel is closed when goroutine exits

		downloadURL := *details.DownloadURL
		log.Printf("Attempting download from: %s", downloadURL)

		resp, err := zlib.MakeRequest(downloadURL)
		if err != nil {
			progCh <- download.DownloadProgress{Err: fmt.Errorf("download request failed for %s: %w", downloadURL, err)}
			return
		}
		defer resp.Body.Close()

		finalURL := downloadURL
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
			if finalURL != downloadURL {
				log.Printf("Download redirected to: %s", finalURL)
			}
		}

		// Check for HTML response which often indicates login/captcha page
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(strings.ToLower(contentType), "text/html") {
			bodyBytes, _ := io.ReadAll(resp.Body) // Read the HTML body
			errMsg := fmt.Sprintf("download failed: Received an HTML page (URL: %s). Login/Captcha/Limit likely required.", finalURL)
			log.Println(errMsg) // Log the error
			if len(bodyBytes) > 0 {
				// Use utils.MinInt
				snippet := string(bodyBytes[:utils.MinInt(500, len(bodyBytes))])
				log.Printf("HTML Response Snippet: %s", snippet)
			}
			progCh <- download.DownloadProgress{Err: fmt.Errorf(errMsg)}
			return
		}

		// Check for non-OK status codes
		if resp.StatusCode != 200 { // Use http.StatusOK
			bodyBytes, _ := io.ReadAll(resp.Body)
			errMsg := fmt.Sprintf("download failed with status %s (URL: %s): %s", resp.Status, finalURL, string(bodyBytes))
			log.Println(errMsg)
			progCh <- download.DownloadProgress{Err: fmt.Errorf(errMsg)}
			return
		}

		// Get total size for progress bar
		totalSize, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if totalSize <= 0 {
			log.Println("Warning: Content-Length header missing or invalid. Progress percentage will be inaccurate.")
			totalSize = -1 // Indicate unknown size
		}
		// Send initial progress state (0 bytes, total size known/unknown)
		progCh <- download.DownloadProgress{Total: totalSize, Current: 0}

		// Create output file
		outFile, err := os.Create(filePath)
		if err != nil {
			progCh <- download.DownloadProgress{Err: fmt.Errorf("failed to create output file '%s': %w", filePath, err)}
			return
		}
		// Use defer with error check for closing the file
		defer func() {
			if cerr := outFile.Close(); cerr != nil {
				log.Printf("Warning: failed to close output file '%s': %v\n", filePath, cerr)
				// If closing fails after a successful copy, the file might be okay, but log it.
				// If copy failed, the file is likely already removed.
			}
		}()

		// Create progress writer and copy data
		writer := download.NewProgressWriter(outFile, totalSize, progCh)
		writtenBytes, copyErr := io.Copy(writer, resp.Body)

		// Handle copy result
		if copyErr != nil {
			log.Printf("Download copy error after writing %d bytes: %v\n", writtenBytes, copyErr)
			// Attempt to remove the partially downloaded file
			// Close is deferred, but try removing now.
			// Explicit close before remove can sometimes help release file handles on some OSes,
			// but defer should generally handle it. Let's rely on defer.
			if remErr := os.Remove(filePath); remErr != nil {
				log.Printf("Warning: failed to remove partially downloaded file '%s': %v\n", filePath, remErr)
			}
			// Send the error through the progress channel
			progCh <- download.DownloadProgress{Current: writer.Current, Total: writer.Total, Err: fmt.Errorf("download interrupted: %w", copyErr)}
		} else {
			log.Printf("Download successful, wrote %d bytes to %s\n", writtenBytes, filePath)
			// Send final progress update indicating completion (error is nil)
			// This ensures the receiver knows the download finished without copy errors.
			progCh <- download.DownloadProgress{Current: writer.Current, Total: writer.Total, Err: nil}
		}

	}()

	// --- Handle Progress Updates in UI Goroutine ---
	// This part MUST run on the main Fyne goroutine to update UI widgets safely.
	// We launch another goroutine to *read* from the channel and then update the UI.
	go func() {
		var lastError error
		var finalBytes int64
		var totalBytes int64 = -1 // Initialize as unknown

		for update := range progCh { // Read updates from the download goroutine
			lastError = update.Err
			finalBytes = update.Current
			if update.Total != 0 { // Capture total size if provided
				totalBytes = update.Total
			}

			if update.Err != nil {
				break // Exit loop on error
			}

			// Update progress bar and status
			if totalBytes > 0 {
				g.progressBar.Max = float64(totalBytes)
				g.progressBar.SetValue(float64(finalBytes))
				g.setStatus(fmt.Sprintf("Downloading %s... %s / %s", filename, utils.FormatBytes(finalBytes), utils.FormatBytes(totalBytes)))
			} else { // Unknown total size
				// Indicate indeterminate progress or just show bytes downloaded
				g.progressBar.Max = 1     // Or hide Max value if possible
				g.progressBar.SetValue(0) // Cannot show percentage
				g.setStatus(fmt.Sprintf("Downloading %s... %s / ???", filename, utils.FormatBytes(finalBytes)))
			}

			// Check if download seems complete based on bytes (only if total is known)
			if totalBytes > 0 && finalBytes >= totalBytes {

				// Don't break here, wait for the channel to close or an explicit nil error
				// to confirm the download goroutine finished cleanly.
			}
		}

		// --- Final UI Update after loop finishes ---
		g.progressBar.Hide()
		// Re-enable button only if the *currently selected* book is the one that was attempted
		// Check g.selectedBook against the 'details' captured at the start of performDownload
		if g.selectedBook != nil && details != nil && g.selectedBook.URL == details.URL {
			g.downloadButton.Enable()
		} else {
			g.downloadButton.Disable() // Keep disabled if selection changed
		}

		// Show final status/dialog
		if lastError != nil {
			g.setStatus(fmt.Sprintf("Download failed for %s.", filename))
			dialog.ShowError(lastError, g.mainWin)
		} else {
			// If no error occurred, assume success.
			// The check for 'downloadComplete' based on bytes written vs total size
			// is a secondary check, the primary indicator is lack of error.
			g.setStatus(fmt.Sprintf("Download complete: %s", filePath))
			dialog.ShowInformation("Download Complete", fmt.Sprintf("Saved book to:\n%s", filePath), g.mainWin)
		}

	}()
}
