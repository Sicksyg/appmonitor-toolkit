#!/bin/bash
# Connects to the jailbroken iPhone over USB via iproxy, regardless of which
# Wi-Fi network the Mac is on. Starts iproxy if needed and supports an
# app-lifetime OpenSSH ControlMaster for multiplexed commands.
set -euo pipefail

LOCAL_PORT=2222
REMOTE_PORT=22
SSH_HOST="ios"          # matches the "Host ios" alias in ~/.ssh/config
PIDFILE="/tmp/.ios-ssh-iproxy.pid"
LOGFILE="/tmp/.ios-ssh-iproxy.log"
IPROXY="${APPMONITOR_IPROXY_PATH:-iproxy}"
SSH="${APPMONITOR_SSH_PATH:-/usr/bin/ssh}"

if [ -z "${SSH_AUTH_SOCK:-}" ]; then
    SSH_AUTH_SOCK="$(/bin/launchctl getenv SSH_AUTH_SOCK 2>/dev/null || true)"
    export SSH_AUTH_SOCK
fi

is_port_open() {
    nc -z -G 1 127.0.0.1 "$LOCAL_PORT" >/dev/null 2>&1
}

start_iproxy() {
    echo "Starting iproxy $LOCAL_PORT -> $REMOTE_PORT ..." >&2
    "$IPROXY" "$LOCAL_PORT" "$REMOTE_PORT" >"$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
    disown

    # Wait briefly for the tunnel to come up (device must be plugged in and
    # trusted). Bail out with the log contents if it never opens.
    for _ in $(seq 1 20); do
        if is_port_open; then
            return 0
        fi
        sleep 0.3
    done

    echo "Error: iproxy did not open port $LOCAL_PORT in time." >&2
    echo "Is the iPhone plugged in via USB and unlocked/trusted?" >&2
    echo "--- iproxy log ---" >&2
    cat "$LOGFILE" >&2 2>/dev/null || true
    exit 1
}

# Reuse an already-running iproxy for this port; only start a new one if
# nothing is listening yet (handles stale PID files gracefully).
if ! is_port_open; then
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
        # Process is alive but the port isn't open yet (still connecting) -
        # give it a moment before giving up on it and starting a fresh one.
        for _ in $(seq 1 10); do
            is_port_open && break
            sleep 0.3
        done
    fi
    is_port_open || start_iproxy
fi

SSH_OPTIONS=()
if [ -n "${APPMONITOR_IOS_SSH_CONTROL_PATH:-}" ]; then
    SSH_OPTIONS+=(-o "ControlPath=$APPMONITOR_IOS_SSH_CONTROL_PATH")
fi
if [ -n "${APPMONITOR_IOS_SSH_IDENTITY_FILE:-}" ]; then
    SSH_OPTIONS+=(
        -i "$APPMONITOR_IOS_SSH_IDENTITY_FILE"
        -o "IdentitiesOnly=yes"
        -o "UseKeychain=yes"
        -o "AddKeysToAgent=yes"
    )
fi
if [ "${APPMONITOR_IOS_SSH_BATCH:-}" = "1" ]; then
    SSH_OPTIONS+=(-o "BatchMode=yes")
fi
if [ "${APPMONITOR_IOS_SSH_TTY:-}" = "1" ]; then
    SSH_OPTIONS+=(-t)
fi
if [ "${APPMONITOR_IOS_SSH_AUTHORIZE:-}" = "1" ]; then
    exec "$SSH" -T "${SSH_OPTIONS[@]}" \
        -o "BatchMode=no" \
        -o "PreferredAuthentications=publickey,password,keyboard-interactive" \
        "$SSH_HOST" \
        'umask 077; mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && key=$(cat) && (grep -qxF "$key" ~/.ssh/authorized_keys || printf "%s\n" "$key" >> ~/.ssh/authorized_keys)'
fi
if [ "${APPMONITOR_IOS_SSH_MASTER:-}" = "1" ]; then
    SSH_OPTIONS+=(-o "ControlMaster=yes" -o "BatchMode=yes")
    exec "$SSH" ${SSH_OPTIONS[@]+"${SSH_OPTIONS[@]}"} -N "$SSH_HOST"
fi
if [ -n "${APPMONITOR_IOS_SSH_CONTROL_ACTION:-}" ]; then
    SSH_OPTIONS+=(-O "$APPMONITOR_IOS_SSH_CONTROL_ACTION")
    exec "$SSH" ${SSH_OPTIONS[@]+"${SSH_OPTIONS[@]}"} "$SSH_HOST"
fi

exec "$SSH" ${SSH_OPTIONS[@]+"${SSH_OPTIONS[@]}"} "$SSH_HOST" "$@"
