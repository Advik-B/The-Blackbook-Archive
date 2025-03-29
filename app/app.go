package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"sync" // Import sync package for Mutex

	zlib "The.Blackbook.Archive/zlibrary" // Adjust import path
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
	detailsArea      *fyne.Container   // Container for detail widgets (content of the scroll)
	detailsContainer *container.Scroll // Scrollable container for detailsArea
	downloadButton   *widget.Button
	progressBar      *widget.ProgressBar
	coverImage       *canvas.Image

	// Data
	searchResults []zlib.BookSearchResult
	selectedBook  *zlib.BookDetails
	imageCache    map[string]fyne.Resource
	cacheMutex    sync.RWMutex
}

// NewGuiApp creates and initializes a new GUI application instance.
func NewGuiApp() *guiApp {
	a := app.New()
	a.Settings().SetTheme(theme.DarkTheme()) // Activate Dark Theme for improved look
	w := a.NewWindow(AppName)

	g := &guiApp{
		fyneApp:    a,
		mainWin:    w,
		imageCache: make(map[string]fyne.Resource),
	}

	g.setupUI()

	w.Resize(fyne.NewSize(950, 750)) // Slightly larger default size again
	w.SetMaster()

	return g
}

// setupUI creates and arranges the widgets in the main window.
func (g *guiApp) setupUI() {
	// --- Search Bar ---
	g.searchEntry = widget.NewEntry()
	g.searchEntry.SetPlaceHolder("Enter book title or author...")
	g.searchEntry.OnSubmitted = g.performSearch
	g.searchButton = widget.NewButtonWithIcon("Search", theme.SearchIcon(), func() {
		g.performSearch(g.searchEntry.Text)
	})
	searchBox := container.NewBorder(nil, nil, nil, g.searchButton, g.searchEntry)

	// --- Status Bar ---
	g.statusBar = widget.NewLabel("Enter a search query and press Search.")
	g.statusBar.Wrapping = fyne.TextTruncate

	// --- Results List ---
	g.resultsList = widget.NewList(
		func() int {
			return len(g.searchResults)
		},
		func() fyne.CanvasObject {
			// Using a slightly richer template label for better spacing potential
			return container.NewPadded(widget.NewLabel("Template Book Title"))
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id < len(g.searchResults) {
				// Access the label within the padded container
				item.(*fyne.Container).Objects[0].(*widget.Label).SetText(g.searchResults[id].Title)
			}
		},
	)
	g.resultsList.OnSelected = func(id widget.ListItemID) {
		if id < len(g.searchResults) {
			g.fetchAndDisplayDetails(g.searchResults[id].URL)
		}
	}

	// --- Details Area Structure (Right Panel Content) ---
	g.coverImage = canvas.NewImageFromResource(nil) // Start empty
	g.coverImage.FillMode = canvas.ImageFillContain
	g.coverImage.SetMinSize(fyne.NewSize(180, 240)) // Slightly larger min size for cover

	// Container to hold the actual detail widgets (e.g., the Form)
	g.detailsArea = container.NewVBox(widget.NewLabel("Select a book from the results to see details."))
	// Make the detailsArea scrollable
	g.detailsContainer = container.NewScroll(g.detailsArea)
	// Optional: Set a minimum height for the scroll container if needed
	// g.detailsContainer.SetMinSize(fyne.NewSize(0, 300))

	// --- Download Section ---
	g.downloadButton = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), g.performDownload)
	g.downloadButton.Disable()
	g.progressBar = widget.NewProgressBar()
	g.progressBar.Hide()
	downloadArea := container.NewVBox(g.downloadButton, g.progressBar)

	// --- Main Layout ---
	// Left side: Results list (Padded for better spacing)
	leftPanel := container.NewPadded(g.resultsList)

	// Right side: Cover image top, details center (scrollable), download bottom
	rightPanel := container.NewBorder(
		container.NewCenter(g.coverImage), // Center the cover image at the top
		downloadArea,                      // Download area at the bottom
		nil,                               // No left element
		nil,                               // No right element
		g.detailsContainer,                // Scrollable details take the central space
	)

	// Main horizontal split
	mainSplit := container.NewHSplit(leftPanel, rightPanel)
	mainSplit.SetOffset(0.35) // Adjust initial split (35% for results list)

	// Overall window content: Search bar top, status bar bottom, main split in center
	// Add padding around the main content area for better spacing from window edges
	content := container.NewPadded(container.NewBorder(searchBox, g.statusBar, nil, nil, mainSplit))

	g.mainWin.SetContent(content)
}

// Run starts the Fyne application event loop.
func (g *guiApp) Run() {
	g.mainWin.Show()
	g.fyneApp.Run()
}
