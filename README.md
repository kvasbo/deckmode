# deckmode

Turns an Elgato Stream Deck into a control panel for a screen that switches modes over MQTT.
Each key is one mode. Pressing it sends the mode to the screen, and the key of the mode the
screen is really showing lights up. It talks to the deck directly over USB HID: no Elgato
software, no Companion. It runs in the background on a Mac with Apple silicon.

- The active mode's key lights up with a white edge, in blue or the `highlight` colour you
  choose, also right after a restart, without any press.
- Without a broker every key goes faint with a red OFFLINE band. It reconnects on its own, with
  backoff, and presses meanwhile send nothing.
- Unplug the deck whenever: it is picked up again when it comes back.
- The panel dims after a configurable idle time, and any key wakes it.

## What the screen must do

| Topic | Who writes | Retained | Payload |
| --- | --- | --- | --- |
| command | deckmode | no | a button's `payload` |
| state | the screen | **yes** | the mode showing, equal to a button's `payload` |

The screen has to publish the state itself, also when it changes mode on its own. deckmode only
reads it, so the light always tells what the screen really shows.

## Try it without hardware

```bash
go run . --dry-run --config config.example.yaml
```

This uses no deck and no broker. It prints the board and reads commands: a key number to press
it, `state <payload>`, `offline`, `online`, `quit`. What you press comes straight back as the
state, as if a screen had obeyed.

```
[0 Home      ][*1 Power*   ][2 Calendar  ][3 Weather   ][4 Week      ]
```

## Run it with a real deck on the Mac

Quit the Elgato Stream Deck app first (menu bar icon, then Quit). It holds the device; if it is
running, deckmode says so and carries on as soon as the app is gone.

```bash
cp config.example.yaml config.local.yaml    # git ignores this; change it to your broker and topics
MQTT_USER=me MQTT_PASS=secret go run . --config config.local.yaml
```

## Config

One YAML file; [config.example.yaml](config.example.yaml) explains every option. It is checked at
startup, and the error lists everything wrong with it. Credentials come from the environment
(`MQTT_USER`, `MQTT_PASS`), never from this file. Your real config stays on the Mac that runs
deckmode, or in a git-ignored `*.local.yaml`.

## Install

For a Mac with Apple silicon that stays logged in, like one driving a wall display. deckmode runs
as a LaunchAgent of that user: it starts at login, restarts if it fails, and a second agent looks
for a new release every hour. No sudo, and nothing outside your home directory. Quit the Elgato
Stream Deck app for good (and remove it from Login Items), or it will hold the deck.

1. **Download and install.** This puts everything in `~/deckmode`: the config, the credentials,
   the icons, the logs and the binary, where Finder and File Sharing show them. It downloads the
   latest binary and starts both agents.

   ```bash
   cd /tmp && curl -fsSL https://github.com/kvasbo/deckmode/releases/latest/download/deckmode-deploy.tar.gz | tar xz
   ```

   ```bash
   /tmp/deckmode-deploy/install.sh
   ```

2. **Configure.** Set the broker, topics and buttons in `config.yaml`, then the broker's user and
   password in `env`. Put your own icons in `icons/`. Any text editor will do, also over File
   Sharing from another Mac.

   ```bash
   open -e ~/deckmode/config.yaml
   ```

   ```bash
   open -e ~/deckmode/env
   ```

3. **Restart it and watch.** Look for `Stream Deck connected` and `MQTT connected`.

   ```bash
   launchctl kickstart -k gui/$(id -u)/io.github.kvasbo.deckmode
   ```

   ```bash
   tail -f ~/deckmode/deckmode.log
   ```

## Updates and rollback

The update agent runs hourly and looks for a newer GitHub release. If there is one, it downloads
the binary, verifies the checksum, checks the binary runs and reports the right version, renames
it into place, and restarts deckmode. The binary it replaced stays as `deckmode.previous`. Its log
is `~/deckmode/deckmode-update.log`.

Nothing tells a bad release from a good one automatically. To go back:

```bash
~/deckmode/deckmode-update --rollback
```

That also remembers the version you left, so the agent doesn't reinstall it. The next newer
release is installed as usual.

