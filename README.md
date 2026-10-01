# AppMonitor: A toolkit for monitoring third-party tracking across iOS and Android apps
## Overview
AppMonitor is an application monitoring tool that provides real-time insights into application infrastructure and tracking capabilities. At the heart of the tool is the analysis of Software Development Kits (SDKs) - third party infrastructures that enable essential functionality and tracking.

## Application Screenshot
![AppMonitor Screenshot](AppMonitor_screenshot.png)

## Requirements
- **Operating System**: macOS
- **Device**: Jailbroken iPhone
- **Apple ID**: Highly recommended to use a spare account to avoid issues with your primary account
- **Cable**: USB-A to Lightning cable (USB-C to Lightning can be unstable)

### iOS Runtime Setup
AppMonitor runs `am_scanner` on the jailbroken iPhone over USB SSH. For a
newly installed or updated app, it runs `am_scanner` first and then the
optional Frida analysis. Install `am_scanner` on the phone before analyzing
apps. Frida is only required for the Frida pass; install and start a compatible
`frida-server` on the device if you want that pass. Connect the device to the
Mac by USB before starting an analysis.

If the selected app is already installed, AppMonitor asks whether to install the
latest version, analyze the installed app with trackerscan only, or cancel.
Choosing the installed app skips download and installation and does not start
Frida.

The distributed AppMonitor application includes its required macOS-side tools. End users do not need to install the Frida development kit, Go, or Frida headers and libraries on their Mac.

