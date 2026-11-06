package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// maxBackoff is the longest wait between connection attempts.
const maxBackoff = time.Minute

// mqttBroker is the real Broker. It reports connection changes and the state
// topic's messages as Events.
type mqttBroker struct {
	client mqtt.Client
	url    string
}

func newMQTTBroker(ctx context.Context, cfg *Config, user, password string, events chan<- Event) *mqttBroker {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(fmt.Sprintf("deckmode-%06x", rand.IntN(1<<24))).
		SetUsername(user).
		SetPassword(password).
		SetKeepAlive(15 * time.Second).
		SetPingTimeout(5 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetAutoReconnect(true). // with backoff, up to the interval below
		SetMaxReconnectInterval(maxBackoff).
		SetOnConnectHandler(func(c mqtt.Client) {
			slog.Info("MQTT connected", "broker", cfg.Broker)
			send(ctx, events, Event{Kind: BrokerUp})
			// Subscribe on every connect; the session is not persistent. The
			// broker answers with the retained state, so the right key lights
			// without any press.
			token := c.Subscribe(cfg.Topics.State, 1, func(_ mqtt.Client, msg mqtt.Message) {
				send(ctx, events, Event{Kind: StateIn, Payload: string(msg.Payload())})
			})
			if token.Wait(); token.Error() != nil {
				slog.Error("MQTT subscribe failed", "topic", cfg.Topics.State, "err", token.Error())
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			slog.Warn("MQTT connection lost", "err", err)
			send(ctx, events, Event{Kind: BrokerDown})
		}).
		SetReconnectingHandler(func(mqtt.Client, *mqtt.ClientOptions) {
			slog.Info("MQTT reconnecting", "broker", cfg.Broker)
		})
	return &mqttBroker{client: mqtt.NewClient(opts), url: cfg.Broker}
}

// Run connects, and blocks until ctx ends. Paho only backs off when
// reconnecting, so the first connection does that here.
func (b *mqttBroker) Run(ctx context.Context) {
	delay := time.Second
	for {
		token := b.client.Connect()
		token.Wait()
		if token.Error() == nil {
			break
		}
		slog.Warn("MQTT connect failed", "broker", b.url, "err", token.Error(), "retry_in", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxBackoff)
	}
	<-ctx.Done()
	b.client.Disconnect(250)
}

// Publish sends at most once and never retained. A press whose message can't
// go now must not change the screen later, after a reconnect; and a screen may
// well ignore retained commands, which the broker would replay.
func (b *mqttBroker) Publish(topic, payload string) {
	b.client.Publish(topic, 0, false, payload)
}
