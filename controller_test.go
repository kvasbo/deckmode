package main

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"
)

type fakeDeck struct {
	keys       map[int]KeyView
	draws      int // calls to Show
	drawn      int // keys drawn
	brightness []int
	fail       error
}

func (d *fakeDeck) Show(keys map[int]KeyView) error {
	if d.fail != nil {
		return d.fail
	}
	if d.keys == nil {
		d.keys = map[int]KeyView{}
	}
	maps.Copy(d.keys, keys)
	d.draws++
	d.drawn += len(keys)
	return nil
}

func (d *fakeDeck) SetBrightness(percent int) error {
	d.brightness = append(d.brightness, percent)
	return nil
}

// lit lists the keys drawn as active.
func (d *fakeDeck) lit() []int {
	out := []int{}
	for key, v := range d.keys {
		if v.Active {
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}

type fakeBroker struct{ sent []string }

func (b *fakeBroker) Publish(topic, payload string) {
	b.sent = append(b.sent, topic+"="+payload)
}

type fixture struct {
	c      *Controller
	deck   *fakeDeck
	broker *fakeBroker
	at     time.Time
}

// newFixture has a deck plugged in and a broker connected.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	cfg := &Config{
		Topics:      Topics{Command: "cmd", State: "state"},
		Brightness:  60,
		IdleTimeout: 10 * time.Minute,
		Buttons: []Button{
			{Key: 0, Label: "Home", Payload: "home"},
			{Key: 1, Label: "Power", Payload: "power"},
			{Key: 7, Label: "Week", Payload: "week"},
		},
	}
	f := &fixture{deck: &fakeDeck{}, broker: &fakeBroker{}, at: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	f.c = NewController(cfg, f.deck, f.broker, func() time.Time { return f.at })
	f.c.Handle(Event{Kind: DeckUp})
	f.c.Handle(Event{Kind: BrokerUp})
	return f
}

func TestStateLightsTheKey(t *testing.T) {
	f := newFixture(t)
	if got := f.deck.lit(); len(got) != 0 {
		t.Errorf("lit before any state: %v", got)
	}
	// The retained state is all it takes after a restart: no press.
	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	if got := f.deck.lit(); !slices.Equal(got, []int{1}) {
		t.Errorf("lit = %v, want key 1", got)
	}
	f.c.Handle(Event{Kind: StateIn, Payload: " week\n"})
	if got := f.deck.lit(); !slices.Equal(got, []int{7}) {
		t.Errorf("lit = %v, want only key 7", got)
	}
}

func TestUnknownOrEmptyStateLightsNothing(t *testing.T) {
	for _, payload := range []string{"nonsense", ""} {
		f := newFixture(t)
		f.c.Handle(Event{Kind: StateIn, Payload: "power"})
		f.c.Handle(Event{Kind: StateIn, Payload: payload})
		if got := f.deck.lit(); len(got) != 0 {
			t.Errorf("state %q: lit = %v", payload, got)
		}
	}
}

func TestPressPublishes(t *testing.T) {
	f := newFixture(t)
	f.c.Handle(Event{Kind: KeyPressed, Key: 7})
	if want := []string{"cmd=week"}; !slices.Equal(f.broker.sent, want) {
		t.Errorf("sent %q, want %q", f.broker.sent, want)
	}
}

// Only the state moves the highlight: it says what the screen really shows.
func TestPressAloneDoesNotLight(t *testing.T) {
	f := newFixture(t)
	f.c.Handle(Event{Kind: KeyPressed, Key: 7})
	if got := f.deck.lit(); len(got) != 0 {
		t.Errorf("lit = %v before the state came back", got)
	}
}

func TestPressOnUnconfiguredKey(t *testing.T) {
	f := newFixture(t)
	f.c.Handle(Event{Kind: KeyPressed, Key: 3})
	if len(f.broker.sent) != 0 {
		t.Errorf("sent %q", f.broker.sent)
	}
}

func TestOffline(t *testing.T) {
	f := newFixture(t)
	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	f.c.Handle(Event{Kind: BrokerDown})

	for key, v := range f.deck.keys {
		if !v.Offline || v.Active {
			t.Errorf("key %d while offline: %+v", key, v)
		}
	}
	f.c.Handle(Event{Kind: KeyPressed, Key: 0})
	if len(f.broker.sent) != 0 {
		t.Errorf("sent while offline: %q", f.broker.sent)
	}

	f.c.Handle(Event{Kind: BrokerUp})
	for key, v := range f.deck.keys {
		if v.Offline {
			t.Errorf("key %d still offline: %+v", key, v)
		}
	}
	if got := f.deck.lit(); !slices.Equal(got, []int{1}) {
		t.Errorf("lit after coming back = %v, want key 1", got)
	}
}

func TestStartsOfflineUntilTheBrokerAnswers(t *testing.T) {
	deck := &fakeDeck{}
	c := NewController(&Config{Brightness: 60, Buttons: []Button{{Key: 0, Label: "Home", Payload: "home"}}},
		deck, &fakeBroker{}, time.Now)
	c.Handle(Event{Kind: DeckUp})
	if !deck.keys[0].Offline {
		t.Errorf("key 0 = %+v, want offline", deck.keys[0])
	}
}

func TestReplugDrawsEverythingAgain(t *testing.T) {
	f := newFixture(t)
	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	f.c.Handle(Event{Kind: DeckDown})

	f.deck.keys, f.deck.brightness = nil, nil
	f.c.Handle(Event{Kind: StateIn, Payload: "week"}) // while unplugged
	if len(f.deck.keys) != 0 {
		t.Errorf("drew to an unplugged deck: %+v", f.deck.keys)
	}

	f.c.Handle(Event{Kind: DeckUp})
	if len(f.deck.keys) != 3 {
		t.Errorf("drew %d keys, want all 3", len(f.deck.keys))
	}
	if got := f.deck.lit(); !slices.Equal(got, []int{7}) {
		t.Errorf("lit = %v, want key 7", got)
	}
	if !slices.Equal(f.deck.brightness, []int{60}) {
		t.Errorf("brightness = %v, want 60 set again", f.deck.brightness)
	}
}

func TestOnlyChangedKeysAreDrawn(t *testing.T) {
	f := newFixture(t)
	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	draws, drawn := f.deck.draws, f.deck.drawn

	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	if f.deck.draws != draws {
		t.Error("the same state drew again")
	}
	f.c.Handle(Event{Kind: StateIn, Payload: "week"})
	if f.deck.drawn != drawn+2 {
		t.Errorf("drew %d keys, want the 2 that changed", f.deck.drawn-drawn)
	}
}

func TestFailedDrawIsRetried(t *testing.T) {
	f := newFixture(t)
	f.deck.fail = errors.New("usb")
	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	f.deck.fail = nil
	f.deck.keys = nil
	f.c.Handle(Event{Kind: StateIn, Payload: "power"})
	if len(f.deck.keys) != 3 {
		t.Errorf("drew %d keys after a failure, want all 3", len(f.deck.keys))
	}
}

func TestIdleDimsAndAnyKeyWakes(t *testing.T) {
	f := newFixture(t)
	f.deck.brightness = nil

	f.at = f.at.Add(9 * time.Minute)
	f.c.checkIdle()
	if len(f.deck.brightness) != 0 {
		t.Errorf("dimmed too early: %v", f.deck.brightness)
	}

	f.at = f.at.Add(time.Minute)
	f.c.checkIdle()
	f.c.checkIdle()
	if !slices.Equal(f.deck.brightness, []int{dimBrightness}) {
		t.Errorf("brightness = %v, want one dim", f.deck.brightness)
	}

	// Also a key without a mode wakes it.
	f.c.Handle(Event{Kind: KeyPressed, Key: 3})
	if !slices.Equal(f.deck.brightness, []int{dimBrightness, 60}) {
		t.Errorf("brightness = %v, want it back at 60", f.deck.brightness)
	}

	// The press counts as activity: the clock started over.
	f.at = f.at.Add(9 * time.Minute)
	f.c.checkIdle()
	if len(f.deck.brightness) != 2 {
		t.Errorf("dimmed again too early: %v", f.deck.brightness)
	}
}

// The dimmed keys stay readable, so a press does its job as well as waking.
func TestPressWhileDimmedStillPublishes(t *testing.T) {
	f := newFixture(t)
	f.at = f.at.Add(time.Hour)
	f.c.checkIdle()
	f.c.Handle(Event{Kind: KeyPressed, Key: 1})
	if want := []string{"cmd=power"}; !slices.Equal(f.broker.sent, want) {
		t.Errorf("sent %q, want %q", f.broker.sent, want)
	}
}

func TestNoIdleTimeoutNeverDims(t *testing.T) {
	f := newFixture(t)
	f.c.cfg.IdleTimeout = 0
	f.deck.brightness = nil
	f.at = f.at.Add(1000 * time.Hour)
	f.c.checkIdle()
	if len(f.deck.brightness) != 0 {
		t.Errorf("dimmed: %v", f.deck.brightness)
	}
}

// A dim level above the configured brightness would brighten the panel.
func TestDimNeverBrightens(t *testing.T) {
	f := newFixture(t)
	f.c.cfg.Brightness = 5
	f.deck.brightness = nil
	f.at = f.at.Add(time.Hour)
	f.c.checkIdle()
	if !slices.Equal(f.deck.brightness, []int{5}) {
		t.Errorf("brightness = %v, want 5", f.deck.brightness)
	}
}
