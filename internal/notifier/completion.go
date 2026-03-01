package notifier

// CompletionNotifier is called when a download finishes. The caller (API or Telegram handler)
// provides an implementation; the core completion logic does not know who started the download.
type CompletionNotifier interface {
	OnStopped(movieID uint, title string)
	OnFailed(movieID uint, title string, err error)
	OnCompleted(movieID uint, title string)
}

// CompletionNoop is a CompletionNotifier that does nothing. Use for resumed downloads (no chat context).
var CompletionNoop CompletionNotifier = completionNoop{}

type completionNoop struct{}

func (completionNoop) OnStopped(uint, string)       {}
func (completionNoop) OnFailed(uint, string, error) {}
func (completionNoop) OnCompleted(uint, string)     {}
