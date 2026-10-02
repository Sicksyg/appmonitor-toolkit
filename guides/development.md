# Building AppMonitor from source

This guide is for contributors building the desktop application or CLI. End
users running a packaged desktop release do not need Go, Node.js, Wails, or
the Frida development kit.

## Prerequisites

- [Go](https://go.dev/doc/install), using the version specified in `go.mod`.
- [Node.js](https://nodejs.org/en/download/), version 18 or higher.
- [Wails](https://wails.io/docs/gettingstarted/installation) to build the
  desktop application.
- The Frida development kit for the platform when building the CGO-dependent
  iOS Frida package. See the [Frida guide](frida.md).

## Build

Build the desktop application with:

```sh
wails build
```

Build the standalone CLI with:

```sh
go build -o build/appmonitor ./cmd
```

## Release helper tools

The release script stages AppMonitor's macOS helper tools and their
non-system dylib dependencies from Homebrew. Install the required formulae:

```sh
brew install libimobiledevice libusbmuxd ipatool ldid
```

Then run:

```sh
scripts/build-release.sh
```

The script resolves `ideviceinstaller`, `idevice_id`, and `ideviceinfo` from
`libimobiledevice`, `iproxy` from `libusbmuxd`, and `ipatool` and `ldid` from
their own formulae. It stages tools and libraries into the architecture
folders under `assets/`, rewrites install names and runtime paths, ad-hoc
signs the staged binaries, then runs the Wails and CLI builds. The bundled
`iproxy` lets Finder-launched builds create the USB tunnel without depending
on Homebrew being on the app's `PATH`.

To stage and sign tools without building or launching the app:

```sh
scripts/build-release.sh --stage-only
```

The script stages only the current Homebrew architecture (`arm64` on Apple
Silicon or `amd64` on Intel Macs); it does not create universal binaries.
Staging requires Xcode command-line tools, including `otool`,
`install_name_tool`, `lipo`, and `codesign`.

The release script clears generated AppMonitor reports and cached `bin`,
`frida`, and `lib` folders under Application Support in normal build mode.
`APPMONITOR_ASSET_ROOT` can select a temporary asset directory for validating
the staging phase.

## Build permission troubleshooting

Avoid running Homebrew, Wails, or Go with `sudo`. If a prior privileged build
left generated output unwritable, restore ownership as your normal user:

```sh
sudo chown -R "$(id -un)" "$PWD/build/bin"
sudo chown -R "$(id -un)" assets/bin assets/lib
```
