package app

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	zlib "The.Blackbook.Archive/zlibrary" // Adjust import paths
)

// setStatus updates the status bar label safely.
func (g *guiApp) setStatus(message string) {
	g.statusBar.SetText(message)
}

// fetchImageResource fetches an image URL and returns it as a Fyne resource.
func (g *guiApp) fetchImageResource(imageURL string) (fyne.Resource, error) {
	log.Printf("Fetching image: %s", imageURL)
	resp, err := zlib.MakeRequest(imageURL) // Use zlib client
	if err != nil {
		return nil, fmt.Errorf("image request failed for %s: %w", imageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch image %s, status: %s", imageURL, resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("unexpected content type '%s' for image URL %s", contentType, imageURL)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data from %s: %w", imageURL, err)
	}
	if len(imgBytes) == 0 {
		return nil, fmt.Errorf("downloaded image data is empty for %s", imageURL)
	}

	filenameHint := "cover_image"
	parsedURL, err := url.Parse(imageURL)
	if err == nil && parsedURL.Path != "" {
		filenameHint = filepath.Base(parsedURL.Path)
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
		// Replace content inside the container managed by the scroll view
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
		g.clearDetails()
		return
	}

	// Use a slice to collect pairs of widgets for the Form layout
	formItems := []*widget.FormItem{}

	// Helper to add a detail row (label + value) to the FormItems slice
	addDetail := func(label string, value *string, isURL ...bool) {
		if value != nil && *value != "" {
			var content fyne.CanvasObject
			valStr := *value
			if len(isURL) > 0 && isURL[0] {
				parsedURL, err := url.Parse(valStr)
				if err == nil {
					displayURL := valStr
					if len(displayURL) > 70 {
						displayURL = displayURL[:67] + "..."
					}
					content = widget.NewHyperlink(displayURL, parsedURL)
				} else {
					log.Printf("Warning: Failed to parse '%s' as URL for label '%s': %v", valStr, label, err)
					content = widget.NewLabel(valStr)
					content.(*widget.Label).Wrapping = fyne.TextWrapWord // Wrap even if it's a fallback URL
				}
			} else {
				content = widget.NewLabel(valStr)
				content.(*widget.Label).Wrapping = fyne.TextWrapWord // Wrap normal text
			}
			formItems = append(formItems, widget.NewFormItem(label, content))
		}
	}

	// Helper to add a row with a custom widget as the value
	addDetailRich := func(label string, content fyne.CanvasObject) {
		if content != nil {
			formItems = append(formItems, widget.NewFormItem(label, content))
		}
	}

	// --- Add details using helpers ---
	// Use simpler labels for FormItem
	addDetail("Title", details.Title)
	addDetail("Author", details.Author)
	addDetail("Author Link", details.AuthorURL, true)

	if details.Description != nil && *details.Description != "" {
		descLabel := widget.NewLabel(*details.Description)
		descLabel.Wrapping = fyne.TextWrapWord
		// For description, using addDetailRich makes it span both columns of the form, which is better
		// We'll create the form below, and add description separately if needed, or just use addDetailRich.
		// Let's use addDetailRich here, it handles the layout internally.
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
		catBox := container.NewVBox() // Use VBox for multiple category links/labels
		hasContent := false
		for _, cat := range details.Categories {
			if cat.Name == "" {
				continue
			}
			hasContent = true
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
		if hasContent {
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
			fileInfo += " "
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
	addDetail("Book ID", details.BookID)
	addDetail("IPFS CID", details.IpfsCID)
	addDetail("Cover URL", details.CoverURL, true)
	addDetail("Primary DL URL", details.DownloadURL, true)

	// Other Formats
	if len(details.OtherFormats) > 0 {
		otherFmtBox := container.NewVBox() // Use VBox
		hasContent := false
		for _, f := range details.OtherFormats {
			text := f.Format
			if text == "" {
				continue // Skip if format name is empty
			}
			hasContent = true

			if f.URL == "CONVERSION_NEEDED" {
				otherFmtBox.Add(widget.NewLabel(fmt.Sprintf("%s (Conversion Needed)", text)))
			} else {
				// Just display the format name for simplicity in the form layout
				otherFmtBox.Add(widget.NewLabel(fmt.Sprintf("%s", text)))
			}
		}
		if hasContent {
			addDetailRich("Other Formats", otherFmtBox)
		}
	}

	// Create the Form using the collected items
	detailsForm := widget.NewForm(formItems...)

	// Replace the content of detailsArea with the new form
	// Wrap the form in a VBox in case we want to add other things above/below it later easily
	g.detailsArea.Objects = []fyne.CanvasObject{container.NewVBox(detailsForm)}
	g.detailsArea.Refresh()
	g.detailsContainer.ScrollToTop() // Ensure view starts at the top
}
