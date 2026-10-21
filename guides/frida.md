# Frida setup and experimental analysis

Frida is available only as an optional standalone CLI analysis pass. The GUI
and the CLI's default iOS workflow use `am_scanner` without Frida.

## End-user setup

To run the optional Frida pass, use `--frida` with the standalone CLI. This
requires a `frida-server` on the jailbroken phone with a version compatible
with AppMonitor's Frida runtime. If Frida fails, the CLI reports the error.
See the [Frida iOS setup documentation](https://frida.re/docs/ios/) for
device-side installation guidance.

## Building AppMonitor with Frida

Contributors building AppMonitor from source need the Frida development kit
because `frida-go` uses CGO. The project has previously verified
`frida-core-devkit-17.2.17-macos-arm64.tar.xz` on Apple Silicon. Download the
matching macOS archive from the [Frida releases page](https://github.com/frida/frida/releases/)
and install its header and static library:

```sh
tar -xf frida-core-devkit-17.2.17-macos-arm64.tar.xz
sudo cp frida-core.h /usr/local/include/
sudo cp libfrida-core.a /usr/local/lib/
```

Use the `macos-arm64` archive on Apple Silicon or the corresponding macOS
archive for Intel Macs. Do not use the `ios-arm64` archive to build the macOS
application. The devkit is only needed to compile AppMonitor; users of a
prebuilt release do not need it.

Frida 17 no longer bundles the Objective-C bridges with the Frida GumJS
runtime. AppMonitor's custom TypeScript agent is kept under `assets/frida/`
and compiled as part of runtime setup. See the
[Frida 17 bridge documentation](https://frida.re/docs/bridges/#manually-compiling-using-frida-compile)
and the [development guide](development.md).

## CLI opt-in

The standalone CLI runs `am_scanner` only by default. Add `--frida` to run
the Frida pass after the scanner. See [CLI usage](cli.md).

## Troubleshooting

- Confirm `frida-server` is running on the device and its version is compatible
  with AppMonitor's Frida runtime.
- Confirm the app is installed and can launch on the device.
- Check AppMonitor's log for setup, spawn, attach, and agent compilation
  errors.
- Omit `--frida` to run the scanner-only CLI workflow.
