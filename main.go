package main // Changed package to main for executable

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/AllenDang/giu"
	// Use the cimgui-go package directly for low-level imgui access, aliased as 'imgui'
	imgui "github.com/AllenDang/cimgui-go"

	// Adjust import paths for your project structure
	"The.Blackbook.Archive/download"
	"The.Blackbook.Archive/utils"
	zlib "The.Blackbook.Archive/zlibrary"
)

const AppName = "AnanyaVerse (giu) - By Advik"

type guiApp struct {
	// Search State
	searchQuery     string
	searchInitiated bool // To trigger search only once per button press/enter

	// Results State
	searchResults     []zlib.BookSearchResult
	searchResultErr   error
	selectedResultIdx int
	isSearching       bool

	// Details State
	selectedBook      *zlib.BookDetails
	selectedBookErr   error
	selectedBookURL   string // Which URL is being fetched
	isFetchingDetails bool
	coverTexture      *giu.Texture
	detailCoverURL    string // URL of the cover being fetched/displayed
	isFetchingCover   bool
	coverFetchErr     error
	detailScrollPosY  float32 // To reset scroll

	// Download State
	isDownloading      bool
	downloadProgress   float32 // 0.0 to 1.0
	downloadErr        error
	downloadTargetPath string
	downloadFilename   string
	showDownloadPopup  bool
	downloadPopupMsg   string

	// Image Cache
	imageCache map[string]*giu.Texture // Cache giu textures directly
	cacheMutex sync.RWMutex

	// General UI State
	statusText     string
	showErrorPopup bool
	errorPopupMsg  string
}

var gApp guiApp // Global state for the app

func main() {
	// Initialize state
	gApp = guiApp{
		searchResults:     make([]zlib.BookSearchResult, 0),
		imageCache:        make(map[string]*giu.Texture),
		selectedResultIdx: -1,
		statusText:        "Enter search query and press Search or Enter.",
	}

	// Create and run the main window
	wnd := giu.NewMasterWindow(AppName, 950, 750, 0)
	wnd.Run(loop)
}

// --- Main UI Loop ---

func loop() {
	// Handle state updates triggered by background tasks first
	handleAsyncUpdates()

	// Define the main layout (using ImGui commands via giu wrappers)
	giu.SingleWindow().Layout(
		// 1. Search Bar Row
		giu.Row(
			giu.InputText("##SearchInput", &gApp.searchQuery).Hint("Enter book title or author...").Size(giu.Auto), // Use remaining width
			giu.Button("Search").OnClick(func() { gApp.searchInitiated = true }).Disabled(gApp.isSearching),
		),
		// Check if Enter key was pressed in the input text
		giu.Condition(giu.IsItemActive() && giu.IsKeyPressed(giu.KeyEnter), func() {
			if !gApp.isSearching { // Prevent triggering search while one is ongoing
				gApp.searchInitiated = true
			}
		}, nil),

		// Spacer
		giu.Dummy(0, 5),

		// 2. Main Content Area (Split View Simulation)
		giu.SplitLayout(giu.DirectionHorizontal, 0.35, // Split ratio (35% for left)
			buildLeftPanel(),  // Results List Panel
			buildRightPanel(), // Details Panel
		),

		// 3. Status Bar Row
		giu.Dummy(0, 5),
		giu.Separator(),
		giu.Row(
			giu.Label(gApp.statusText), // Status text spans the row
		),

		// Error Popup
		buildErrorPopup(),
		// Download Completion Popup
		buildDownloadCompletePopup(),
	)

	// Trigger search if requested and not already running
	if gApp.searchInitiated && !gApp.isSearching {
		gApp.searchInitiated = false // Reset trigger
		go performSearch(gApp.searchQuery)
	}
}

// --- Panel Building Functions ---

