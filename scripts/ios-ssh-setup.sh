#!/bin/bash
set -euo pipefail

IDENTITY_FILE="${APPMONITOR_IOS_SSH_IDENTITY_FILE:?AppMonitor SSH identity path is not set}"
SSH_SCRIPT="${APPMONITOR_IOS_SSH_SCRIPT:?AppMonitor SSH script path is not set}"
SSH_DIR="$(dirname "$IDENTITY_FILE")"

pause_on_error() {
    status=$?
    if [ "$status" -ne 0 ]; then
        echo
        echo "AppMonitor SSH setup failed (exit $status)."
        read -r -p "Press Enter to close this Terminal window. "
    fi
}
trap pause_on_error EXIT

mkdir -p "$SSH_DIR"
chmod 700 "$SSH_DIR"

if [ ! -e "$IDENTITY_FILE" ] && [ ! -e "$IDENTITY_FILE.pub" ]; then
    echo "Creating an AppMonitor-specific Ed25519 key."
    echo "Choose a passphrase when prompted; it will be stored in your macOS Keychain."
    /usr/bin/ssh-keygen -t ed25519 -a 64 -C "AppMonitor" -f "$IDENTITY_FILE"
elif [ ! -f "$IDENTITY_FILE" ]; then
    echo "The AppMonitor SSH private key is missing at $IDENTITY_FILE." >&2
    exit 1
elif [ ! -f "$IDENTITY_FILE.pub" ]; then
    echo "Reconstructing the public key from the existing private key."
    /usr/bin/ssh-keygen -y -f "$IDENTITY_FILE" > "$IDENTITY_FILE.pub"
fi

chmod 600 "$IDENTITY_FILE"
chmod 644 "$IDENTITY_FILE.pub"

if [ -z "${SSH_AUTH_SOCK:-}" ]; then
    SSH_AUTH_SOCK="$(/bin/launchctl getenv SSH_AUTH_SOCK 2>/dev/null || true)"
    export SSH_AUTH_SOCK
fi
if [ -z "${SSH_AUTH_SOCK:-}" ]; then
    echo "No macOS SSH agent is available. Ensure the login keychain is unlocked and try again." >&2
    exit 1
fi

echo
echo "Adding the AppMonitor key to your macOS Keychain and SSH agent."
/usr/bin/ssh-add --apple-use-keychain "$IDENTITY_FILE"

echo
echo "Authorizing the public key on the iPhone."
echo "If asked, verify the iPhone host key and enter the device's SSH password."
echo "The password is not saved; only the public key is added to authorized_keys."
APPMONITOR_IOS_SSH_AUTHORIZE=1 "$SSH_SCRIPT" < "$IDENTITY_FILE.pub"

echo
echo "AppMonitor SSH setup completed. Future launches use this key."
read -r -p "Press Enter to close this Terminal window. "
trap - EXIT
