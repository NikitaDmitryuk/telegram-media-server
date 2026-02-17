package downloader

import "errors"

// ErrStoppedByUser: monitor treats as success (no failure notification) but does not call SetLoaded.
var ErrStoppedByUser = errors.New("download stopped by user")

// ErrStoppedByDeletion: handler must not send "download stopped" message to the user.
var ErrStoppedByDeletion = errors.New("download stopped by deletion")