func buildLeftPanel() giu.Widget {
	return giu.Child("LeftPanel").Layout(
		// Use a Table for the "card" layout
		giu.Table("ResultsTable").Columns(
			// Thumbnail column (fixed width)
			giu.TableColumn("Cover").WidthFixed().InnerWidthOrWeight(60), // Adjust width as needed
			// Title column (takes remaining space)
			giu.TableColumn("Title").WidthStretch(),
		).Rows(buildResultRows()...), // Spread the slice of TableRowWidgets
	)
}
func buildRightPanel() giu.Widget {
	return giu.Child("RightPanel").Layout(
		// 1. Cover Image Area
		giu.Custom(func() { // Center the image
			winWidth := giu.GetContentRegionAvail().X
			if gApp.coverTexture != nil {
				texWidth := float32(gApp.coverTexture.Width())
				// Ensure we don't indent negatively if image is wider than panel
				indent := (winWidth - texWidth) * 0.5
				if indent > 1.0 { // Only indent if there's significant space
					imgui.Indent(indent) // Use the aliased import
				}
				giu.Image(gApp.coverTexture).Size(180, 240).Build() // Use Size for fixed dimensions
				if indent > 1.0 {
					imgui.Unindent(indent) // Use the aliased import
				}
			} else if gApp.isFetchingCover {
				// Center the loading text too
				loadingText := "Loading Cover..."
				textWidth := giu.CalcTextSize(loadingText).X
				indent := (winWidth - textWidth) * 0.5
				if indent > 1.0 {
					imgui.Indent(indent)
				}
				giu.Label(loadingText).Build()
				if indent > 1.0 {
					imgui.Unindent(indent)
				}
			} else if gApp.coverFetchErr != nil {
				// Center the error text
				errorText := "Failed to load cover"
				textWidth := giu.CalcTextSize(errorText).X
				indent := (winWidth - textWidth) * 0.5
				if indent > 1.0 {
					imgui.Indent(indent)
				}
				giu.Label(errorText).Build()
				if indent > 1.0 {
					imgui.Unindent(indent)
				}
			} else if gApp.selectedBook != nil {
				// Center the "No Cover" text
				noCoverText := "No Cover Available"
				textWidth := giu.CalcTextSize(noCoverText).X
				indent := (winWidth - textWidth) * 0.5
				if indent > 1.0 {
					imgui.Indent(indent)
				}
				giu.Label(noCoverText).Build()
				if indent > 1.0 {
					imgui.Unindent(indent)
				}
			} else {
				// Center the placeholder if needed, or just let it be
				giu.Dummy(180, 240).Build() // Placeholder space if nothing selected
			}
		}),

		// Spacer
		giu.Dummy(0, 10),

		// 2. Details Area (Scrollable Child)
		giu.Child("DetailsScroll").Border(false).Scrollable(true).AutoExpandWidth(true).Layout(
			buildDetailsContent(), // Function to build the actual details widgets
		),
		// Set scroll position when details change (needs to be done carefully in ImGui)
		giu.Condition(gApp.selectedBook != nil && giu.IsWindowAppearing(), func() {
			// Attempt to reset scroll - might need refinement
			//giu.Set(0) // Reset scroll when details appear/change
		}, nil),

		// 3. Download Area
		giu.Separator(),
		giu.ProgressBar(gApp.downloadProgress).Overlay(fmt.Sprintf("%.0f%%", gApp.downloadProgress*100)).Size(giu.Auto, 20).Visible(gApp.isDownloading),
		giu.Button(buildDownloadButtonText()).OnClick(performDownload).Disabled(!canDownload()),
	)
}

// --- Widget Building Helpers ---

func buildResultRows() []*giu.TableRowWidget {
	rows := make([]*giu.TableRowWidget, len(gApp.searchResults))
	gApp.cacheMutex.RLock() // Lock for reading search results
	results := gApp.searchResults
	selIdx := gApp.selectedResultIdx
	gApp.cacheMutex.RUnlock()

	for i, result := range results {
		localI := i // Capture loop variable for closures
		localResult := result

		// Create widgets for the row
		imgWidget := buildThumbnailImage(localResult.SmallCoverURL, localI)
		titleSelectable := giu.Selectable(localResult.Title).Selected(localI == selIdx).OnClick(func() {
			if gApp.selectedResultIdx != localI {
				gApp.selectedResultIdx = localI
				// Trigger detail fetch
				go fetchAndDisplayDetails(localResult.URL)
			}
		})

		// Build the table row
		rows[i] = giu.TableRow(
			imgWidget,
			titleSelectable,
		)
	}
	return rows
}

