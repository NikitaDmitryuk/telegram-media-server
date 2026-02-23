package notifier

// CompletionNotifier is called when a download finishes. The caller (API or Telegram handler)
// provides an implementation; the core completion logic does not know who started the download.
type CompletionNotifier interface {
	OnStopped(movieID uint, title string)
	OnFailed(movieID uint, title string, err error)
	OnCompleted(movieID uint, title string)
}
