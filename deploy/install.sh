#!/bin/bash
# Installs deckmode on a Raspberry Pi. Safe to run again: it never touches an
# existing config or env file, and it brings the units and the updater up to
# date. Run it from the unpacked deckmode-deploy.tar.gz: sudo ./install.sh
set -euo pipefail

NAME=deckmode
ETC=/etc/$NAME

if [ "$(id -u)" != 0 ]; then
	echo "install.sh: run me with sudo" >&2
	exit 1
fi
if [ "$(uname -m)" != aarch64 ]; then
	echo "install.sh: this needs 64-bit Raspberry Pi OS; this is $(uname -m)" >&2
	exit 1
fi
cd "$(dirname "$0")"

# The user first: the udev rule and the file modes below name its group.
if ! id "$NAME" >/dev/null 2>&1; then
	useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin "$NAME"
fi

install -d -m 755 "$ETC" "$ETC/icons"
if [ ! -e "$ETC/config.yaml" ]; then
	install -m 640 -o root -g "$NAME" config.example.yaml "$ETC/config.yaml"
fi
if [ ! -e "$ETC/env" ]; then
	install -m 640 -o root -g "$NAME" env.example "$ETC/env"
fi
for icon in icons/*; do
	if [ ! -e "$ETC/$icon" ]; then
		install -m 644 "$icon" "$ETC/$icon"
	fi
done

install -m 644 50-deckmode-streamdeck.rules /etc/udev/rules.d/
udevadm control --reload-rules
udevadm trigger --subsystem-match=hidraw

install -m 755 deckmode-update /usr/local/sbin/
install -m 644 deckmode.service deckmode-update.service deckmode-update.timer /etc/systemd/system/
systemctl daemon-reload

/usr/local/sbin/deckmode-update

systemctl enable --now deckmode-update.timer
systemctl enable deckmode.service
systemctl restart deckmode.service

cat <<EOF

Installed $(/usr/local/bin/deckmode --version). Now:

  sudo nano $ETC/config.yaml      the broker, the topics, the buttons
  sudo nano $ETC/env              the broker's user and password
  sudo systemctl restart $NAME
  journalctl -u $NAME -f          to watch it
EOF