func buildThumbnailImage(url string, itemID int) giu.Widget {
	if url == "" {
		return giu.Dummy(40, 60) // Placeholder if no URL
	}

	gApp.cacheMutex.RLock()
	tex, found := gApp.imageCache[url]
	gApp.cacheMutex.RUnlock()

	if found {
		return giu.Image(tex).Size(40, 60)
	}

	// Not found, trigger fetch if not already fetching for this URL (needs tracking)
	// For simplicity, we trigger fetch every time it's not found, caching prevents re-download
	go fetchAndCacheTexture(url, true) // true indicates it's a thumbnail

	return giu.Label("...") // Simple loading indicator
}

func buildDetailsContent() giu.Widget {
	if gApp.isFetchingDetails {
		return giu.Label("Fetching details...")
	}
	if gApp.selectedBookErr != nil {
		return giu.Label("Error loading details: " + gApp.selectedBookErr.Error())
	}
	if gApp.selectedBook == nil {
		return giu.Label("Select a book to see details.")
	}

	// Use a table for layout (Label | Value)
	details := gApp.selectedBook
	return giu.Table("DetailsTable").Columns(
		giu.TableColumn("Label").WidthFixed(),
		giu.TableColumn("Value").WidthStretch(),
	).Rows(
		// Add rows using a helper
		detailRow("Title", details.Title),
		detailRow("Author", details.Author),
		detailURLRow("Author Link", details.AuthorURL),
		detailRow("Description", details.Description), // Requires wrapping
		detailRow("Rating", combineRatings(details)),
		detailCategories("Categories", details.Categories),
		detailRow("File", combineFileInfo(details)),
		detailRow("Language", details.Language),
		detailRow("Year", details.Year),
		detailRow("Publisher", details.Publisher),
		detailRow("Series", details.Series),
		detailRow("Volume", details.Volume),
		detailRow("ISBN 10", details.ISBN10),
		detailRow("ISBN 13", details.ISBN13),
		detailRow("Content Type", details.ContentType),
		detailRow("Book ID", details.BookID),
		detailRow("IPFS CID", details.IpfsCID),
		detailURLRow("Cover URL", details.CoverURL),
		detailURLRow("Primary DL URL", details.DownloadURL),
		detailOtherFormats("Other Formats", details.OtherFormats),
	)

}

// --- Detail Row Helpers (Simplified) ---

func detailRow(label string, value *string) *giu.TableRowWidget {
	valStr := ""
	if value != nil {
		valStr = *value
	}
	// Basic wrapping attempt - ImGui handles text wrapping within columns
	return giu.TableRow(giu.Label(label+":").Align(giu.AlignRight), giu.Label(valStr).Wrapped(true))
}

func detailURLRow(label string, value *string) *giu.TableRowWidget {
	valStr := ""
	urlPtr := (*url.URL)(nil)
	if value != nil && *value != "" {
		valStr = *value
		parsed, err := url.Parse(*value)
		if err == nil {
			urlPtr = parsed
		}
	}

	displayStr := valStr
	if len(displayStr) > 60 { // Shorten displayed URL
		displayStr = displayStr[:57] + "..."
	}

	// Make URL clickable (basic - just shows tooltip, opening needs OS call)
	widget := giu.Selectable(displayStr).OnClick(func() {
		if urlPtr != nil {
			log.Printf("Info: Would open URL %s", urlPtr.String())
			// In a real app: utils.OpenURL(urlPtr.String()) // Need an OS-specific open function
		}
	}).Tooltip(valStr) // Show full URL on hover

	return giu.TableRow(giu.Label(label+":").Align(giu.AlignRight), widget)
}

