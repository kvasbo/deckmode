package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
)

const (
	dryColumns  = 5 // as on a Stream Deck MK.2
	dryCellSize = 12
)

const dryHelp = `dry run: no deck, no broker. Type a command and press return:
  <number>        press that key
  state <payload> a message on the state topic
  offline, online the broker goes away or comes back
  quit`

// dryDeck is a Deck that prints the board.
type dryDeck struct {
	out        io.Writer
	keys       map[int]KeyView
	brightness int
}

func (d *dryDeck) Show(keys map[int]KeyView) error {
	if d.keys == nil {
		d.keys = map[int]KeyView{}
	}
	maps.Copy(d.keys, keys)
	fmt.Fprint(d.out, d.board())
	return nil
}

func (d *dryDeck) SetBrightness(percent int) error {
	d.brightness = percent
	fmt.Fprintf(d.out, "brightness %d%%\n", percent)
	return nil
}

// board is the keys in rows: [*1 Power*] is the lit one, and OFFLINE is
// printed above them when the keys show it.
func (d *dryDeck) board() string {
	var b strings.Builder
	last := slices.Max(slices.Collect(maps.Keys(d.keys)))
	rows := last/dryColumns + 1
	offline := false
	for _, v := range d.keys {
		offline = offline || v.Offline
	}
	if offline {
		b.WriteString("OFFLINE\n")
	}
	for row := range rows {
		for col := range dryColumns {
			b.WriteString(dryCell(row*dryColumns+col, d.keys))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func dryCell(key int, keys map[int]KeyView) string {
	v, ok := keys[key]
	text := strconv.Itoa(key)
	if ok {
		text += " " + v.Label
	}
	if v.Active {
		text = "*" + text + "*"
	}
	if r := []rune(text); len(r) > dryCellSize {
		text = string(r[:dryCellSize])
	}
	return fmt.Sprintf("[%-*s]", dryCellSize, text)
}

// dryBroker is a Broker and a screen in one: what's sent to the command topic
// comes straight back on the state topic, as if the screen had obeyed.
type dryBroker struct {
	ctx     context.Context
	out     io.Writer
	command string
	events  chan<- Event
}

func (b *dryBroker) Publish(topic, payload string) {
	fmt.Fprintf(b.out, "publish %s: %s\n", topic, payload)
	if topic == b.command {
		// Not from here: the controller that called this is what drains events.
		go send(b.ctx, b.events, Event{Kind: StateIn, Payload: payload})
	}
}

// parseDryCommand reads one line of input; ok is false if it means nothing.
func parseDryCommand(line string) (ev Event, ok bool) {
	line = strings.TrimSpace(line)
	if rest, found := strings.CutPrefix(line, "state "); found {
		return Event{Kind: StateIn, Payload: rest}, true
	}
	switch line {
	case "offline":
		return Event{Kind: BrokerDown}, true
	case "online":
		return Event{Kind: BrokerUp}, true
	}
	key, err := strconv.Atoi(line)
	if err != nil || key < 0 {
		return Event{}, false
	}
	return Event{Kind: KeyPressed, Key: key}, true
}

// runDryInput turns lines of input into events, and returns on "quit" or at
// the end of the input.
func runDryInput(ctx context.Context, in io.Reader, out io.Writer, events chan<- Event) {
	fmt.Fprintln(out, dryHelp)
	send(ctx, events, Event{Kind: DeckUp})
	send(ctx, events, Event{Kind: BrokerUp})
	lines := bufio.NewScanner(in)
	for lines.Scan() {
		if strings.TrimSpace(lines.Text()) == "quit" {
			return
		}
		ev, ok := parseDryCommand(lines.Text())
		if !ok {
			fmt.Fprintln(out, dryHelp)
			continue
		}
		send(ctx, events, ev)
	}
}
