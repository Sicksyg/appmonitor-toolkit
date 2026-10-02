# am_scanner setup and installation

`am_scanner` is the on-device iOS scanner used by AppMonitor. AppMonitor
expects the executable to be installed on the jailbroken phone before it can
load installed-app metadata or analyze an app.

## Install from the source repository

The source, Theos package definition, and standalone deploy helpers are in the
[Sicksyg/am_scanner repository](https://github.com/Sicksyg/am_scanner).
Building the Debian package requires Theos and its iOS rootless toolchain.
From a clone:

```sh
make package
scripts/deploy.sh
```

The deploy script builds the package, transfers it over SSH, and installs it
on the phone. It uses the repository's own `scripts/ios-ssh.sh`, so AppMonitor
does not need to be installed first. The helper requires the SSH alias and
`iproxy` setup described in the [iOS device setup guide](ios-device-setup.md).

To run a verification scan after installing:

```sh
scripts/deploy.sh --run com.example.app
```

Set `AM_SCANNER_IOS_SSH_HOST` to use an SSH alias other than `ios`,
`AM_SCANNER_IOS_SSH_SCRIPT` to select a different helper, or
`AM_SCANNER_IPROXY_PATH` to set the full path to `iproxy`.

The package installs the executable as `/var/jb/usr/local/bin/am_scanner` and
its signature list at `/var/jb/usr/share/am_scanner/signatures.json`.

## AppMonitor integration

AppMonitor connects over its own persistent SSH connection and defaults to
running `am_scanner` from the phone's `PATH`. It adds common rootless and
rootful executable directories to the remote command's `PATH`.

AppMonitor uses:

- `am_scanner --list --json` to retrieve installed app names, bundle IDs,
  versions, and optional PNG icons for the GUI.
- `am_scanner --dump <bundleID>` to retrieve raw app evidence, including
  class names, framework names, plist tokens, permissions, bundle metadata,
  and privacy-manifest information.

The JSON evidence is analyzed on the Mac against AppMonitor's iOS SDK
signatures. The scanner's `--dump` output may include a `runtimeError`; in
that case runtime evidence may be incomplete, and the missing runtime classes
should not be treated as a negative finding.

Use `APPMONITOR_TRACKERSCAN_COMMAND` to configure a different remote
executable. The variable retains its historical name; the default is now
`am_scanner`.

## More information

See the [am_scanner README](https://github.com/Sicksyg/am_scanner) for its
build requirements, command options, JSON schema, and scanner limitations.
