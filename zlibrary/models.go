package zlibrary

// --- Data Structures ---

// BookSearchResult represents a single book found in search results.
type BookSearchResult struct {
	Title  string
	URL    string
	BookID *string // Made pointer consistent with BookDetails
}

// Category represents a book category.
type Category struct {
	Name string
	URL  *string
}

// DownloadFormat represents an available download format and its URL.
type DownloadFormat struct {
	Format string
	URL    string
}

// BookDetails holds detailed information about a specific book.
type BookDetails struct {
	URL            string
	BookID         *string
	Title          *string
	Author         *string
	AuthorURL      *string
	Description    *string
	RatingInterest *string
	RatingQuality  *string
	Categories     []Category
	ContentType    *string
	Volume         *string
	Year           *string
	Publisher      *string
	Language       *string
	ISBN10         *string `json:"isbn_10"`
	ISBN13         *string `json:"isbn_13"`
	Series         *string
	FileFormat     *string
	FileSize       *string
	IpfsCID        *string `json:"ipfs_cid"`
	IpfsCIDBlake2b *string `json:"ipfs_cid_blake2b"`
	CoverURL       *string
	DownloadURL    *string // Primary download link
	OtherFormats   []DownloadFormat
}