func detailCategories(label string, cats []zlib.BookCategory) *giu.TableRowWidget {
	if len(cats) == 0 {
		return nil
	} // Return nil to skip row

	widgets := make([]giu.Widget, 0, len(cats))
	for _, cat := range cats {
		if cat.Name == "" {
			continue
		}
		if cat.URL != nil && *cat.URL != "" {
			parsed, err := url.Parse(*cat.URL)
			if err == nil {
				catURL := *cat.URL // Local copy for closure
				widgets = append(widgets, giu.Selectable(cat.Name).OnClick(func() {
					log.Printf("Info: Would open Category URL %s", catURL)
				}).Tooltip(catURL))
			} else {
				widgets = append(widgets, giu.Label(cat.Name))
			}
		} else {
			widgets = append(widgets, giu.Label(cat.Name))
		}
	}
	return giu.TableRow(giu.Label(label+":").Align(giu.AlignRight), giu.Column(widgets...))
}

func detailOtherFormats(label string, formats []zlib.FormatInfo) *giu.TableRowWidget {
	if len(formats) == 0 {
		return nil
	}
	widgets := make([]giu.Widget, 0, len(formats))
	for _, f := range formats {
		text := f.Format
		if f.URL == "CONVERSION_NEEDED" {
			text += " (Conversion)"
		}
		widgets = append(widgets, giu.Label(text))
	}
	return giu.TableRow(giu.Label(label+":").Align(giu.AlignRight), giu.Column(widgets...))
}

// Combine helper functions (similar to Fyne version)
func combineRatings(details *zlib.BookDetails) *string { /* ... implementation ... */
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
	if ratingStr == "" {
		return nil
	}
	return &ratingStr
}
func combineFileInfo(details *zlib.BookDetails) *string { /* ... implementation ... */
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
	if fileInfo == "" {
		return nil
	}
	return &fileInfo
}

// --- Download Button Logic ---

func buildDownloadButtonText() string {
	if !canDownload() || gApp.selectedBook == nil {
		return "Download" // Default disabled text
	}
	details := gApp.selectedBook
	buttonText := "Download"
	if details.FileFormat != nil && *details.FileFormat != "" {
		buttonText += fmt.Sprintf(" (%s", *details.FileFormat)
		if details.FileSize != nil && *details.FileSize != "" {
			buttonText += fmt.Sprintf(", %s", *details.FileSize)
		}
		buttonText += ")"
	}
	return buttonText
}

func canDownload() bool {
	return !gApp.isDownloading &&
		gApp.selectedBook != nil &&
		gApp.selectedBook.DownloadURL != nil &&
		*gApp.selectedBook.DownloadURL != "" &&
		*gApp.selectedBook.DownloadURL != "CONVERSION_NEEDED"
}

// --- Popup Builders ---

func buildErrorPopup() giu.Widget {
	return giu.PopupModal("Error##ErrorPopup").IsOpen(&gApp.showErrorPopup).Flags(giu.WindowFlagsAlwaysAutoResize).Layout(
		giu.Label(gApp.errorPopupMsg),
		giu.Button("OK").OnClick(func() { gApp.showErrorPopup = false }),
	)
}

func buildDownloadCompletePopup() giu.Widget {
	return giu.PopupModal("Download Complete##DownloadPopup").IsOpen(&gApp.showDownloadPopup).Flags(giu.WindowFlagsAlwaysAutoResize).Layout(
		giu.Label(gApp.downloadPopupMsg),
		giu.Button("OK").OnClick(func() { gApp.showDownloadPopup = false }),
	)
}

// --- Asynchronous Task Handling ---

// Channel for signaling UI updates from goroutines
var updateSignal = make(chan bool, 1) // Buffered channel to prevent blocking

// Call this from goroutines when state changes require a UI redraw
func signalUpdate() {
	select {
	case updateSignal <- true: // Try to send signal
	default: // If channel is full, an update is already pending
	}
}

// Check for signals and trigger giu.Update
func handleAsyncUpdates() {
	select {
	case <-updateSignal:
		giu.Update() // Tell giu to redraw the UI
	default:
		// No update needed
	}
}

// --- Core Logic Functions (Adapted for giu state and async) ---

