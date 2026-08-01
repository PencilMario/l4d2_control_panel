package releases

import "context"

type DownloadRequest struct {
	URL         string
	Destination string
	Filename    string
	Total       int64
	MaxBytes    int64
}

type Downloader interface {
	Download(context.Context, DownloadRequest) (int64, error)
}
