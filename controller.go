package main

import (
	"context"
	"image"
	"log/slog"
	"maps"
	"strings"
	"time"
)

// dimBrightness is the panel's brightness after IdleTimeout, in percent.
const dimBrightness = 10

// Deck is the panel: the real one or the terminal's.
type Deck interface {
	// Show draws the given keys and leaves the others as they are.
	Show(keys map[int]KeyView) error
	SetBrightness(percent int) error
}

// Broker sends a message. Connection changes and messages coming the other
// way arrive as Events.
type Broker interface {
	Publish(topic, payload string)
}

// KeyView is what one key should look like. It is comparable, so a key whose
// view hasn't changed isn't drawn again.
type KeyView struct {
	Label   string
	Icon    image.Image
	Active  bool // this key's mode is on the screen
	Offline bool // no broker
}

type EventKind int

const (
	DeckUp     EventKind = iota // plugged in: everything must be drawn again
	DeckDown                    // unplugged
	KeyPressed                  // Key
	BrokerUp                    // connected
	BrokerDown                  // connection lost
	StateIn                     // Payload arrived on the state topic
)

type Event struct {
	Kind    EventKind
	Key     int
	Payload string
}

// send delivers an event unless the program is on its way down.
func send(ctx context.Context, events chan<- Event, ev Event) {
	select {
	case events <- ev:
	case <-ctx.Done():
	}
}

// Controller holds all the logic and all the state. Only its Run goroutine
// touches them; everything else talks to it through Events.
type Controller struct {
	cfg    *Config
	deck   Deck
	broker Broker
	now    func() time.Time

	active     string // the state topic's last payload
	online     bool
	deckUp     bool
	dimmed     bool
	lastActive time.Time
	shown      map[int]KeyView
}

func NewController(cfg *Config, deck Deck, broker Broker, now func() time.Time) *Controller {
	return &Controller{cfg: cfg, deck: deck, broker: broker, now: now, lastActive: now(), shown: map[int]KeyView{}}
}

// Run handles events until ctx ends.
func (c *Controller) Run(ctx context.Context, events <-chan Event) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			c.Handle(ev)
		case <-tick.C:
			c.checkIdle()
		}
	}
}

// Handle applies one event.
func (c *Controller) Handle(ev Event) {
	switch ev.Kind {
	case DeckUp:
		c.deckUp = true
		c.dimmed = false
		c.lastActive = c.now()
		c.shown = map[int]KeyView{}
		c.setBrightness(c.cfg.Brightness)
	case DeckDown:
		c.deckUp = false
	case KeyPressed:
		c.wake()
		c.press(ev.Key)
	case BrokerUp:
		c.online = true
	case BrokerDown:
		c.online = false
	case StateIn:
		c.active = strings.TrimSpace(ev.Payload)
		slog.Info("screen state", "payload", c.active)
		if c.active != "" && c.buttonFor(c.active) == nil {
			slog.Warn("state matches no button, so no key is lit", "payload", c.active)
		}
	}
	c.draw()
}

func (c *Controller) press(key int) {
	for _, b := range c.cfg.Buttons {
		if b.Key != key {
			continue
		}
		if !c.online {
			slog.Warn("key pressed while offline; nothing sent", "key", key)
			return
		}
		slog.Info("key pressed", "key", key, "payload", b.Payload)
		c.broker.Publish(c.cfg.Topics.Command, b.Payload)
		return
	}
}

func (c *Controller) buttonFor(payload string) *Button {
	for i := range c.cfg.Buttons {
		if c.cfg.Buttons[i].Payload == payload {
			return &c.cfg.Buttons[i]
		}
	}
	return nil
}

// views says how every configured key should look now.
func (c *Controller) views() map[int]KeyView {
	out := make(map[int]KeyView, len(c.cfg.Buttons))
	for _, b := range c.cfg.Buttons {
		out[b.Key] = KeyView{
			Label:   b.Label,
			Icon:    b.Image,
			Active:  c.online && b.Payload == c.active,
			Offline: !c.online,
		}
	}
	return out
}

// draw sends the deck the keys that changed. After a failure everything is
// drawn again next time; a deck that's gone reports DeckDown soon enough.
func (c *Controller) draw() {
	if !c.deckUp {
		return
	}
	views := c.views()
	changed := map[int]KeyView{}
	for key, v := range views {
		if old, ok := c.shown[key]; !ok || old != v {
			changed[key] = v
		}
	}
	if len(changed) == 0 {
		return
	}
	if err := c.deck.Show(changed); err != nil {
		slog.Warn("drawing keys", "err", err)
		c.shown = map[int]KeyView{}
		return
	}
	maps.Copy(c.shown, changed)
}

func (c *Controller) wake() {
	c.lastActive = c.now()
	if c.dimmed && c.deckUp {
		c.dimmed = false
		c.setBrightness(c.cfg.Brightness)
	}
}

func (c *Controller) checkIdle() {
	if c.cfg.IdleTimeout == 0 || !c.deckUp || c.dimmed {
		return
	}
	if c.now().Sub(c.lastActive) >= c.cfg.IdleTimeout {
		c.dimmed = true
		c.setBrightness(min(dimBrightness, c.cfg.Brightness))
	}
}

func (c *Controller) setBrightness(percent int) {
	if err := c.deck.SetBrightness(percent); err != nil {
		slog.Warn("setting brightness", "err", err)
	}
}
