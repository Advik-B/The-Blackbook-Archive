package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	// "fyne.io/fyne/v2/dialog" // Moved to handlers
	// "fyne.io/fyne/v2/layout" // Moved to helpers
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	// Adjust import path
	zlib "The.Blackbook.Archive/zlibrary"
	"sync" // Import sync package for Mutex
)

// AppName is the name of the application.
const AppName = "AnanyaVerse - By Advik"

// guiApp holds the state and UI elements of the application.
type guiApp struct {
	fyneApp fyne.App
	mainWin fyne.Window

	// UI Widgets
	searchEntry      *widget.Entry
	searchButton     *widget.Button
	resultsList      *widget.List
	statusBar        *widget.Label
	detailsArea      *fyne.Container // Container for detail widgets
	downloadButton   *widget.Button
	progressBar      *widget.ProgressBar
	coverImage       *canvas.Image
	detailsContainer *container.Scroll // Scrollable container for detailsArea

	// Data
	searchResults []zlib.BookSearchResult // Use type from zlib package
	selectedBook  *zlib.BookDetails       // Use type from zlib package
	imageCache    map[string]fyne.Resource
	cacheMutex    sync.RWMutex // Mutex to protect imageCache
}

// NewGuiApp creates and initializes a new GUI application instance.
func NewGuiApp() *guiApp {
	a := app.New()
	// a.Settings().SetTheme(theme.DarkTheme()) // Optional: Set dark theme
	w := a.NewWindow(AppName)

	g := &guiApp{
		fyneApp:    a,
		mainWin:    w,
		imageCache: make(map[string]fyne.Resource),
		// Initialize other fields if necessary
	}

	g.setupUI() // Setup the UI elements

	w.Resize(fyne.NewSize(900, 700)) // Slightly larger default size
	w.SetMaster()                    // Declare this as the main window

	return g
}

// setupUI creates and arranges the widgets in the main window.
func (g *guiApp) setupUI() {
	// --- Search Bar ---
	g.searchEntry = widget.NewEntry()
	g.searchEntry.SetPlaceHolder("Enter book title or author...")
	g.searchEntry.OnSubmitted = g.performSearch // Use handler function
	g.searchButton = widget.NewButtonWithIcon("Search", theme.SearchIcon(), func() {
		g.performSearch(g.searchEntry.Text) // Use handler function
	})
	// Use container.NewBorder for better layout control
	searchBox := container.NewBorder(nil, nil, nil, g.searchButton, g.searchEntry)

	// --- Status Bar ---
	g.statusBar = widget.NewLabel("Enter a search query and press Search.")
	g.statusBar.Wrapping = fyne.TextTruncate // Prevent long status from wrapping badly

	// --- Results List ---
	g.resultsList = widget.NewList(
		func() int {
			return len(g.searchResults) // Get length from app state
		},
		func() fyne.CanvasObject {
			// Use a more descriptive template if needed, maybe include subtitle space
			return widget.NewLabel("Template Book Title")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			// Safely access searchResults
			if id < len(g.searchResults) {
				item.(*widget.Label).SetText(g.searchResults[id].Title)
			}
		},
	)
	g.resultsList.OnSelected = func(id widget.ListItemID) {
		// Safely access searchResults
		if id < len(g.searchResults) {
			g.fetchAndDisplayDetails(g.searchResults[id].URL) // Use handler
		}
	}
	// Add placeholder when list is empty?
	// resultsPlaceholder := widget.NewLabel("No search results")
	// resultsContainer := container.NewMax(g.resultsList, resultsPlaceholder) // Manage visibility

	// --- Details Area ---
	g.coverImage = canvas.NewImageFromResource(nil) // Start empty
	g.coverImage.FillMode = canvas.ImageFillContain
	g.coverImage.SetMinSize(fyne.NewSize(150, 200)) // Minimum size for the cover

	// Container to hold the actual detail widgets (like the Form)
	g.detailsArea = container.NewVBox(widget.NewLabel("Select a book from the results to see details."))
	// Make the detailsArea scrollable
	g.detailsContainer = container.NewScroll(g.detailsArea)

	// Left pane: Cover image centered
	detailsLeftPane := container.NewCenter(g.coverImage)
	// Right pane: Scrollable details
	detailsRightPane := g.detailsContainer

	// Split container for cover and details
	detailsSplit := container.NewHSplit(detailsLeftPane, detailsRightPane)
	detailsSplit.SetOffset(0.25) // Adjust initial split ratio (25% for cover)

	// --- Download Section ---
	g.downloadButton = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), g.performDownload) // Use handler
	g.downloadButton.Disable()                                                                       // Start disabled

	g.progressBar = widget.NewProgressBar()
	g.progressBar.Hide() // Start hidden

	downloadArea := container.NewVBox(g.downloadButton, g.progressBar)

	// --- Main Layout ---
	// Left side: Results list
	leftPanel := container.NewBorder(nil, nil, nil, nil, g.resultsList)

	// Right side: Details split view (cover/info) on top, download area below
	rightPanel := container.NewBorder(nil, downloadArea, nil, nil, detailsSplit)

	// Main horizontal split
	mainSplit := container.NewHSplit(leftPanel, rightPanel)
	mainSplit.SetOffset(0.35) // Adjust initial split (35% for results list)

	// Overall window content: Search bar top, status bar bottom, main split in center
	content := container.NewBorder(searchBox, g.statusBar, nil, nil, mainSplit)

	g.mainWin.SetContent(content)
}

// Run starts the Fyne application event loop.
func (g *guiApp) Run() {
	g.mainWin.Show() // Show the window first
	g.fyneApp.Run()  // Then run the application loop
}
