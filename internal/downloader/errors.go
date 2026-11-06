package downloader

import "errors"

// ErrStoppedByUser: monitor treats as success (no failure notification) but does not call SetLoaded.
var ErrStoppedByUser = errors.New("download stopped by user")

// ErrStoppedByDeletion: handler must not send "download stopped" message to the user.
var ErrStoppedByDeletion = errors.New("download stopped by deletion")

// ErrTVPreparation marks a preparation failure after media was downloaded. Keep the source and movie record for retry.
var ErrTVPreparation = errors.New("TV preparation failed; original preserved")
