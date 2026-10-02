# iOS device, jailbreak, and SSH setup

This guide covers the iPhone prerequisites shared by AppMonitor and the
standalone `am_scanner` deploy script.

## Device prerequisites

- A jailbroken iPhone with SSH available.
- A USB connection to the Mac; the iPhone must be unlocked and trusted by
  the computer.
- `am_scanner` installed on the iPhone before loading the phone's app list or
  starting scanner analysis. See [am_scanner setup](am-scanner.md).

## AppMonitor SSH setup

AppMonitor embeds its own `scripts/ios-ssh.sh` helper and bundles `iproxy` in
its desktop release. It starts a persistent OpenSSH connection when the app
launches and reuses it for scanner commands.

On first use, AppMonitor opens Terminal to create a dedicated Ed25519 SSH key
and authorize its public key on the iPhone:

1. Verify the iPhone's SSH host-key fingerprint before accepting it.
2. Choose a key passphrase when prompted. macOS stores it in the login
   Keychain through `ssh-add --apple-use-keychain`.
3. Enter the iPhone SSH password when prompted so the public key can be added
   to `~/.ssh/authorized_keys`.

AppMonitor does not collect or save the iPhone password. The private key is
kept in the current user's Application Support directory with owner-only
permissions; it is not bundled or shared between users. To revoke access,
remove the AppMonitor public-key line from the phone's
`~/.ssh/authorized_keys` and delete
`~/Library/Application Support/AppMonitor/ssh/`.

The helper uses the `ios` host from the user's SSH configuration. Configure
the alias for the phone's SSH endpoint and account if it is not already
available. Keep SSH host-key verification enabled.

## SSH access for standalone `am_scanner` deployment

If deploying from the `am_scanner` source repository before AppMonitor is
installed, use its bundled `scripts/ios-ssh.sh`. That helper starts or reuses
`iproxy` on local port 2222 and connects through the configured SSH alias,
which defaults to `ios`.

Configure `~/.ssh/config` with the phone's SSH account and a key that is
authorized on the device. For example:

```sshconfig
Host ios
    HostName 127.0.0.1
    Port 2222
    User mobile
    IdentityFile ~/.ssh/id_ed25519
    IdentitiesOnly yes
```

Use the username and key appropriate for your phone. Verify and accept its
host key on the first connection. The standalone helper needs `iproxy`
available on `PATH`; install it on macOS with:

```sh
brew install libusbmuxd
```

If it is installed outside `PATH`, set `AM_SCANNER_IPROXY_PATH` to the full
path. See [am_scanner setup](am-scanner.md) for deployment instructions.

## Troubleshooting

- Confirm the iPhone is plugged in, unlocked, and trusted.
- Confirm SSH works through the `ios` alias and the host key has been verified.
- For the standalone deploy helper, run `command -v iproxy` and check that
  port 2222 is not occupied by an unrelated service.
- AppMonitor logs SSH connection and setup failures; fix the reported
  connection or authorization issue and restart the app.
