package downloader

import "errors"

// ErrStoppedByUser is sent on errChan when the download was stopped manually.
// The monitor treats it as success (no failure notification) but does not call SetLoaded.
var ErrStoppedByUser = errors.New("download stopped by user")