### am_scanner and compatibility installation
`am_scanner` is the on-device scanner forked from
[TrackerControl/trackerscan-ios](https://github.com/TrackerControl/trackerscan-ios).
AppMonitor invokes it over SSH using `--list --json` to load installed app
names, bundle IDs, versions, and optional icons, and `--dump <bundleID>` to
collect class names, framework names, plist tokens, permissions, bundle
metadata, and privacy-manifest evidence. AppMonitor decodes the JSON and
matches the evidence against its local iOS SDK signatures.

Install the `am_scanner` package on the jailbroken phone first. Its source
repository includes a standalone `scripts/deploy.sh` and `scripts/ios-ssh.sh`,
so deployment does not require AppMonitor to be installed first. The deploy
helper uses an SSH host alias (default `ios`) and starts or reuses `iproxy`.
On macOS, install `iproxy` with `brew install libusbmuxd`; configure the SSH
alias to use the USB-forwarded endpoint (`127.0.0.1`, port `2222`) and the
device's SSH user and credentials. The helper supports
`AM_SCANNER_IOS_SSH_HOST`, `AM_SCANNER_IOS_SSH_SCRIPT`, and
`AM_SCANNER_IPROXY_PATH` overrides. See the
[am_scanner repository](https://github.com/Sicksyg/am_scanner) for build and
deployment instructions.

When AppMonitor connects, it configures its own persistent OpenSSH
multiplexing connection and reuses it for scanner commands. On first use, it
opens Terminal to create and authorize an AppMonitor-specific SSH key. The
first-time key authorization and host-key verification steps are described
below.

On first use, AppMonitor opens Terminal to create a dedicated Ed25519 key for
the current macOS user and authorize its public key on the phone. Choose a key
passphrase when prompted; `ssh-add --apple-use-keychain` stores it in the
macOS Keychain. OpenSSH then asks for the phone's SSH password once to add the
public key to `authorized_keys`; AppMonitor does not collect or store that
password. Later launches use the key through the macOS SSH agent. The private
key remains in the user's Application Support directory with owner-only
permissions; no private key is bundled or shared between users. Keep SSH host
key verification enabled and verify the phone's host-key fingerprint when
OpenSSH prompts. To revoke access, remove the AppMonitor public-key line from
the phone's `~/.ssh/authorized_keys` and delete the local
`~/Library/Application Support/AppMonitor/ssh/` key directory.

For remote `am_scanner` execution, AppMonitor adds the common rootless and
rootful jailbreak executable directories to the non-interactive SSH command's
`PATH`, including `/var/jb/usr/local/bin` where the provided Theos package
installs `am_scanner`.

For apps whose selected iTunes lookup declares iOS 16.x through 18.x as the
minimum, AppMonitor prepares the downloaded IPA in Go, then installs it with
the bundled `ideviceinstaller`. If installation fails and the IPA contains
incompatible app extensions, AppMonitor prunes those extensions and retries
once. Apps requiring iOS 19 or later are reported as unsupported. IPA
preparation uses macOS `ditto` for archive handling and `ldid` only when a
Mach-O minimum-OS field must be changed and re-signed. See
[the compatibility preparation notes](./scripts/prune_install_README.md).

The AppMonitor SSH helper script is embedded in the application. Override its
path or the remote scanner executable with `APPMONITOR_IOS_SSH_SCRIPT` and
`APPMONITOR_TRACKERSCAN_COMMAND`, respectively. The default remote executable
is `am_scanner`. `APPMONITOR_DITTO_PATH` can
select a non-default `ditto` executable, `APPMONITOR_LDID_PATH` can select an
`ldid` executable, and `APPMONITOR_COMPATIBLE_EXTENSION_POINTS` can provide a
whitespace-separated extension-point allowlist.
The iOS workflow and `am_scanner` integration live in `ios/ios.go`; the
retained Frida implementation lives separately in `ios/frida.go`. Scanner JSON
evidence is saved beside the iOS reports, and its class/framework evidence is
matched against AppMonitor's existing iOS SDK signatures. A scanner failure is
shown as a warning and does not prevent the Frida pass from running.

The standalone CLI defaults to `am_scanner`-only analysis for iOS. It saves
the complete scanner JSON under `ios/trackerscan/` and writes class names,
one per line, under `ios/classlogs/`. Use `--frida` to additionally run the
Frida analysis. When `--manual-download` is used, the CLI opens the App Store
link on the phone over SSH with `uiopen`. The CLI accepts the same
`APPMONITOR_IOS_SSH_SCRIPT` and `APPMONITOR_TRACKERSCAN_COMMAND` overrides.

## Caveats
**The tool is a proof of concept and may have bugs or incomplete features.** If something does not work as expected, please report it in the issues section or make a pull request.
- As of August, Apple has (again) changed the authentication for itunes, meaning that the purchase (optaining licenses), and download of apps from the App Store is currently broken. We are working on a solution, but it is not yet implemented.

### Jailbreak Issues
Some apps will not run on jailbroken devices due to jailbreak detection mechanisms. In such cases, the tool will not be able to perform dynamic analysis. We are working on a solution to bypass jailbreak detection, but it is not yet implemented and has been deprioritized due to the complexity of the task. If you encounter an app that does not run on a jailbroken device, please report it in the issues section.

## Methodology
This tool uses two analytical approaches:
- **Static Analysis**: Identification of known SDKs used by an app
- **Permission Analysis**: Identification of permissions used by the app
- _**Dynamic Analysis**: Identification of domains used by an app (In progress)_

For detailed methodology, see [Monitoring infrastructural power: Methodological challenges in studying mobile infrastructures for datafication](https://example.com) by Lomborg, S., Sick Svendsen, K., Flensburg, S., & Sophus Lai, S. (2024)

## Citation
If you use this software, please cite the provided research paper.



## Tech Stack

### Desktop Framework
- [Wails](https://wails.io/) - Lightweight Go-based desktop app framework that bridges the Go backend and the web frontend, enabling native macOS/Linux desktop packaging without Electron.

### Backend (Go)
- **Go** - Core backend language handling analysis logic, device communication, and data processing.
  - `iOS/` - SDK and permission detection engine for both iOS apps (using Frida)
  - `android/` - Android APK parsing and analysis
  - `appstores/` - App Store and Play Store API interactions for app metadata retrieval - custom implementation of the iTunes Search API and Play Store scraping
  - `report/` - Report generation from analysis results
  - `helpers/` - Shared utility functions
  - `models/` - Shared data models used across the application
  - `cmd/` - Standalone command-line tools for mass analysis and testing

### Frontend
- [Vue 3](https://vuejs.org/) - Component-based UI framework powering the four main views: iOS, Android, Utilities, and Settings
- [Vite](https://vite.dev/) - Fast frontend build tool and dev server

### Dynamic Analysis
- [Frida](https://frida.re/) - Dynamic instrumentation toolkit used for runtime analysis of iOS apps on jailbroken devices
  - A custom TypeScript agent (`frida/agent/`) is compiled at runtime using `frida-compile` and injected into target processes to capture network traffic and domain activity

## Dev Notes
### Program structure
Flowchart

# Debug and FAQ
## Frida Script:
From frida 17.0, the FridaGumJS runtime (Frida injects QuickJS into a running process) is no longer bundled with the objective-c bridges (see more on [Frida Bridges](https://frida.re/docs/bridges/#manually-compiling-using-frida-compile)). This means that when we are creating our own API, we have to install and compile the runtime. The frida agent is created in the "frida" directory and is compiled at runtime.

## Frida errors on iOS:
If frida error is encountered in the iOS analysis without explanation, make sure that the device is running the frida-server (deamon). If not, reinstall it using the repo as explained in [Frida Docs](https://frida.re/docs/ios/).


## Building the project
### Prerequisites
- To build the project, you need to have the following installed:
  - [Go](https://go.dev/doc/install) (the version specified in `go.mod`)
  - [Node.js](https://nodejs.org/en/download/) (version 18 or higher)
  - [Wails](https://wails.io/docs/gettingstarted/installation) (for building the desktop application)

### Frida Development Kit
Contributors building AppMonitor from source need the Frida development kit because `frida-go` uses CGO at build time. The latest verified working devkit for this project on Apple Silicon is `frida-core-devkit-17.2.17-macos-arm64.tar.xz`. Download it from the [Frida releases page](https://github.com/frida/frida/releases/), then extract it and install its header and static library:


```sh
tar -xf frida-core-devkit-17.2.17-macos-arm64.tar.xz
sudo cp frida-core.h /usr/local/include/
sudo cp libfrida-core.a /usr/local/lib/
```

Use `macos-arm64` on Apple Silicon or the corresponding macOS archive for an Intel Mac. Do not use `frida-core-devkit-<version>-ios-arm64.tar.xz` to build AppMonitor on macOS: that archive targets iOS, whereas the Go application runs on macOS.

The development kit is required only to compile AppMonitor. It is not required by end users running a prebuilt release. Keep downloaded devkit archives out of source control.

### Build

```sh
wails build
```

#### Homebrew helper tools and release build

The release script stages AppMonitor's macOS helper tools and their non-system
dylib dependency closure from Homebrew before building. Install the required
formulae if they are not already available:

```sh
brew install libimobiledevice libusbmuxd ipatool ldid
```

Then run:

```sh
scripts/build-release.sh
```

The script resolves `ideviceinstaller`, `idevice_id`, and `ideviceinfo` from
`libimobiledevice`, plus `iproxy` from `libusbmuxd`, and `ipatool` and `ldid`
from their own formulae. It recursively lists required non-system dylibs first, copies the tools and
libraries into `assets/bin/darwin-<arch>/` and `assets/lib/darwin-<arch>/`,
then rewrites Homebrew install names and runtime paths for AppMonitor's
extracted layout. Finally, it ad-hoc signs each copied Mach-O after patching
and runs the Wails and CLI builds.
The bundled `iproxy` path is passed to the SSH helper when AppMonitor launches
the USB tunnel, so Finder-launched builds do not depend on Homebrew being in
the app's `PATH`. When running `scripts/ios-ssh.sh` manually, it still falls
back to `iproxy` found on `PATH`.
During restaging it removes each previous destination before copying its
replacement, so files left read-only or root-owned by an earlier privileged
build can be refreshed as long as the `assets/bin` and `assets/lib`
architecture directories remain writable.

To stage and sign the tools without building or launching the app:

```sh
scripts/build-release.sh --stage-only
```

The script stages only the current Homebrew architecture (`arm64` on Apple
Silicon or `amd64` on Intel); it does not make universal binaries. Staging
requires the Xcode command-line tools, including `otool`, `install_name_tool`,
`lipo`, and `codesign`. The ad-hoc signatures are for the rewritten local
tools, not Developer ID distribution signatures. `APPMONITOR_ASSET_ROOT` can
be set to a temporary asset directory when validating the staging phase.
Normal build mode also clears the generated AppMonitor report directory and
cached `bin`, `frida`, and `lib` folders under Application Support before
building.

If Wails reports `permission denied` for `build/bin` or macOS cannot open the
generated app with `NSCocoaErrorDomain Code=257`, check whether the output was
created by running a previous build with `sudo`. Restore ownership, then rerun
the build as your normal user (do not run Homebrew, Wails, or Go with `sudo`):

```sh
sudo chown -R "$(id -un)" "$PWD/build/bin"
```

The release script checks whether `build/bin` is writable before staging tools
and prints this repair command when it detects the problem. It also checks
that the architecture-specific asset directories are writable before
restaging; if those directories themselves are not writable, repair ownership
with:

```sh
sudo chown -R "$(id -un)" assets/bin assets/lib
```


## License

### Main Licence
This repository is licensed under Creative Commons Attribution 4.0 International CC BY 4.0.

### Additional Licences
This repository and its software components are a part of the Datafied Living Project and has received funding from the European Research Council (ERC) under
the European Union’s Horizon 2020 research and innovation programme [Datafied Living at The University of Copenhagen](https://datafiedliving.ku.dk/) (Grant agreement ID: 947735) and the Horizon ERC 2024 POC [AppMonitor](https://cordis.europa.eu/project/id/101189401) (Grant agreement ID: 101189401)

![image](https://github.com/user-attachments/assets/fe732ac6-0468-4421-a7a6-62e7b24c1633)
![image](https://designguide.ku.dk/download/co-branding/ku_co_uk_h.jpg)


# Use
https://github.com/tabler/tabler-icons