func performSearch(query string) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		gApp.statusText = "Please enter a search query."
		signalUpdate()
		return
	}

	// Update state BEFORE starting goroutine
	gApp.isSearching = true
	gApp.statusText = fmt.Sprintf("Searching for '%s'...", trimmedQuery)
	gApp.searchResults = nil // Clear results
	gApp.searchResultErr = nil
	gApp.selectedResultIdx = -1
	gApp.selectedBook = nil // Clear details when starting new search
	gApp.coverTexture = nil
	gApp.selectedBookErr = nil
	gApp.coverFetchErr = nil
	signalUpdate()

	// Goroutine for actual search
	results, finalURL, err := zlib.SearchZLibrary(trimmedQuery) // Assuming this is synchronous internally

	// Update state AFTER goroutine finishes
	gApp.searchResults = results
	gApp.searchResultErr = err
	gApp.isSearching = false
	if err != nil {
		gApp.statusText = fmt.Sprintf("Search failed (URL: %s)", finalURL)
		gApp.errorPopupMsg = fmt.Sprintf("Search Error (URL: %s):\n%v", finalURL, err)
		gApp.showErrorPopup = true
		log.Printf("Search Error (URL: %s): %v\n", finalURL, err)
	} else {
		gApp.statusText = fmt.Sprintf("Found %d results for '%s'. Select one.", len(results), trimmedQuery)
		if len(results) == 0 {
			gApp.statusText = fmt.Sprintf("No results found for '%s'.", trimmedQuery)
		}
	}
	signalUpdate() // Signal UI update
}

func fetchAndDisplayDetails(bookURL string) {
	// Update state BEFORE starting
	gApp.isFetchingDetails = true
	gApp.selectedBook = nil // Clear previous book
	gApp.selectedBookErr = nil
	gApp.coverTexture = nil // Clear previous cover
	gApp.coverFetchErr = nil
	gApp.isFetchingCover = false // Reset cover fetch status
	gApp.selectedBookURL = bookURL
	gApp.statusText = fmt.Sprintf("Fetching details for %s...", bookURL)
	signalUpdate()

	// Goroutine for fetching details
	details, finalURL, err := zlib.GetBookDetails(bookURL)

	// Update state AFTER fetching
	gApp.isFetchingDetails = false
	if err != nil {
		gApp.selectedBookErr = fmt.Errorf("failed to get details from %s: %w", finalURL, err)
		gApp.statusText = fmt.Sprintf("Error fetching details (URL: %s).", finalURL)
		gApp.errorPopupMsg = fmt.Sprintf("Detail Fetch Error (URL: %s):\n%v", finalURL, gApp.selectedBookErr)
		gApp.showErrorPopup = true
		log.Printf("Detail Fetch Error (URL: %s): %v\n", finalURL, err)
	} else {
		gApp.selectedBook = details
		gApp.statusText = "Details loaded."
		// Trigger cover fetch if URL exists
		if details.CoverURL != nil && *details.CoverURL != "" {
			go fetchAndCacheTexture(*details.CoverURL, false) // false = not thumbnail
		}
	}
	signalUpdate()
}

