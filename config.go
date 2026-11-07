package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // icons may be JPEG
	_ "image/png"  // or PNG
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the YAML file; see config.example.yaml.
type Config struct {
	Broker       string   `yaml:"broker"`
	Topics       Topics   `yaml:"topics"`
	Brightness   int      `yaml:"brightness"`   // percent
	RawIdle      string   `yaml:"idle_timeout"` // a string, so that a bare 0 is allowed
	RawHighlight string   `yaml:"highlight"`    // like #1565C0
	Buttons      []Button `yaml:"buttons"`

	IdleTimeout time.Duration `yaml:"-"` // 0: never dim
	Highlight   color.RGBA    `yaml:"-"` // the active key's background
}

type Topics struct {
	Command string `yaml:"command"`
	State   string `yaml:"state"`
}

// Button is one mode on one key.
type Button struct {
	RawKey  *int   `yaml:"key"` // a pointer, to tell a missing key from key 0
	Label   string `yaml:"label"`
	Icon    string `yaml:"icon"` // relative to the config file
	Payload string `yaml:"payload"`

	Key   int         `yaml:"-"`
	Image image.Image `yaml:"-"` // the decoded icon, nil without one
}

// LoadConfig reads and validates the file. The error lists every problem, not
// just the first.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{Brightness: 70, RawHighlight: "#1565C0"}
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true) // a misspelled option is an error, not silently ignored
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.validate(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("%s:\n%w", path, err)
	}
	return cfg, nil
}

func (c *Config) validate(dir string) error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("  "+format, args...))
	}

	if err := checkBroker(c.Broker); err != nil {
		fail("broker: %v", err)
	}
	for _, t := range []struct{ name, topic string }{{"command", c.Topics.Command}, {"state", c.Topics.State}} {
		switch {
		case t.topic == "":
			fail("topics.%s: required", t.name)
		case strings.ContainsAny(t.topic, "+#"):
			fail("topics.%s: %q must not contain wildcards", t.name, t.topic)
		}
	}
	if c.Brightness < 1 || c.Brightness > 100 {
		fail("brightness: %d is not between 1 and 100", c.Brightness)
	}
	if c.RawIdle != "" {
		idle, err := time.ParseDuration(c.RawIdle)
		switch {
		case err != nil:
			fail("idle_timeout: %q is not a duration like 10m, or 0", c.RawIdle)
		case idle < 0:
			fail("idle_timeout: %v is negative", idle)
		default:
			c.IdleTimeout = idle
		}
	}
	if hl, err := parseColor(c.RawHighlight); err != nil {
		fail("highlight: %q is not a colour like \"#1565C0\"", c.RawHighlight)
	} else {
		c.Highlight = hl
	}
	if len(c.Buttons) == 0 {
		fail("buttons: at least one is required")
	}

	keys := map[int]bool{}
	payloads := map[string]bool{}
	for i := range c.Buttons {
		b := &c.Buttons[i]
		at := fmt.Sprintf("buttons[%d]", i)

		switch {
		case b.RawKey == nil:
			fail("%s: key is required", at)
		case *b.RawKey < 0:
			fail("%s: key %d is negative", at, *b.RawKey)
		case keys[*b.RawKey]:
			fail("%s: key %d is used twice", at, *b.RawKey)
		default:
			b.Key = *b.RawKey
			keys[b.Key] = true
		}

		switch {
		case b.Payload == "":
			fail("%s: payload is required", at)
		case payloads[b.Payload]:
			// The state topic couldn't say which of the two keys to light.
			fail("%s: payload %q is used twice", at, b.Payload)
		}
		payloads[b.Payload] = true

		if b.Label == "" && b.Icon == "" {
			fail("%s: needs a label or an icon", at)
		}
		if b.Icon != "" {
			img, err := loadIcon(dir, b.Icon)
			if err != nil {
				fail("%s: icon: %v", at, err)
			}
			b.Image = img
		}
	}
	return errors.Join(errs...)
}

// checkBroker wants a URL paho can use; paho needs the port spelled out.
func checkBroker(s string) error {
	if s == "" {
		return errors.New("required, e.g. tcp://broker.example.net:1883")
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "tcp", "mqtt", "ssl", "tls", "mqtts", "ws", "wss":
	default:
		return fmt.Errorf("%q needs a scheme like tcp:// or ssl://", s)
	}
	if u.Hostname() == "" || u.Port() == "" {
		return fmt.Errorf("%q needs both a host and a port", s)
	}
	return nil
}

// parseColor reads six hex digits, with or without a # in front.
func parseColor(s string) (color.RGBA, error) {
	hex := strings.TrimPrefix(s, "#")
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return color.RGBA{}, errors.New("not a colour")
	}
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 0xff}, nil
}

func loadIcon(dir, name string) (image.Image, error) {
	if !filepath.IsAbs(name) {
		name = filepath.Join(dir, name)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return img, nil
}
