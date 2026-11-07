package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// exitRestart is deckmode's exit status after a watched file changed. It is
// not 0, so launchd starts it again, with the new files.
const exitRestart = 3

// watchForRestart looks at the files every interval and returns true once
// they have changed and then stayed the same for one more look, so a file
// still being copied (over File Sharing, say) is not read half done. A folder
// changes when a file in it is added, removed or replaced. If check fails for
// the files as they now are, that is logged and the watch goes on: a typo in
// the config must not take the panel down. It returns false when ctx ends.
func watchForRestart(ctx context.Context, interval time.Duration, files []string, check func() error) bool {
	base := signature(files)
	prev := base
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
		}
		now := signature(files)
		settled := now == prev
		prev = now
		if now == base || !settled {
			continue
		}
		if err := check(); err != nil {
			slog.Error("the changed config can't be used, so the running one stays", "err", err)
			base = now
			continue
		}
		return true
	}
}

// signature changes when any of the files is changed, created or removed.
func signature(files []string) string {
	var b strings.Builder
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", f, fi.Size(), fi.ModTime().UnixNano())
		} else {
			fmt.Fprintf(&b, "%s missing\n", f)
		}
	}
	return b.String()
}
