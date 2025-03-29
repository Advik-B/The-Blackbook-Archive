package app

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"path/filepath" // Added
	"strings"       // Added

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	// Adjust import paths
	zlib "The.Blackbook.Archive/zlibrary"
)

// setStatus updates the status bar label safely.
func (g *guiApp) setStatus(message string) {
	// Ensure UI updates run on the Fyne goroutine
	// No need for separate go func() if called from handlers already on main thread or
	// if the calling goroutine manages its UI updates correctly.
	// Fyne handles thread safety internally for widget updates.
	g.statusBar.SetText(message)
}

// fetchImageResource fetches an image URL and returns it as a Fyne resource.
func (g *guiApp) fetchImageResource(imageURL string) (fyne.Resource, error) {
	log.Printf("Fetching image: %s", imageURL)
	resp, err := zlib.MakeRequest(imageURL) // Use zlib client
	if err != nil {
		return nil, fmt.Errorf("image request failed for %s: %w", imageURL, err)
	}
	defer resp.Body.Close() // Ensure body is closed

	if resp.StatusCode != 200 { // Use http.StatusOK
		return nil, fmt.Errorf("failed to fetch image %s, status: %s", imageURL, resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("unexpected content type '%s' for image URL %s", contentType, imageURL)
	}

	imgBytes, err := io.ReadAll(resp.Body) // Use io.ReadAll
	if err != nil {
		return nil, fmt.Errorf("failed to read image data from %s: %w", imageURL, err)
	}
	if len(imgBytes) == 0 {
		return nil, fmt.Errorf("downloaded image data is empty for %s", imageURL)
	}

	// Create a filename hint from the URL path
	filenameHint := "cover_image" // Default
	parsedURL, err := url.Parse(imageURL)
	if err == nil && parsedURL.Path != "" {
		filenameHint = filepath.Base(parsedURL.Path) // Get the last part of the path
	}

	return fyne.NewStaticResource(filenameHint, imgBytes), nil
}

// clearDetails resets the details pane to its initial state.
func (g *guiApp) clearDetails() {
	g.selectedBook = nil
	if g.coverImage != nil {
		g.coverImage.Resource = nil
		g.coverImage.Refresh()
	}
	if g.detailsArea != nil {
		// Keep the container, just replace objects
		g.detailsArea.Objects = []fyne.CanvasObject{widget.NewLabel("Select a book from the results to see details.")}
		g.detailsArea.Refresh()
	}
	if g.detailsContainer != nil {
		g.detailsContainer.ScrollToTop() // Scroll back up when cleared
	}
	if g.downloadButton != nil {
		g.downloadButton.Disable()
		g.downloadButton.SetText("Download")
	}
	if g.progressBar != nil {
		g.progressBar.Hide()
		g.progressBar.SetValue(0)
	}

}

// populateDetailsArea fills the details UI elements with book information.
func (g *guiApp) populateDetailsArea(details *zlib.BookDetails) {
	if details == nil {
		g.clearDetails() // Clear if details are nil
		return
	}

	var widgets []fyne.CanvasObject

	// Helper to add a detail row (label + value)
	addDetail := func(label string, value *string, isURL ...bool) {
		if value != nil && *value != "" {
			var content fyne.CanvasObject
			valStr := *value
			if len(isURL) > 0 && isURL[0] {
				parsedURL, err := url.Parse(valStr)
				if err == nil {
					// Shorten long URLs for display
					displayURL := valStr
					if len(displayURL) > 70 {
						displayURL = displayURL[:67] + "..."
					}
					content = widget.NewHyperlink(displayURL, parsedURL)
				} else {
					log.Printf("Warning: Failed to parse '%s' as URL for label '%s': %v", valStr, label, err)
					content = widget.NewLabel(valStr) // Fallback to label
				}
			} else {
				content = widget.NewLabel(valStr)
				content.(*widget.Label).Wrapping = fyne.TextWrapWord
			}
			// Use a Grid layout for better alignment
			// widgets = append(widgets, container.NewBorder(nil, nil, widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, content))
			widgets = append(widgets, widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
			widgets = append(widgets, content)
		}
	}

	// Helper to add a row with a custom widget as the value
	addDetailRich := func(label string, content fyne.CanvasObject) {
		if content != nil {
			// widgets = append(widgets, container.NewBorder(nil, nil, widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, content))
			widgets = append(widgets, widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
			widgets = append(widgets, content)
		}
	}

	// --- Add details using helpers ---
	addDetail("Title", details.Title)
	addDetail("Author", details.Author)
	addDetail("Author Link", details.AuthorURL, true)

	if details.Description != nil && *details.Description != "" {
		descLabel := widget.NewLabel(*details.Description)
		descLabel.Wrapping = fyne.TextWrapWord
		addDetailRich("Description", descLabel)
	}

	// Combine Ratings
	ratingStr := ""
	if details.RatingInterest != nil && *details.RatingInterest != "" {
		ratingStr += "Interest: " + *details.RatingInterest
	}
	if details.RatingQuality != nil && *details.RatingQuality != "" {
		if ratingStr != "" {
			ratingStr += " / "
		}
		ratingStr += "Quality: " + *details.RatingQuality
	}
	if ratingStr != "" {
		addDetail("Rating", &ratingStr)
	}

	// Categories
	if len(details.Categories) > 0 {
		catBox := container.NewVBox()
		for _, cat := range details.Categories {
			if cat.Name == "" {
				continue
			} // Skip empty category names
			if cat.URL != nil && *cat.URL != "" {
				parsedURL, err := url.Parse(*cat.URL)
				if err == nil {
					catBox.Add(widget.NewHyperlink(cat.Name, parsedURL))
				} else {
					log.Printf("Warning: Failed to parse category URL '%s': %v", *cat.URL, err)
					catBox.Add(widget.NewLabel(cat.Name)) // Fallback
				}
			} else {
				catBox.Add(widget.NewLabel(cat.Name))
			}
		}
		if len(catBox.Objects) > 0 {
			addDetailRich("Categories", catBox)
		}
	}

	// Combine File Info
	fileInfo := ""
	if details.FileFormat != nil && *details.FileFormat != "" {
		fileInfo += *details.FileFormat
	}
	if details.FileSize != nil && *details.FileSize != "" {
		if fileInfo != "" {
			fileInfo += " " // Just a space, looks better
		}
		fileInfo += "(" + *details.FileSize + ")"
	}
	if fileInfo != "" {
		addDetail("File", &fileInfo)
	}

	addDetail("Language", details.Language)
	addDetail("Year", details.Year)
	addDetail("Publisher", details.Publisher)
	addDetail("Series", details.Series)
	addDetail("Volume", details.Volume)
	addDetail("ISBN 10", details.ISBN10)
	addDetail("ISBN 13", details.ISBN13)
	addDetail("Content Type", details.ContentType)
	addDetail("Book ID", details.BookID) // Can be useful for debugging/reporting
	addDetail("IPFS CID", details.IpfsCID)
	// addDetail("IPFS CID Blake2b", details.IpfsCIDBlake2b) // Often less relevant for user
	addDetail("Cover URL", details.CoverURL, true)         // Show the URL
	addDetail("Primary DL URL", details.DownloadURL, true) // Show the URL

	// Other Formats
	if len(details.OtherFormats) > 0 {
		otherFmtBox := container.NewVBox()
		for _, f := range details.OtherFormats {
			text := f.Format
			if text == "" {
				text = "??"
			}

			if f.URL == "CONVERSION_NEEDED" {
				otherFmtBox.Add(widget.NewLabel(fmt.Sprintf("%s (Conversion)", text)))
			} else if f.URL != "" {
				// Create a small button or link for other formats? For now, just label.
				// parsedURL, err := url.Parse(f.URL)
				// if err == nil {
				// 	otherFmtBox.Add(widget.NewHyperlink(fmt.Sprintf("%s (%s)", text, f.URL), parsedURL))
				// } else {
				otherFmtBox.Add(widget.NewLabel(fmt.Sprintf("%s", text))) // Just show format name
				// }
			} else {
				otherFmtBox.Add(widget.NewLabel(text)) // Should not happen if URL is required unless CONVERSION_NEEDED
			}
		}
		if len(otherFmtBox.Objects) > 0 {
			addDetailRich("Other Formats", otherFmtBox)
		}
	}

	// Use a Grid layout with 2 columns for the details
	grid := container.New(layout.NewFormLayout(), widgets...)

	g.detailsArea.Objects = []fyne.CanvasObject{grid} // Replace content
	g.detailsArea.Refresh()
	g.detailsContainer.ScrollToTop() // Ensure view starts at the top
}
