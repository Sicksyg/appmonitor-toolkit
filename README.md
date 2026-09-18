# AppMonitor: A toolkit for monitoring third-party tracking across iOS and Android apps
## Overview
AppMonitor is an application monitoring tool that provides real-time insights into application infrastructure and tracking capabilities. At the heart of the tool is the analysis of Software Development Kits (SDKs) - third party infrastructures that enable essential functionality and tracking.


## Requirements
- **Operating System**: macOS
- **Device**: Jailbroken iPhone
- **Apple ID**: Highly recommended to use a spare account to avoid issues with your primary account
- **Cable**: USB-A to Lightning cable (USB-C to Lightning can be unstable)

### iOS Runtime Setup
AppMonitor uses Frida to inspect a running iOS app. Before running an iOS analysis, install and start a `frida-server` on the jailbroken device. The server version must be compatible with the Frida version used by AppMonitor. Connect the device to the Mac by USB before starting an analysis.

The distributed AppMonitor application includes its required macOS-side tools. End users do not need to install the Frida development kit, Go, or Frida headers and libraries on their Mac.

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