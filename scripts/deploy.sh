#!/usr/bin/env bash
#
# Build trackerscan with Theos, copy the resulting .deb to a jailbroken
# iPhone over SSH, and install it there.
#
# Usage:
#   scripts/deploy.sh [-h host] [-u user] [-p port] [--usb] [--no-build] [--run <bundleID>]
#
# Configuration (in priority order): command-line flags, then environment
# variables, then the defaults below.
#   TRACKERSCAN_HOST   iPhone IP address or hostname          (default: 192.168.50.77)
#   TRACKERSCAN_USER   SSH login user                          (default: mobile)
#   TRACKERSCAN_PORT   SSH port                                (default: 22)
#
# --usb connects over a USB cable via iproxy/usbmuxd instead of Wi-Fi, so it
# works regardless of which network the Mac is on and even if the phone has
# no network access at all. It starts (or reuses) a local iproxy tunnel and
# overrides host/port to 127.0.0.1:<local iproxy port>.
#
# Examples:
#   scripts/deploy.sh
#   scripts/deploy.sh -h 192.168.1.42 -u mobile
#   TRACKERSCAN_HOST=192.168.1.42 scripts/deploy.sh --run com.example.app
#   scripts/deploy.sh --usb --run com.example.app
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

HOST="${TRACKERSCAN_HOST:-10.192.90.25}"
USER="${TRACKERSCAN_USER:-mobile}"
PORT="${TRACKERSCAN_PORT:-22}"
DO_BUILD=1
RUN_BUNDLE_ID=""
USE_USB=0

# USB tunnel settings; the pidfile/logfile names match ios-ssh.sh so both
# scripts reuse the same background iproxy process instead of starting
# duplicates.
USB_LOCAL_PORT=2222
USB_REMOTE_PORT=22
USB_PIDFILE="/tmp/.ios-ssh-iproxy.pid"
USB_LOGFILE="/tmp/.ios-ssh-iproxy.log"

usage() {
    cat <<EOF
Usage: $(basename "$0") [-h host] [-u user] [-p port] [--usb] [--no-build] [--run <bundleID>]

  -h, --host <host>       iPhone IP/hostname (default: \$TRACKERSCAN_HOST or $HOST)
  -u, --user <user>       SSH user (default: \$TRACKERSCAN_USER or $USER)
  -p, --port <port>       SSH port (default: \$TRACKERSCAN_PORT or $PORT)
      --usb               connect over USB via iproxy instead of Wi-Fi/IP;
                           overrides -h/-p with 127.0.0.1:$USB_LOCAL_PORT
      --no-build          skip 'make package', reuse the newest existing .deb
      --run <bundleID>    after installing, run 'trackerscan <bundleID>' on the
                           device and print its JSON output
  --help                  show this help
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        -h|--host) HOST="$2"; shift 2 ;;
        -u|--user) USER="$2"; shift 2 ;;
        -p|--port) PORT="$2"; shift 2 ;;
        --usb) USE_USB=1; shift ;;
        --no-build) DO_BUILD=0; shift ;;
        --run) RUN_BUNDLE_ID="$2"; shift 2 ;;
        --help) usage; exit 0 ;;
        *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

# --- USB tunnel (iproxy/usbmuxd) -------------------------------------------
usb_port_open() {
    nc -z -G 1 127.0.0.1 "$USB_LOCAL_PORT" >/dev/null 2>&1
}

usb_start_iproxy() {
    echo "==> Starting iproxy $USB_LOCAL_PORT -> $USB_REMOTE_PORT (USB)" >&2
    iproxy "$USB_LOCAL_PORT" "$USB_REMOTE_PORT" >"$USB_LOGFILE" 2>&1 &
    echo $! > "$USB_PIDFILE"
    disown

    for _ in $(seq 1 20); do
        usb_port_open && return 0
        sleep 0.3
    done

    echo "error: iproxy did not open port $USB_LOCAL_PORT in time." >&2
    echo "Is the iPhone plugged in via USB and unlocked/trusted?" >&2
    echo "--- iproxy log ---" >&2
    cat "$USB_LOGFILE" >&2 2>/dev/null || true
    exit 1
}

