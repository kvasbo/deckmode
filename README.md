# deckmode

Turns an Elgato Stream Deck into a control panel for a screen that switches modes over MQTT.
Each key is one mode. Pressing it sends the mode to the screen, and the key of the mode the
screen is really showing lights up. It talks to the deck directly over USB HID: no Elgato
software, no Companion. It is developed on a Mac and runs headless on a Raspberry Pi.

- The active mode's key is blue with a white edge, also right after a restart, without any press.
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
(`MQTT_USER`, `MQTT_PASS`), never from this file. Your real config stays on the Pi, or in a
git-ignored `*.local.yaml`.

## From a freshly flashed SD card

1. **Flash.** In Raspberry Pi Imager choose *Raspberry Pi OS Lite (64-bit)*. In its settings, set
   a hostname, a user, Wi-Fi if you need it, and turn on SSH.
2. **Boot and log in.** Put in the card, plug the Stream Deck into the Pi, power it on and wait a
   minute or two.

   ```bash
   ssh <user>@<hostname>.local
   ```

3. **Update the system and get the tools.**

   ```bash
   sudo apt update && sudo apt full-upgrade -y && sudo apt install -y curl
   ```

4. **Download the installer.**

   ```bash
   curl -fsSLO https://github.com/kvasbo/deckmode/releases/latest/download/deckmode-deploy.tar.gz
   ```

   ```bash
   tar xzf deckmode-deploy.tar.gz
   ```

5. **Install.** This creates the `deckmode` user, the udev rule, `/etc/deckmode/`, the service and
   the update timer, and downloads the latest binary.

   ```bash
   sudo deckmode-deploy/install.sh
   ```

6. **Configure.** Set the broker, topics and buttons, and then the broker's user and password.
   Put your own icons in `/etc/deckmode/icons/`.

   ```bash
   sudo nano /etc/deckmode/config.yaml
   ```

   ```bash
   sudo nano /etc/deckmode/env
   ```

7. **Start it and watch.** Look for `Stream Deck connected` and `MQTT connected`.

   ```bash
   sudo systemctl restart deckmode
   ```

   ```bash
   journalctl -u deckmode -f
   ```

## Updates and rollback

`deckmode-update.timer` runs hourly and looks for a newer GitHub release. If there is one, it
downloads the binary, verifies the checksum, checks the binary runs and reports the right version,
renames it into place, and restarts the service. The binary it replaced stays as
`/usr/local/bin/deckmode.previous`.

Nothing tells a bad release from a good one automatically. To go back:

```bash
sudo deckmode-update --rollback
```

That also remembers the version you left, so the timer doesn't reinstall it. The next newer
release is installed as usual. To see what the timer has done:

```bash
journalctl -u deckmode-update
```

The updater replaces only the binary. If a release changes the units or the updater itself, run
`install.sh` from that release again; it leaves your config and credentials alone.

## Manual test checklist

Unit tests cover the logic against a fake deck and a fake broker. These need the real things. Use
a test broker, or know that a press changes the real screen.

- [ ] **Starts dark, then right:** with a retained state on the broker, start deckmode. The state's
      key lights within seconds, without a press.
- [ ] **Press:** press another key. The screen changes, and the light follows once the screen has
      published its state.
- [ ] **Screen changes by itself:** change the mode from elsewhere, or let it time out. The light follows.
- [ ] **Unknown state:** publish a state no button has. No key is lit, and the log says why.
- [ ] **Busy (Mac):** with the Elgato app running, start deckmode. The log tells you to quit the
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
- [ ] **Stop:** `Ctrl-C` or `systemctl stop` leaves the keys dark.
- [ ] **Bad config:** a typo in the config gives a clear error at startup.
- [ ] **Pi, service:** after a reboot it comes back by itself, running as `deckmode`, not root
      (`ps -o user= -C deckmode`).
- [ ] **Pi, update:** tag a release. Within about an hour the Pi runs it (or
      `sudo systemctl start deckmode-update`, then `deckmode --version`), and `deckmode.previous` exists.
- [ ] **Pi, rollback:** `sudo deckmode-update --rollback`. The old version runs, and the timer leaves it alone.

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

It is the only one that covers replugging and the busy message without patching. Having no cgo
also settles glibc compatibility: the release is a static binary, which runs on any Pi OS release.
The cost: it's a small one-person project, and its GitHub repo is archived because development moved to
the author's own Forgejo. Go's module proxy keeps the pinned version available, and everything
that touches it is in `deck_hid.go`, so it can be swapped.

Icons in `icons/` are simple examples made for this repo.

## Development

```bash
go test -race ./...
```

Every push runs vet, tests and shellcheck. A tag `v*` builds the arm64 binary on GitHub's native arm64 runner,
checks it runs on Debian bookworm and trixie, and publishes it with `deckmode-deploy.tar.gz` and
`checksums.txt`. No workflow needs access to your network.

```bash
git tag v0.1.0 && git push origin v0.1.0
```
