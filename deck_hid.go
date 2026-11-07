package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"sync"
	"time"

	"rafaelmartins.com/p/streamdeck"
)

// pollInterval is how often a missing or busy deck is looked for.
const pollInterval = 2 * time.Second

// hidDeck is the real Deck. It looks for a Stream Deck for as long as the
// program runs, so unplugging, replugging or quitting the app that holds it
// needs no restart.
type hidDeck struct {
	highlight color.RGBA // the active key's background

	mu   sync.Mutex
	dev  *streamdeck.Device // nil while there's no deck
	rect image.Rectangle    // a key's size
}

// Run blocks until ctx ends. A message is logged when the reason for having no
// deck changes, not on every attempt.
func (d *hidDeck) Run(ctx context.Context, events chan<- Event) {
	last := ""
	for ctx.Err() == nil {
		if err := d.session(ctx, events); err != nil {
			if msg := explain(err); msg != last {
				slog.Warn(msg)
				last = msg
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(pollInterval):
		}
	}
}

// session finds the deck and blocks while it stays connected.
func (d *hidDeck) session(ctx context.Context, events chan<- Event) error {
	dev, err := streamdeck.GetDevice("")
	if err != nil {
		return err
	}
	if err := dev.Open(); err != nil {
		return err
	}

	err = dev.ForEachKey(func(k streamdeck.KeyID) error {
		if err := dev.ClearKey(k); err != nil {
			return err
		}
		index := int(k - streamdeck.KEY_1)
		return dev.AddKeyHandler(k, func(*streamdeck.Device, *streamdeck.Key) error {
			send(ctx, events, Event{Kind: KeyPressed, Key: index})
			return nil
		})
	})
	if err != nil {
		dev.Close()
		return err
	}
	rect, err := dev.GetKeyImageRectangle()
	if err != nil {
		dev.Close()
		return err
	}

	d.mu.Lock()
	d.dev, d.rect = dev, rect
	d.mu.Unlock()
	slog.Info("Stream Deck connected", "model", dev.GetModelName(), "keys", dev.GetKeyCount())
	send(ctx, events, Event{Kind: DeckUp})

	gone := make(chan error, 1)
	go func() { gone <- dev.Listen(nil) }()

	select {
	case err = <-gone:
		slog.Warn("Stream Deck disconnected", "err", err)
		d.forget()
		send(ctx, events, Event{Kind: DeckDown})
		dev.Close()
		return nil
	case <-ctx.Done():
		// A dark panel, rather than keys that look alive with nothing behind.
		d.forget()
		_ = dev.ForEachKey(dev.ClearKey)
		dev.Close()
		<-gone
		return nil
	}
}

func (d *hidDeck) forget() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dev = nil
}

func (d *hidDeck) Show(keys map[int]KeyView) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dev == nil {
		return errors.New("no deck connected")
	}
	var errs []error
	for key, v := range keys {
		if key >= int(d.dev.GetKeyCount()) {
			errs = append(errs, fmt.Errorf("key %d: this deck has %d keys, numbered from 0", key, d.dev.GetKeyCount()))
			continue
		}
		if err := d.dev.SetKeyImage(streamdeck.KEY_1+streamdeck.KeyID(key), renderKey(v, d.rect, d.highlight)); err != nil {
			errs = append(errs, fmt.Errorf("key %d: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

func (d *hidDeck) SetBrightness(percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dev == nil {
		return errors.New("no deck connected")
	}
	return d.dev.SetBrightness(byte(percent))
}

// explain turns a failure to open the deck into what to do about it.
func explain(err error) string {
	switch {
	case errors.Is(err, streamdeck.ErrNoDeviceFound):
		return "no Stream Deck found; waiting for one to be plugged in"
	case errors.Is(err, streamdeck.ErrMoreThanOneDeviceFound):
		return "more than one Stream Deck found; leave one plugged in"
	case errors.Is(err, streamdeck.ErrDeviceLocked):
		return "the Stream Deck is busy: quit the Elgato Stream Deck app (menu bar icon, then Quit). Waiting for it to be free"
	}
	return fmt.Sprintf("can't open the Stream Deck: %v", err)
}
