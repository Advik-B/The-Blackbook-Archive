package zlibrary

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	// Assumes utils package is in the parent directory or GOPATH
	"The.Blackbook.Archive/utils"
)

// SearchZLibrary performs a search on Z-Library and returns results.
// It returns the search results, the final URL accessed, and any error encountered.
func SearchZLibrary(query string) ([]BookSearchResult, string, error) {
	if query == "" {
		return nil, "", fmt.Errorf("search query cannot be empty")
	}

	// Construct search URL
	searchURL, err := url.Parse(BaseURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse base URL: %w", err)
	}
	searchURL = searchURL.JoinPath(SearchPath) // Use JoinPath for cleaner path segment joining
	q := searchURL.Query()
	q.Set("q", query)
	searchURL.RawQuery = q.Encode()

	fetchURLStr := searchURL.String()
	log.Printf("Searching URL: %s\n", fetchURLStr) // Log the URL being fetched

	resp, err := MakeRequest(fetchURLStr)
	if err != nil {
		// Return the intended fetch URL even on request error
		return nil, fetchURLStr, fmt.Errorf("search request failed: %w", err)
	}
	// Defer closing with error handling
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			// Log closing error, but prioritize returning the original error if any
			log.Printf("Warning: failed to close search response body: %v\n", cerr)
		}
	}()

	// Determine the final URL after potential redirects
	finalURLStr := fetchURLStr
	if resp.Request != nil && resp.Request.URL != nil {
		finalURLStr = resp.Request.URL.String()
		if finalURLStr != fetchURLStr {
			log.Printf("Redirected to: %s\n", finalURLStr)
		}
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("Warning: failed to read error response body: %v\n", readErr)
		}
		return nil, finalURLStr, fmt.Errorf("search failed with status %s\nURL: %s\nResponse: %s", resp.Status, finalURLStr, string(bodyBytes))
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, finalURLStr, fmt.Errorf("failed to parse search results HTML from %s: %w", finalURLStr, err)
	}

	// Extract results
	var results []BookSearchResult
	var parseErrors []string
	doc.Find("div#searchResultBox div.book-item.resItemBoxBooks").Each(func(i int, s *goquery.Selection) {
		bookCard := s.Find("z-bookcard").First() // Use First() for potentially single element
		if bookCard.Length() == 0 {
			// Try finding direct link if z-bookcard fails (potential layout change)
			directLink := s.Find("h3[itemprop='name'] a").First()
			if directLink.Length() == 0 {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Skipping, could not find z-bookcard or direct title link.", i))
				return
			}
			// If direct link found, extract info from there (less reliable)
			relativeURL, exists := directLink.Attr("href")
			if !exists || relativeURL == "" {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Skipping, found direct link but missing href.", i))
				return
			}
			title := strings.TrimSpace(directLink.Text())
			bookID := "" // Cannot easily get ID this way

			base, err := url.Parse(finalURLStr)
			if err != nil {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Error parsing final search URL '%s' as base: %v", i, finalURLStr, err))
				base, _ = url.Parse(BaseURL) // Fallback
			}

			resolvedURL, err := base.Parse(relativeURL)
			if err != nil {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Error resolving book URL '%s' against base '%s': %v", i, relativeURL, base.String(), err))
				return
			}

			results = append(results, BookSearchResult{
				Title:  title,
				URL:    resolvedURL.String(),
				BookID: utils.PtrStr(bookID), // Will be nil if empty
			})

		} else {
			// Standard extraction using z-bookcard
			relativeURL, exists := bookCard.Attr("href")
			if !exists || relativeURL == "" {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Skipping book card, missing href attribute.", i))
				return
			}

			bookID, _ := bookCard.Attr("id") // Get ID if available
			title := "Title N/A"
			titleSlot := bookCard.Find("div[slot='title']").First() // More specific? Check HTML structure
			if titleSlot.Length() > 0 {
				title = strings.TrimSpace(titleSlot.Text())
			} else {
				// Fallback title extraction if slot fails
				titleFallback := s.Find("h3[itemprop='name'] a").First()
				if titleFallback.Length() > 0 {
					title = strings.TrimSpace(titleFallback.Text())
				}
			}

			base, err := url.Parse(finalURLStr)
			if err != nil {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Error parsing final search URL '%s' as base: %v", i, finalURLStr, err))
				base, _ = url.Parse(BaseURL) // Fallback
			}

			resolvedURL, err := base.Parse(relativeURL)
			if err != nil {
				parseErrors = append(parseErrors, fmt.Sprintf("Item %d: Error resolving book URL '%s' against base '%s': %v", i, relativeURL, base.String(), err))
				return
			}

			results = append(results, BookSearchResult{
				Title:  title,
				URL:    resolvedURL.String(),
				BookID: utils.PtrStr(bookID), // Use utility function
			})
		}
	})

	// Report parsing errors if any
	if len(parseErrors) > 0 {
		// Return results found so far, but also the error
		return results, finalURLStr, fmt.Errorf("encountered %d errors during result parsing:\n- %s", len(parseErrors), strings.Join(parseErrors, "\n- "))
	}

	return results, finalURLStr, nil
}