The updater replaces only the binary. If a release changes the agents or the updater itself, run
`install.sh` from that release again; it leaves your config and credentials alone.

## Remove it

```bash
for a in io.github.kvasbo.deckmode io.github.kvasbo.deckmode-update; do launchctl bootout gui/$(id -u)/$a; rm ~/Library/LaunchAgents/$a.plist; done
```

```bash
rm -r ~/deckmode
```

## Manual test checklist

Unit tests cover the logic against a fake deck and a fake broker. These need the real things. Use
a test broker, or know that a press changes the real screen.

- [ ] **Starts dark, then right:** with a retained state on the broker, start deckmode. The state's
      key lights within seconds, without a press.
- [ ] **Press:** press another key. The screen changes, and the light follows once the screen has
      published its state.
- [ ] **Screen changes by itself:** change the mode from elsewhere, or let it time out. The light follows.
- [ ] **Unknown state:** publish a state no button has. No key is lit, and the log says why.
- [ ] **Busy:** with the Elgato app running, start deckmode. The log tells you to quit the
      app. Quit it. The keys appear without a restart.
- [ ] **Broker down:** stop the broker. All keys show OFFLINE, within about 20 seconds at worst. A press
      sends nothing.
- [ ] **Broker back:** start it again. Within a minute the keys return with the right light, without a restart.
- [ ] **Broker down at start:** start deckmode with no broker. OFFLINE shows, and it connects once
      the broker appears.
- [ ] **Unplug:** pull the deck's cable, and wait a few seconds. deckmode keeps running, and the log says so.
- [ ] **Replug:** plug it back in. Keys and light return within a few seconds, at full brightness.
- [ ] **State changes while unplugged:** the replugged deck shows the new state.
- [ ] **Idle:** with a short `idle_timeout`, the panel dims. Any key wakes it, also one with no button.
- [ ] **Stop:** `Ctrl-C`, or `launchctl kill TERM gui/$(id -u)/io.github.kvasbo.deckmode`, leaves
      the keys dark.
- [ ] **Bad config:** a typo in the config gives a clear error at startup.
- [ ] **Login:** after a restart and login it runs by itself
      (`launchctl print gui/$(id -u)/io.github.kvasbo.deckmode | grep state`).
- [ ] **Update:** tag a release. Within about an hour the Mac runs it (or
      `launchctl kickstart gui/$(id -u)/io.github.kvasbo.deckmode-update`), the log says so, and
      `deckmode.previous` exists.
- [ ] **Rollback:** `deckmode-update --rollback`. The old version runs, and the agent leaves it alone.

## How it's built

All the logic is in `controller.go`, on one goroutine, fed by events. The rest is thin:
`deck_hid.go` (the deck), `mqtt.go` (the broker), `render.go` (a key's pixels), `dryrun.go` (the
terminal's deck and broker), `config.go`.

**Stream Deck library: [`rafaelmartins.com/p/streamdeck`](https://rafaelmartins.com/p/streamdeck/).**
These were compared:

| Library | No cgo | Reports unplugging | Says device is busy | Last active |
| --- | --- | --- | --- | --- |
| **rafaelmartins** | yes | yes, `Listen` returns | yes, `ErrDeviceLocked` | 2026 |
| muesli/streamdeck | no (karalabe/hid from 2019) | yes, channel closes | no | 2023 |
| dh1tw/streamdeck | no (bearsh/hid) | no, the read loop keeps spinning | no | 2025 |
| magicmonkey/go-streamdeck | no (karalabe/hid from 2019) | not checked | not checked | 2024 |

It is the only one that covers replugging and the busy message without patching. The cost: it's a small one-person project, and its GitHub repo is archived because development moved to
the author's own Forgejo. Go's module proxy keeps the pinned version available, and everything
that touches it is in `deck_hid.go`, so it can be swapped.

Icons in `icons/` are simple examples made for this repo.

## Development

```bash
go test -race ./...
```

Every push runs vet, tests and shellcheck. A tag `v*` builds the binary on GitHub's Apple silicon
runner, checks it runs and reports the tag, and publishes it with `deckmode-deploy.tar.gz` (the
installer) and `checksums.txt`. No workflow needs access to your network.

```bash
git tag v0.3.0 && git push origin v0.3.0
```