func fetchAndCacheTexture(imageURL string, isThumbnail bool) {
	// Check cache again within the goroutine (might have been fetched by another request)
	gApp.cacheMutex.RLock()
	_, found := gApp.imageCache[imageURL]
	gApp.cacheMutex.RUnlock()
	if found {
		if !isThumbnail && gApp.selectedBook != nil && gApp.selectedBook.CoverURL != nil && *gApp.selectedBook.CoverURL == imageURL {
			// Ensure the main cover texture is updated if this was the intended one
			gApp.cacheMutex.RLock()
			gApp.coverTexture = gApp.imageCache[imageURL]
			gApp.cacheMutex.RUnlock()
			signalUpdate()
		}
		return // Already cached
	}

	// Update cover fetching status if it's the main cover
	isMainCover := !isThumbnail && gApp.selectedBook != nil && gApp.selectedBook.CoverURL != nil && *gApp.selectedBook.CoverURL == imageURL
	if isMainCover {
		gApp.isFetchingCover = true
		gApp.coverFetchErr = nil
		gApp.detailCoverURL = imageURL // Track which cover is being fetched
		signalUpdate()
	}

	log.Printf("Fetching image texture: %s", imageURL)
	resp, err := zlib.MakeRequest(imageURL)
	var fetchErr error
	var tex *giu.Texture

	if err != nil {
		fetchErr = fmt.Errorf("image request failed for %s: %w", imageURL, err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			fetchErr = fmt.Errorf("failed to fetch image %s, status: %s", imageURL, resp.Status)
		} else {
			imgBytes, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				fetchErr = fmt.Errorf("failed to read image data from %s: %w", imageURL, readErr)
			} else if len(imgBytes) == 0 {
				fetchErr = fmt.Errorf("downloaded image data is empty for %s", imageURL)
			} else {
				// Decode image bytes
				imgData, _, decodeErr := image.Decode(bytes.NewReader(imgBytes))
				if decodeErr != nil {
					fetchErr = fmt.Errorf("failed to decode image %s: %w", imageURL, decodeErr)
				} else {
					// Create giu texture
					var creationErr error
					tex, creationErr = giu.NewTextureFromRgba(imgData.(*image.RGBA)) // Assumes RGBA, might need conversion
					if creationErr != nil {
						fetchErr = fmt.Errorf("failed to create texture for %s: %w", imageURL, creationErr)
						tex = nil // Ensure tex is nil on error
					}
				}
			}
		}
	}

	// Update state after fetching/decoding/texture creation
	if isMainCover {
		gApp.isFetchingCover = false
		gApp.coverFetchErr = fetchErr
	}

	if fetchErr != nil {
		log.Printf("Failed to get/process image texture %s: %v", imageURL, fetchErr)
		// Don't cache errors
	} else if tex != nil {
		// Cache the texture
		gApp.cacheMutex.Lock()
		gApp.imageCache[imageURL] = tex
		gApp.cacheMutex.Unlock()
		log.Printf("Successfully cached texture for %s", imageURL)

		// If this was the main cover for the currently selected book, update gApp.coverTexture
		if isMainCover {
			gApp.coverTexture = tex
		}
	}
	signalUpdate() // Signal UI update needed (either for error or new texture)
}

