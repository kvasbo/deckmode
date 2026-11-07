#!/bin/bash
# Installs deckmode on a Mac, for the logged-in user: no sudo. Everything goes
# in ~/deckmode, where Finder and File Sharing show it: the config, the
# credentials, the icons, the logs and the binary. deckmode runs as a
# LaunchAgent, so it starts at login and again if it fails, and a second one
# looks for a new release every hour. Safe to run again: it never touches an
# existing config or env file, and it brings the agents and the updater up to
# date. Run it from the unpacked deckmode-deploy.tar.gz: ./install.sh
set -euo pipefail

LABEL=io.github.kvasbo.deckmode
DIR=$HOME/deckmode
AGENTS=$HOME/Library/LaunchAgents
DOMAIN=gui/$(id -u)

if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
	echo "install.sh: this needs a Mac with Apple silicon; this is $(uname -sm)" >&2
	exit 1
fi
if [ "$(id -u)" = 0 ]; then
	echo "install.sh: run me as yourself, not with sudo" >&2
	exit 1
fi
cd "$(dirname "$0")"

mkdir -p "$DIR/icons" "$AGENTS"
if [ ! -e "$DIR/config.yaml" ]; then
	install -m 644 config.example.yaml "$DIR/config.yaml"
fi
if [ ! -e "$DIR/env" ]; then
	install -m 600 env.example "$DIR/env"
fi
for icon in icons/*; do
	if [ ! -e "$DIR/$icon" ]; then
		install -m 644 "$icon" "$DIR/$icon"
	fi
done

install -m 755 deckmode-update "$DIR/"
"$DIR/deckmode-update"

# launchd reads no env file, so a shell loads it first. set -a exports it.
# --watch: an edit to any of these restarts deckmode within seconds, once the
# config still loads.
cat >"$AGENTS/$LABEL.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/sh</string>
		<string>-c</string>
		<string>set -a; . ./env; exec ./deckmode --config config.yaml --watch config.yaml,env,icons</string>
	</array>
	<key>WorkingDirectory</key>
	<string>$DIR</string>
	<key>RunAtLoad</key>
	<true/>
	<!-- Never give up: the broker or the deck may come back whenever. -->
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>5</integer>
	<key>StandardOutPath</key>
	<string>$DIR/deckmode.log</string>
	<key>StandardErrorPath</key>
	<string>$DIR/deckmode.log</string>
</dict>
</plist>
EOF

cat >"$AGENTS/$LABEL-update.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL-update</string>
	<key>ProgramArguments</key>
	<array>
		<string>$DIR/deckmode-update</string>
	</array>
	<key>StartInterval</key>
	<integer>3600</integer>
	<key>StandardOutPath</key>
	<string>$DIR/deckmode-update.log</string>
	<key>StandardErrorPath</key>
	<string>$DIR/deckmode-update.log</string>
</dict>
</plist>
EOF

# Unload the old agents first, and wait: launchd finishes that in the background.
for label in "$LABEL" "$LABEL-update"; do
	launchctl bootout "$DOMAIN/$label" 2>/dev/null || true
	for _ in 1 2 3 4 5 6 7 8 9 10; do
		launchctl print "$DOMAIN/$label" >/dev/null 2>&1 || break
		sleep 1
	done
	launchctl bootstrap "$DOMAIN" "$AGENTS/$label.plist"
done

cat <<EOF

Installed $("$DIR/deckmode" --version). Now:

  open -e ~/deckmode/config.yaml    the broker, the topics, the buttons
  open -e ~/deckmode/env            the broker's user and password
  tail -f ~/deckmode/deckmode.log   to watch it

deckmode restarts by itself a few seconds after you save either file.
EOF
