// deckmode turns an Elgato Stream Deck into a control panel for a screen that
// takes its mode over MQTT: one key per mode, with the active one lit.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// version is set by the release build.
var version = "dev"

func main() {
	home, _ := os.UserHomeDir()
	configPath := flag.String("config", filepath.Join(home, "deckmode", "config.yaml"), "the config file")
	dryRun := flag.Bool("dry-run", false, "no deck and no broker: print the board and read presses from the terminal")
	showVersion := flag.Bool("version", false, "print the version and exit")
	watch := flag.String("watch", "", "comma-separated files and folders: when one changes, and the config still loads, exit with status 3 so launchd starts deckmode again")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deckmode: bad config: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	events := make(chan Event, 64)
	var deck Deck
	var broker Broker
	var wg sync.WaitGroup

	if *dryRun {
		deck = &dryDeck{out: os.Stdout}
		broker = &dryBroker{ctx: ctx, out: os.Stdout, command: cfg.Topics.Command, events: events}
		go func() {
			runDryInput(ctx, os.Stdin, os.Stdout, events)
			time.Sleep(100 * time.Millisecond) // lets the last output appear
			stop()
		}()
	} else {
		hid := &hidDeck{highlight: cfg.Highlight}
		mqtt := newMQTTBroker(ctx, cfg, os.Getenv("MQTT_USER"), os.Getenv("MQTT_PASS"), events)
		deck, broker = hid, mqtt
		wg.Go(func() { hid.Run(ctx, events) })
		wg.Go(func() { mqtt.Run(ctx) })
	}

	restart := false
	if *watch != "" {
		files := strings.Split(*watch, ",")
		check := func() error { _, err := LoadConfig(*configPath); return err }
		wg.Go(func() {
			if watchForRestart(ctx, 2*time.Second, files, check) {
				slog.Info("restarting for the changed files")
				restart = true
				stop()
			}
		})
	}

	slog.Info("deckmode starting", "version", version, "buttons", len(cfg.Buttons))
	NewController(cfg, deck, broker, time.Now).Run(ctx, events)
	wg.Wait()
	if restart {
		os.Exit(exitRestart)
	}
}
