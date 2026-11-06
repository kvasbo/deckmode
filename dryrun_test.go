package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestBoardShowsTheLitKeyInRows(t *testing.T) {
	var out bytes.Buffer
	d := &dryDeck{out: &out}
	d.Show(map[int]KeyView{0: {Label: "Home"}, 1: {Label: "Power", Active: true}, 7: {Label: "Week"}})

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d rows:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "[0 Home      ]") || !strings.Contains(lines[0], "[*1 Power*   ]") {
		t.Errorf("row 0: %s", lines[0])
	}
	if !strings.Contains(lines[1], "[7 Week      ]") {
		t.Errorf("row 1: %s", lines[1])
	}
	if strings.Contains(out.String(), "OFFLINE") {
		t.Errorf("online, but:\n%s", out.String())
	}
}

func TestBoardShowsOffline(t *testing.T) {
	var out bytes.Buffer
	d := &dryDeck{out: &out}
	d.Show(map[int]KeyView{0: {Label: "Home", Offline: true}})
	if !strings.HasPrefix(out.String(), "OFFLINE\n") {
		t.Errorf("got:\n%s", out.String())
	}
}

func TestParseDryCommand(t *testing.T) {
	cases := map[string]Event{
		"3":          {Kind: KeyPressed, Key: 3},
		" 12 ":       {Kind: KeyPressed, Key: 12},
		"state week": {Kind: StateIn, Payload: "week"},
		"offline":    {Kind: BrokerDown},
		"online":     {Kind: BrokerUp},
	}
	for in, want := range cases {
		if got, ok := parseDryCommand(in); !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "-1", "press", "state"} {
		if _, ok := parseDryCommand(in); ok {
			t.Errorf("%q should mean nothing", in)
		}
	}
}

// A whole session: a press goes out, the fake screen answers, the key lights.
func TestDryRunSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := &Config{
		Topics:     Topics{Command: "cmd", State: "state"},
		Brightness: 60,
		Buttons:    []Button{{Key: 0, Label: "Home", Payload: "home"}, {Key: 1, Label: "Power", Payload: "power"}},
	}
	var out bytes.Buffer
	events := make(chan Event, 16)
	c := NewController(cfg, &dryDeck{out: &out}, &dryBroker{ctx: ctx, out: &out, command: "cmd", events: events}, time.Now)

	runDryInput(ctx, strings.NewReader("1\n"), &out, events)
	for range 3 { // deck up, broker up, the press
		c.Handle(<-events)
	}
	c.Handle(<-events) // the fake screen's answer

	for _, want := range []string{"OFFLINE", "publish cmd: power", "[*1 Power*   ]"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("%q is missing from:\n%s", want, out.String())
		}
	}
}