// GetBookDetails fetches and parses the details page for a given book URL.
// It returns the details, the final URL accessed, and any error encountered.
func GetBookDetails(bookURL string) (*BookDetails, string, error) {
	log.Printf("Fetching details for: %s\n", bookURL)

	resp, err := MakeRequest(bookURL)
	if err != nil {
		return nil, bookURL, fmt.Errorf("detail page request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("Warning: failed to close detail response body: %v\n", cerr)
		}
	}()

	finalURLStr := bookURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURLStr = resp.Request.URL.String()
		if finalURLStr != bookURL {
			log.Printf("Redirected to: %s\n", finalURLStr)
		}
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("Warning: failed to read error response body: %v\n", readErr)
		}
		bodySnippet := string(bodyBytes)
		// Use utils.MinInt
		if len(bodySnippet) > 500 {
			bodySnippet = bodySnippet[:utils.MinInt(500, len(bodySnippet))] + "..."
		}
		return nil, finalURLStr, fmt.Errorf("detail page failed with status %s\nURL: %s\nResponse: %s", resp.Status, finalURLStr, bodySnippet)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, finalURLStr, fmt.Errorf("failed to parse detail page HTML from %s: %w", finalURLStr, err)
	}

	details := BookDetails{URL: bookURL} // Initialize with original URL

	// Base URL for resolving relative links from the *final* page URL
	base, err := url.Parse(finalURLStr)
	if err != nil {
		log.Printf("Warning: Error parsing final detail URL '%s' as base: %v\n", finalURLStr, err)
		base, _ = url.Parse(BaseURL) // Fallback to constant base URL
	}

	// --- Extract Details using goquery ---

	// Book ID (try multiple methods)
	bookCardTag := doc.Find("z-bookcard").First() // Often present on detail page too
	if id, exists := bookCardTag.Attr("id"); exists && id != "" {
		details.BookID = utils.PtrStr(id)
	}
	if details.BookID == nil { // If not found in z-bookcard, try URL regex
		r := regexp.MustCompile(`/book/(\d+)/`)
		matches := r.FindStringSubmatch(finalURLStr) // Try final URL first
		if len(matches) > 1 {
			details.BookID = utils.PtrStr(matches[1])
		} else {
			matches = r.FindStringSubmatch(bookURL) // Fallback to original URL
			if len(matches) > 1 {
				details.BookID = utils.PtrStr(matches[1])
			}
		}
	}
	// Add another fallback using a meta tag if needed:
	// if details.BookID == nil {
	//  if metaContent, exists := doc.Find("meta[name='book_id']").Attr("content"); exists {
	//      details.BookID = utils.PtrStr(metaContent)
	//  }
	// }

	// Title
	titleElem := doc.Find("h1.book-title[itemprop='name']").First()
	details.Title = utils.PtrStr(strings.TrimSpace(titleElem.Text()))

	// Author
	// Selector might need adjustment based on actual page structure
	mainContent := doc.Find("div.book-main-info").First() // Try a more specific container
	if mainContent.Length() == 0 {
		mainContent = doc.Find("div.col-sm-9").First() // Fallback
	}
	authorTag := mainContent.Find("a.color1[itemprop='author'], a.color1[href*='/g/']").First() // Try itemprop first, then common author link pattern
	details.Author = utils.PtrStr(strings.TrimSpace(authorTag.Text()))
	if href, exists := authorTag.Attr("href"); exists {
		resolved, err := base.Parse(href)
		if err == nil {
			details.AuthorURL = utils.PtrStr(resolved.String())
		} else {
			log.Printf("Warning: Failed to resolve author URL '%s': %v\n", href, err)
			// Don't assign AuthorURL if resolution fails
		}
	}

	// Description
	descElem := doc.Find("div#bookDescriptionBox div[itemprop='description']").First() // Use itemprop
	if descElem.Length() == 0 {
		descElem = doc.Find("div#bookDescriptionBox").First() // Fallback
	}
	// Handle potential "truncated" descriptions needing expansion (more complex, skip for now)
	details.Description = utils.PtrStr(strings.TrimSpace(descElem.Text()))

	// Ratings (Selectors might change)
	details.RatingInterest = utils.PtrStr(strings.TrimSpace(doc.Find("span.book-rating-interest-score, .rating-interest .rating-value").First().Text()))
	details.RatingQuality = utils.PtrStr(strings.TrimSpace(doc.Find("span.book-rating-quality-score, .rating-quality .rating-value").First().Text()))

	// Properties (Robust extraction)
	propertiesMap := make(map[string]string) // Store raw key-value
	propertiesList := []Category{}           // For Categories specifically

	doc.Find("div.bookDetailsBox div.property, div.properties div.property").Each(func(i int, s *goquery.Selection) {
		labelElem := s.Find(".property_label, .property__label").First()
		valueElem := s.Find(".property_value, .property__value").First()

		label := strings.ToLower(strings.TrimSpace(labelElem.Text()))
		label = strings.TrimSuffix(label, ":")
		label = strings.ReplaceAll(label, " ", "_") // Normalize label

		// Handle specific cases like categories first
		if label == "categories" {
			valueElem.Find("a").Each(func(j int, link *goquery.Selection) {
				catName := strings.TrimSpace(link.Text())
				var catURLStr *string
				if href, exists := link.Attr("href"); exists {
					resolved, err := base.Parse(href)
					if err == nil {
						catURLStr = utils.PtrStr(resolved.String())
					} else {
						log.Printf("Warning: Failed to resolve category URL '%s': %v\n", href, err)
					}
				}
				if catName != "" {
					propertiesList = append(propertiesList, Category{Name: catName, URL: catURLStr})
				}
			})
		} else {
			// General key-value properties
			value := strings.TrimSpace(valueElem.Text())
			// Special handling for combined file info
			if label == "file" {
				parts := strings.Split(value, ",")
				if len(parts) > 0 {
					details.FileFormat = utils.PtrStr(strings.TrimSpace(parts[0]))
				}
				if len(parts) > 1 {
					details.FileSize = utils.PtrStr(strings.TrimSpace(parts[1]))
				}
			} else if label == "ipfs" { // Special handling for IPFS
				cids := valueElem.Find("span[data-copy]")
				if cids.Length() > 0 {
					if cid, exists := cids.Eq(0).Attr("data-copy"); exists {
						details.IpfsCID = utils.PtrStr(cid)
					}
				}
				if cids.Length() > 1 {
					if cid, exists := cids.Eq(1).Attr("data-copy"); exists {
						details.IpfsCIDBlake2b = utils.PtrStr(cid)
					}
				}
			} else if value != "" { // Store other non-empty values
				propertiesMap[label] = value
			}
		}
	})
	// Assign from propertiesMap to details struct
	details.Categories = propertiesList
	details.ContentType = utils.PtrStr(propertiesMap["content_type"]) // Using normalized key
	details.Volume = utils.PtrStr(propertiesMap["volume"])
	details.Year = utils.PtrStr(propertiesMap["year"])
	details.Publisher = utils.PtrStr(propertiesMap["publisher"])
	details.Language = utils.PtrStr(propertiesMap["language"])
	details.ISBN10 = utils.PtrStr(propertiesMap["isbn_10"])
	details.ISBN13 = utils.PtrStr(propertiesMap["isbn_13"])
	details.Series = utils.PtrStr(propertiesMap["series"])
	// Assign FileFormat/FileSize again from map if they weren't found in the combined 'file' property
	if details.FileFormat == nil {
		details.FileFormat = utils.PtrStr(propertiesMap["format"]) // Check for separate 'format' key
	}
	if details.FileSize == nil {
		details.FileSize = utils.PtrStr(propertiesMap["size"]) // Check for separate 'size' key
	}

	// Cover Image (More robust selectors)
	var coverSrcAttr string
	var coverSrcExists bool

	// Try specific z-cover first
	coverZTag := doc.Find("z-cover img").First()
	coverSrcAttr, coverSrcExists = coverZTag.Attr("data-src")
	if !coverSrcExists {
		coverSrcAttr, coverSrcExists = coverZTag.Attr("src")
	}

	// Fallback to itemprop image
	if !coverSrcExists {
		coverImgProp := doc.Find("img[itemprop='image']").First()
		coverSrcAttr, coverSrcExists = coverImgProp.Attr("data-src")
		if !coverSrcExists {
			coverSrcAttr, coverSrcExists = coverImgProp.Attr("src")
		}
	}

	// Fallback to book-img container
	if !coverSrcExists {
		coverImgCont := doc.Find(".z-book-cover img, .book-img img").First()
		coverSrcAttr, coverSrcExists = coverImgCont.Attr("data-src")
		if !coverSrcExists {
			coverSrcAttr, coverSrcExists = coverImgCont.Attr("src")
		}
	}

	if coverSrcExists && coverSrcAttr != "" && !strings.Contains(coverSrcAttr, "data:image") { // Ignore inline data URIs
		// Attempt to get larger cover
		if strings.Contains(coverSrcAttr, "covers100") {
			coverSrcAttr = strings.Replace(coverSrcAttr, "covers100", "covers300", 1)
		} else if strings.Contains(coverSrcAttr, "/s/") { // Another common pattern for small covers
			coverSrcAttr = strings.Replace(coverSrcAttr, "/s/", "/m/", 1) // Try medium
		}

		resolved, err := base.Parse(coverSrcAttr)
		if err == nil {
			details.CoverURL = utils.PtrStr(resolved.String())
		} else {
			log.Printf("Warning: Failed to resolve cover URL '%s': %v\n", coverSrcAttr, err)
			// If it looks like an absolute URL already, use it
			if strings.HasPrefix(coverSrcAttr, "http") {
				details.CoverURL = utils.PtrStr(coverSrcAttr)
			}
		}
	}

	// --- Download Links ---

	// Primary Download Link (Look for common patterns)
	// Prioritize buttons with download icon or specific classes
	primaryDLLink := doc.Find("a.btn-primary[href*='/dl/'], a.btn-primary[href*='/download'], a.addDownloadedBook[href*='/dl/']").First()
	if primaryDLLink.Length() > 0 {
		if href, exists := primaryDLLink.Attr("href"); exists {
			resolved, err := base.Parse(href)
			if err == nil {
				dlURLStr := resolved.String()
				details.DownloadURL = utils.PtrStr(dlURLStr)

				// Extract format/size from the button text if not found elsewhere
				buttonText := strings.ToLower(strings.TrimSpace(primaryDLLink.Text()))
				if details.FileFormat == nil || *details.FileFormat == "" {
					// Look for format extension in text like "(epub)" or "download epub"
					reFormat := regexp.MustCompile(`\b(epub|pdf|mobi|azw3|fb2|txt|rtf|djvu|cbz|cbr)\b`)
					match := reFormat.FindString(buttonText)
					if match != "" {
						details.FileFormat = utils.PtrStr(match)
					} else {
						// Check span inside button
						fmtTag := primaryDLLink.Find("span.book-property__extension, span.download-formats-single__item").First()
						details.FileFormat = utils.PtrStr(strings.TrimSpace(fmtTag.Text()))
					}
				}
				if details.FileSize == nil || *details.FileSize == "" {
					// Look for size in text like "1.2 mb"
					reSize := regexp.MustCompile(`(\d+(\.\d+)?\s*(kb|mb|gb))`)
					match := reSize.FindStringSubmatch(buttonText)
					if len(match) > 1 {
						details.FileSize = utils.PtrStr(match[1])
					}
				}
			} else {
				log.Printf("Warning: Failed to resolve primary download URL '%s': %v\n", href, err)
			}
		}
	} else {
		log.Println("Warning: Could not find primary download button.")
	}

	// Other Formats (More robust selectors)
	details.OtherFormats = []DownloadFormat{} // Initialize empty slice

	// Look in specific container first
	doc.Find("#bookOtherFormatsContainer a.addDownloadedBook, .download-formats__items a").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			formatStr := ""
			// Try to find format text within the link
			formatTextTag := s.Find("span.book-property__extension, span.download-formats-single__item, span").First()
			formatStr = strings.TrimSpace(formatTextTag.Text())

			// If format is still empty, try getting it from the href itself (less reliable)
			if formatStr == "" {
				reFormat := regexp.MustCompile(`\.(epub|pdf|mobi|azw3|fb2|txt|rtf|djvu|cbz|cbr)$`)
				match := reFormat.FindStringSubmatch(href)
				if len(match) > 1 {
					formatStr = match[1]
				}
			}

			if formatStr != "" && strings.Contains(href, "/dl/") { // Ensure it's a direct download link
				resolved, err := base.Parse(href)
				if err == nil {
					details.OtherFormats = append(details.OtherFormats, DownloadFormat{Format: formatStr, URL: resolved.String()})
				} else {
					log.Printf("Warning: Failed to resolve other format URL '%s': %v\n", href, err)
				}
			}
		}
	})

	// Conversion Links
	doc.Find(".convert-to-list a.converterLink, .js-convert-item").Each(func(i int, s *goquery.Selection) {
		convertTo, exists := s.Attr("data-convert_to")
		if !exists {
			// Try another attribute or text if needed
			convertTo = strings.TrimSpace(s.Text()) // Less reliable
		}
		if convertTo != "" {
			// Check if it's already in OtherFormats (sometimes duplicates exist)
			found := false
			for _, f := range details.OtherFormats {
				if strings.EqualFold(f.Format, convertTo) {
					found = true
					break
				}
			}
			if !found {
				details.OtherFormats = append(details.OtherFormats, DownloadFormat{Format: convertTo, URL: "CONVERSION_NEEDED"})
			}
		}
	})

	// --- Final Checks ---
	// If primary download URL is still nil, try taking the first format if available
	if details.DownloadURL == nil && len(details.OtherFormats) > 0 {
		firstFormat := details.OtherFormats[0]
		if firstFormat.URL != "CONVERSION_NEEDED" {
			log.Printf("Warning: Primary download link not found, using first available format (%s) as primary.\n", firstFormat.Format)
			details.DownloadURL = utils.PtrStr(firstFormat.URL)
			if details.FileFormat == nil {
				details.FileFormat = utils.PtrStr(firstFormat.Format)
			}
			// Remove it from OtherFormats to avoid duplication in UI
			details.OtherFormats = details.OtherFormats[1:]
		}
	}
	// Ensure FileFormat/FileSize are set if DownloadURL is present but they are missing
	if details.DownloadURL != nil && *details.DownloadURL != "" && *details.DownloadURL != "CONVERSION_NEEDED" {
		if details.FileFormat == nil || *details.FileFormat == "" {
			// Attempt to guess format from URL extension
			parsedDlURL, _ := url.Parse(*details.DownloadURL)
			if parsedDlURL != nil {
				ext := strings.TrimPrefix(filepath.Ext(parsedDlURL.Path), ".")
				if len(ext) > 1 && len(ext) < 6 { // Basic sanity check for extension length
					details.FileFormat = utils.PtrStr(ext)
					log.Printf("Warning: File format not found, guessed '%s' from download URL.\n", ext)
				} else {
					log.Println("Warning: Could not determine file format for primary download.")
				}
			}
		}
		// FileSize is harder to guess, leave nil if not found
		if details.FileSize == nil || *details.FileSize == "" {
			log.Println("Warning: Could not determine file size for primary download.")
		}
	}

	return &details, finalURLStr, nil
}
