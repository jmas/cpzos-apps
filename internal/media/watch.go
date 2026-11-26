package media

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Watch reports changes to the automount state directory. The channel is
// buffered by one; bursts of udev state files coalesce into one redraw.
func Watch() (<-chan struct{}, func(), error) {
	dir := StateDir()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err := w.Add(dir); err != nil {
		// /run/cpz-automount is created by tmpfiles at boot, but an older
		// image may create it only on the first mount. Watch the parent and
		// add the directory when it appears.
		if err := w.Add(filepath.Dir(dir)); err != nil {
			w.Close()
			return nil, nil, err
		}
	}

	events := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Name == dir {
					_ = w.Add(dir)
				}
				if strings.HasSuffix(ev.Name, ".mountpoint") {
					select {
					case events <- struct{}{}:
					default:
					}
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	closeFn := func() {
		select {
		case <-done:
		default:
			close(done)
		}
		w.Close()
	}
	return events, closeFn, nil
}

// WatchSupported is true when the state directory can be watched. It exists so
// the UI can skip the watcher on an image without the automount tmpfiles.
func WatchSupported() bool {
	st, err := os.Stat(StateDir())
	return err == nil && st.IsDir()
}
