package db

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"time"
)

var sessionDeleteRetryDelays = [...]time.Duration{
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	200 * time.Millisecond,
}

func removeSessionDirWithRetry(ctx context.Context, path string, removeAll func(string) error) error {
	for attempt := 0; ; attempt++ {
		err := removeAll(path)
		if err == nil || !isWindowsSharingViolation(err) || attempt == len(sessionDeleteRetryDelays) {
			return err
		}

		timer := time.NewTimer(sessionDeleteRetryDelays[attempt])
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isWindowsSharingViolation(err error) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
