package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const tick = 10 * time.Millisecond

// startWatch runs watchForRestart in the background and returns its answer.
func startWatch(t *testing.T, files []string, check func() error) (<-chan bool, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan bool, 1)
	go func() { done <- watchForRestart(ctx, tick, files, check) }()
	time.Sleep(3 * tick) // lets it take its first look
	return done, cancel
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func answer(t *testing.T, done <-chan bool) bool {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(time.Second):
		t.Fatal("watchForRestart didn't return")
		return false
	}
}

func TestWatchUnchangedWaitsForCtx(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config.yaml")
	write(t, f, "a")
	done, cancel := startWatch(t, []string{f}, func() error { return nil })
	time.Sleep(10 * tick)
	select {
	case <-done:
		t.Fatal("returned without a change")
	default:
	}
	cancel()
	if answer(t, done) {
		t.Error("ctx ended: want false")
	}
}

func TestWatchChangeRestarts(t *testing.T) {
	dir := t.TempDir()
	config, env := filepath.Join(dir, "config.yaml"), filepath.Join(dir, "env")
	write(t, config, "a")
	write(t, env, "a")
	done, _ := startWatch(t, []string{config, env}, func() error { return nil })
	write(t, env, "MQTT_PASS=new")
	if !answer(t, done) {
		t.Error("env changed: want true")
	}
}

// A file that appears, like an icon dropped into a watched folder, is a change.
func TestWatchFolderGainingAFile(t *testing.T) {
	icons := t.TempDir()
	done, _ := startWatch(t, []string{icons}, func() error { return nil })
	write(t, filepath.Join(icons, "new.png"), "png")
	if !answer(t, done) {
		t.Error("a new file in the folder: want true")
	}
}

// A config that doesn't load keeps the running one, until it is fixed.
func TestWatchBadConfigIsKept(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config.yaml")
	write(t, f, "good")
	check := func() error {
		b, _ := os.ReadFile(f)
		if string(b) != "good again" {
			return errors.New("bad config")
		}
		return nil
	}
	done, _ := startWatch(t, []string{f}, check)

	write(t, f, "typo")
	time.Sleep(10 * tick)
	select {
	case <-done:
		t.Fatal("restarted for a bad config")
	default:
	}

	write(t, f, "good again")
	if !answer(t, done) {
		t.Error("fixed config: want true")
	}
}

// A file still changing is not read until it has stayed the same for one poll.
func TestWatchWaitsUntilSettled(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config.yaml")
	write(t, f, "a")
	checks := 0
	done, _ := startWatch(t, []string{f}, func() error { checks++; return nil })
	for i := range 10 {
		write(t, f, string(make([]byte, i+2))) // a new size on every poll
		time.Sleep(tick / 2)
	}
	if !answer(t, done) {
		t.Error("want true once it settles")
	}
	if checks != 1 {
		t.Errorf("checked %d times, want once, after it settled", checks)
	}
}