func performDownload() {
	if !canDownload() {
		gApp.errorPopupMsg = "Download conditions not met (no book selected, URL missing, or conversion needed)."
		gApp.showErrorPopup = true
		signalUpdate()
		return
	}
	details := gApp.selectedBook // Use the currently selected book

	// Prepare filename and path (same logic as Fyne version)
	if err := os.MkdirAll(utils.DownloadDir, 0755); err != nil {
		gApp.errorPopupMsg = fmt.Sprintf("Failed to create download directory '%s': %v", utils.DownloadDir, err)
		gApp.showErrorPopup = true
		signalUpdate()
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
	ext := "bin"
	if details.FileFormat != nil && *details.FileFormat != "" {
		ext = strings.ToLower(*details.FileFormat)
	}
	baseFilename := fmt.Sprintf("%s - %s", author, title)
	sanitizedBase := utils.SanitizeFilename(baseFilename)
	filename := fmt.Sprintf("%s.%s", sanitizedBase, ext)
	filePath := filepath.Join(utils.DownloadDir, filename)

	gApp.downloadTargetPath = filePath
	gApp.downloadFilename = filename

	// Check if file exists - NOTE: ImGui doesn't have a direct equivalent to Fyne's ShowConfirm.
	// We'll handle this by either always overwriting or providing feedback differently.
	// For simplicity here, we'll just proceed. A real app might add a checkbox or config setting.
	if _, err := os.Stat(filePath); err == nil {
		log.Printf("File %s exists, overwriting.", filename)
		// Alternative: Show a status message and don't proceed without confirmation?
		// gApp.statusText = fmt.Sprintf("File '%s' exists. Overwrite not implemented.", filename)
		// signalUpdate()
		// return
	} else if !os.IsNotExist(err) {
		gApp.errorPopupMsg = fmt.Sprintf("Could not check for existing file '%s': %v", filePath, err)
		gApp.showErrorPopup = true
		signalUpdate()
		return
	}

	// Start the actual download
	go startActualDownload(details, filePath, filename)
}

func startActualDownload(details *zlib.BookDetails, filePath, filename string) {
	// Update state before starting
	gApp.isDownloading = true
	gApp.downloadProgress = 0.0
	gApp.downloadErr = nil
	gApp.statusText = fmt.Sprintf("Starting download: %s", filename)
	signalUpdate()

	// Progress channel
	progCh := make(chan download.DownloadProgress, 10) // Increased buffer

	// Download Goroutine (similar to Fyne)
	go func() {
		defer close(progCh)
		downloadURL := *details.DownloadURL
		log.Printf("Attempting download from: %s", downloadURL)

		resp, err := zlib.MakeRequest(downloadURL)
		if err != nil {
			progCh <- download.DownloadProgress{Err: fmt.Errorf("request failed for %s: %w", downloadURL, err)}
			return
		}
		defer resp.Body.Close()

		finalURL := downloadURL
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(strings.ToLower(contentType), "text/html") {
			// ... (handle HTML error as in Fyne version) ...
			errMsg := fmt.Sprintf("download failed: Received HTML (URL: %s). Login/Captcha/Limit likely.", finalURL)
			progCh <- download.DownloadProgress{Err: fmt.Errorf(errMsg)}
			return
		}
		if resp.StatusCode != 200 {
			// ... (handle status code error as in Fyne version) ...
			errMsg := fmt.Sprintf("download failed: Status %s (URL: %s)", resp.Status, finalURL)
			progCh <- download.DownloadProgress{Err: fmt.Errorf(errMsg)}
			return
		}

		totalSize, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if totalSize <= 0 {
			totalSize = -1
		}
		progCh <- download.DownloadProgress{Total: totalSize, Current: 0}

		outFile, err := os.Create(filePath)
		if err != nil {
			progCh <- download.DownloadProgress{Err: fmt.Errorf("failed to create file '%s': %w", filePath, err)}
			return
		}
		defer func() {
			if cerr := outFile.Close(); cerr != nil {
				log.Printf("Warning: failed to close output file '%s': %v", filePath, cerr)
			}
		}()

		writer := download.NewProgressWriter(outFile, totalSize, progCh)
		writtenBytes, copyErr := io.Copy(writer, resp.Body)

		if copyErr != nil {
			// ... (handle copy error, remove partial file as in Fyne version) ...
			if remErr := os.Remove(filePath); remErr != nil {
				log.Printf("Warning: failed to remove partial file '%s': %v", filePath, remErr)
			}
			progCh <- download.DownloadProgress{Current: writtenBytes, Total: totalSize, Err: fmt.Errorf("download interrupted: %w", copyErr)}
		} else {
			log.Printf("Download successful, wrote %d bytes to %s", writtenBytes, filePath)
			progCh <- download.DownloadProgress{Current: writtenBytes, Total: totalSize, Err: nil}
		}
	}()

	// Progress Handling (reading from channel)
	var lastError error
	for update := range progCh {
		lastError = update.Err
		if update.Err != nil {
			gApp.downloadErr = update.Err
			break // Stop processing progress on error
		}

		if update.Total > 0 {
			gApp.downloadProgress = float32(update.Current) / float32(update.Total)
			gApp.statusText = fmt.Sprintf("Downloading %s... %s / %s", filename, utils.FormatBytes(update.Current), utils.FormatBytes(update.Total))
		} else {
			gApp.downloadProgress = 0 // Indeterminate
			gApp.statusText = fmt.Sprintf("Downloading %s... %s / ???", filename, utils.FormatBytes(update.Current))
		}
		signalUpdate() // Update UI frequently during download
	}

	// Final state update after download finishes or errors out
	gApp.isDownloading = false
	gApp.downloadProgress = 0.0 // Reset progress bar display value

	if lastError != nil {
		gApp.statusText = fmt.Sprintf("Download failed for %s.", filename)
		gApp.errorPopupMsg = fmt.Sprintf("Download failed for %s:\n%v", filename, lastError)
		gApp.showErrorPopup = true
	} else {
		gApp.statusText = fmt.Sprintf("Download complete: %s", filePath)
		gApp.downloadPopupMsg = fmt.Sprintf("Download Complete!\nSaved to:\n%s", filePath)
		gApp.showDownloadPopup = true
	}
	signalUpdate() // Final UI update
}
