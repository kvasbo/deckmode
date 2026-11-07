package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConfig puts the YAML in a temp dir, beside an icon called icon.png.
func writeConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const goodConfig = `
broker: tcp://broker.example.net:1883
topics:
  command: screen/mode/set
  state: screen/mode/state
brightness: 40
idle_timeout: 10m
buttons:
  - {key: 0, label: Home, icon: icon.png, payload: home}
  - {key: 4, label: Week, payload: week}
`

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, goodConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Brightness != 40 || cfg.IdleTimeout != 10*time.Minute || cfg.Topics.State != "screen/mode/state" {
		t.Errorf("got %+v", cfg)
	}
	if len(cfg.Buttons) != 2 || cfg.Buttons[1].Key != 4 || cfg.Buttons[1].Payload != "week" {
		t.Errorf("buttons: %+v", cfg.Buttons)
	}
	if cfg.Buttons[0].Image == nil || cfg.Buttons[1].Image != nil {
		t.Error("the icon should be loaded relative to the config, and only for the button that has one")
	}
}

func TestBrightnessDefaults(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, strings.Replace(goodConfig, "brightness: 40\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Brightness != 70 {
		t.Errorf("brightness = %d, want the default 70", cfg.Brightness)
	}
}

func TestHighlight(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, goodConfig))
	if err != nil {
		t.Fatal(err)
	}
	if want := (color.RGBA{0x15, 0x65, 0xc0, 0xff}); cfg.Highlight != want {
		t.Errorf("default highlight = %v, want %v", cfg.Highlight, want)
	}

	// With or without the #, any case. A bare 123456 is a number to YAML.
	cases := map[string]color.RGBA{
		`"#ff8800"`: {0xff, 0x88, 0x00, 0xff},
		`"#FF8800"`: {0xff, 0x88, 0x00, 0xff},
		`'ff8800'`:  {0xff, 0x88, 0x00, 0xff},
		`2e7d32`:    {0x2e, 0x7d, 0x32, 0xff},
		`123456`:    {0x12, 0x34, 0x56, 0xff},
	}
	for in, want := range cases {
		cfg, err := LoadConfig(writeConfig(t, goodConfig+"highlight: "+in+"\n"))
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if cfg.Highlight != want {
			t.Errorf("%s: got %v, want %v", in, cfg.Highlight, want)
		}
	}

	for _, in := range []string{`blue`, `"#12345"`, `"#1234567"`, `"#12345g"`, `"+12345"`} {
		_, err := LoadConfig(writeConfig(t, goodConfig+"highlight: "+in+"\n"))
		if err == nil || !strings.Contains(err.Error(), "highlight:") {
			t.Errorf("%s: want a highlight error, got %v", in, err)
		}
	}
}

// The example's comment promises that 0 means never dim, and a bare 0 is what
// people write.
func TestIdleTimeoutForms(t *testing.T) {
	cases := map[string]time.Duration{"0": 0, "0s": 0, "90s": 90 * time.Second, "1h": time.Hour}
	for in, want := range cases {
		cfg, err := LoadConfig(writeConfig(t, strings.Replace(goodConfig, "10m", in, 1)))
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if cfg.IdleTimeout != want {
			t.Errorf("%q: got %v, want %v", in, cfg.IdleTimeout, want)
		}
	}
	cfg, err := LoadConfig(writeConfig(t, strings.Replace(goodConfig, "idle_timeout: 10m\n", "", 1)))
	if err != nil || cfg.IdleTimeout != 0 {
		t.Errorf("left out: got %v (%v), want no dimming", cfg, err)
	}
}

// The example in the repo must always load: people start from it.
func TestExampleConfigLoads(t *testing.T) {
	if _, err := LoadConfig("config.example.yaml"); err != nil {
		t.Error(err)
	}
}

func TestConfigErrors(t *testing.T) {
	cases := []struct {
		name, old, new, want string
	}{
		{"no broker", "broker: tcp://broker.example.net:1883\n", "", "broker: required"},
		{"no scheme", "tcp://broker.example.net:1883", "broker.example.net:1883", "broker:"},
		{"no port", "tcp://broker.example.net:1883", "tcp://broker.example.net", "host and a port"},
		{"no command topic", "command: screen/mode/set", "command: ''", "topics.command: required"},
		{"wildcard", "state: screen/mode/state", "state: screen/#", "wildcards"},
		{"brightness", "brightness: 40", "brightness: 140", "brightness: 140"},
		{"idle", "idle_timeout: 10m", "idle_timeout: -1m", "negative"},
		{"bad duration", "idle_timeout: 10m", "idle_timeout: soon", "soon"},
		{"typo", "brightness: 40", "brightnes: 40", "brightnes"},
		{"no key", "{key: 4, label: Week", "{label: Week", "key is required"},
		{"negative key", "key: 4", "key: -4", "negative"},
		{"same key", "key: 4", "key: 0", "key 0 is used twice"},
		{"no payload", ", payload: week", "", "payload is required"},
		{"same payload", "payload: week", "payload: home", `"home" is used twice`},
		{"no label or icon", "label: Week, ", "", "label or an icon"},
		{"missing icon", "icon: icon.png", "icon: nope.png", "nope.png"},
		{"not an image", "icon: icon.png", "icon: config.yaml", "config.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(goodConfig, tc.old) {
				t.Fatalf("the case doesn't apply: %q", tc.old)
			}
			_, err := LoadConfig(writeConfig(t, strings.Replace(goodConfig, tc.old, tc.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestConfigListsEveryProblem(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "broker: nope\nbrightness: 0\n"))
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"broker:", "topics.command", "topics.state", "brightness", "buttons:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q is missing from:\n%v", want, err)
		}
	}
}

func TestConfigFileMissing(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("want an error")
	}
}
