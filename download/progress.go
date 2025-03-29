package download

import (
	"io"
)

// DownloadProgress holds the state of a download.
type DownloadProgress struct {
	Current int64 // Bytes downloaded so far
	Total   int64 // Total expected bytes (-1 if unknown)
	Err     error // Error encountered during download
}

// ProgressWriter wraps an io.Writer, tracking bytes written and sending updates.
type ProgressWriter struct {
	writer  io.Writer               // Underlying writer (e.g., file)
	Total   int64                   // Total expected size
	Current int64                   // Bytes written so far
	ProgCh  chan<- DownloadProgress // Channel to send progress updates
}

// NewProgressWriter creates a new ProgressWriter.
func NewProgressWriter(w io.Writer, totalSize int64, progCh chan<- DownloadProgress) *ProgressWriter {
	return &ProgressWriter{
		writer: w,
		Total:  totalSize,
		ProgCh: progCh,
	}
}

// Write implements the io.Writer interface.
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.Current += int64(n)

	// Send progress update (non-blocking if channel buffer full, though unlikely here)
	if pw.ProgCh != nil {
		update := DownloadProgress{Current: pw.Current, Total: pw.Total, Err: err}
		// Use a select with a default case to avoid blocking if the receiver isn't ready,
		// though in this app's structure, the receiver should always be ready.
		select {
		case pw.ProgCh <- update:
		default:
			// Receiver not ready? This shouldn't happen in the current design.
			// Log or handle if necessary, but likely indicates a problem elsewhere.
		}

	}

	return n, err // Return the original error from the underlying writer
}
