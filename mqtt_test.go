package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Against a real broker, for a throwaway one only, as it writes to the topics:
//
//	docker run -d --rm --name mqtt-test -p 127.0.0.1:18830:1883 eclipse-mosquitto:2 \
//	  mosquitto -c /mosquitto-no-auth.conf
//	DECKMODE_TEST_BROKER=tcp://127.0.0.1:18830 go test -run RealBroker .
func TestRealBroker(t *testing.T) {
	url := os.Getenv("DECKMODE_TEST_BROKER")
	if url == "" {
		t.Skip("DECKMODE_TEST_BROKER is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	stamp := time.Now().UnixNano()
	command := fmt.Sprintf("deckmode-test/%d/command", stamp)
	state := fmt.Sprintf("deckmode-test/%d/state", stamp)

	// A separate client: the screen.
	screen := mqtt.NewClient(mqtt.NewClientOptions().AddBroker(url).SetClientID(fmt.Sprintf("screen-%d", stamp)))
	if token := screen.Connect(); token.Wait() && token.Error() != nil {
		t.Fatal(token.Error())
	}
	defer screen.Disconnect(100)
	received := make(chan mqtt.Message, 1)
	if token := screen.Subscribe(command, 1, func(_ mqtt.Client, m mqtt.Message) { received <- m }); token.Wait() && token.Error() != nil {
		t.Fatal(token.Error())
	}
	if token := screen.Publish(state, 1, true, "power"); token.Wait() && token.Error() != nil {
		t.Fatal(token.Error())
	}
	defer screen.Publish(state, 1, true, "") // clears the retained message

	events := make(chan Event, 8)
	cfg := &Config{Broker: url, Topics: Topics{Command: command, State: state}}
	b := newMQTTBroker(ctx, cfg, "", "", events)
	go b.Run(ctx)

	// The state retained from before deckmode started arrives with no press.
	want := []Event{{Kind: BrokerUp}, {Kind: StateIn, Payload: "power"}}
	for _, w := range want {
		select {
		case got := <-events:
			if got != w {
				t.Fatalf("got %+v, want %+v", got, w)
			}
		case <-ctx.Done():
			t.Fatalf("waiting for %+v: %v", w, ctx.Err())
		}
	}

	b.Publish(command, "week")
	select {
	case m := <-received:
		if string(m.Payload()) != "week" || m.Retained() {
			t.Errorf("got %q, retained %v", m.Payload(), m.Retained())
		}
	case <-ctx.Done():
		t.Fatal("the command never arrived")
	}

	// A latecomer gets nothing on the command topic: it isn't retained.
	late := mqtt.NewClient(mqtt.NewClientOptions().AddBroker(url).SetClientID(fmt.Sprintf("late-%d", stamp)))
	if token := late.Connect(); token.Wait() && token.Error() != nil {
		t.Fatal(token.Error())
	}
	defer late.Disconnect(100)
	retained := make(chan mqtt.Message, 1)
	if token := late.Subscribe(command, 1, func(_ mqtt.Client, m mqtt.Message) { retained <- m }); token.Wait() && token.Error() != nil {
		t.Fatal(token.Error())
	}
	select {
	case m := <-retained:
		t.Errorf("the command was retained: %q", m.Payload())
	case <-time.After(500 * time.Millisecond):
	}
}