if [ "$USE_USB" -eq 1 ]; then
    # Reuse an already-running iproxy for this port; only start a new one if
    # nothing is listening yet (handles stale PID files gracefully).
    if ! usb_port_open; then
        if [ -f "$USB_PIDFILE" ] && kill -0 "$(cat "$USB_PIDFILE" 2>/dev/null)" 2>/dev/null; then
            for _ in $(seq 1 10); do
                usb_port_open && break
                sleep 0.3
            done
        fi
        usb_port_open || usb_start_iproxy
    fi
    HOST="127.0.0.1"
    PORT="$USB_LOCAL_PORT"
    echo "==> USB mode: using $USER@$HOST:$PORT (via iproxy)"
fi

cd "$REPO_ROOT"

if [ "$DO_BUILD" -eq 1 ]; then
    if [ -z "${THEOS:-}" ]; then
        export THEOS="$HOME/theos"
    fi
    if [ ! -f "$THEOS/makefiles/common.mk" ]; then
        echo "error: Theos not found at THEOS=$THEOS (missing makefiles/common.mk)" >&2
        echo "install it first: git clone --recursive https://github.com/theos/theos.git \"$THEOS\"" >&2
        exit 1
    fi
    echo "==> Building package (THEOS=$THEOS)"
    make package
fi

DEB_PATH="$(ls -t packages/com.trackerscan.cli_*.deb 2>/dev/null | head -1)"
if [ -z "$DEB_PATH" ]; then
    echo "error: no .deb found under packages/ — run without --no-build first" >&2
    exit 1
fi
echo "==> Using package: $DEB_PATH"

SSH_OPTS=(-p "$PORT")
SCP_OPTS=(-P "$PORT")
REMOTE_DEB="/tmp/$(basename "$DEB_PATH")"

echo "==> Copying to $USER@$HOST:$REMOTE_DEB"
scp "${SCP_OPTS[@]}" "$DEB_PATH" "$USER@$HOST:$REMOTE_DEB"

echo "==> Installing on device (sudo apt-get install -y)"
# -t forces a pseudo-terminal so sudo has somewhere to prompt for a password.
ssh -t "${SSH_OPTS[@]}" "$USER@$HOST" "sudo apt-get install -y '$REMOTE_DEB' && rm -f '$REMOTE_DEB'"

# Non-interactive SSH commands run zsh without sourcing rc files, so PATH may
# not include the rootless jailbreak's bin dirs. Resolve the real on-device
# path once (checking common rootless locations, then falling back to a
# filesystem search) instead of guessing, so verification and --run always
# find the binary that was just installed.
REMOTE_LOCATE='
p=$(command -v trackerscan 2>/dev/null)
if [ -z "$p" ]; then
  for c in /var/jb/usr/local/bin/trackerscan /usr/local/bin/trackerscan /var/jb/usr/bin/trackerscan; do
    [ -x "$c" ] && p="$c" && break
  done
fi
if [ -z "$p" ]; then
  p=$(find /var/jb /usr /Applications -maxdepth 6 -name trackerscan -type f -perm -u+x 2>/dev/null | head -1)
fi
if [ -z "$p" ]; then
  echo "trackerscan binary not found on device" >&2
  exit 1
fi
echo "$p"
'

echo "==> Locating installed binary on device"
REMOTE_BIN="$(ssh "${SSH_OPTS[@]}" "$USER@$HOST" "$REMOTE_LOCATE")"
if [ -z "$REMOTE_BIN" ]; then
    echo "error: could not locate trackerscan on the device after install" >&2
    exit 1
fi
echo "==> Found: $REMOTE_BIN"

echo "==> Verifying:"
ssh "${SSH_OPTS[@]}" "$USER@$HOST" "'$REMOTE_BIN' --help" || true

if [ -n "$RUN_BUNDLE_ID" ]; then
    echo "==> Running: trackerscan $RUN_BUNDLE_ID"
    ssh "${SSH_OPTS[@]}" "$USER@$HOST" "'$REMOTE_BIN' '$RUN_BUNDLE_ID'"
fi

echo "==> Done."
