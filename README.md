# AppMonitor: A toolkit for monitoring third-party tracking across iOS and Android apps
## Overview
AppMonitor is an application monitoring tool that provides real-time insights into application infrastructure and tracking capabilities. At the heart of the tool is the analysis of Software Development Kits (SDKs) - third party infrastructures that enable essential functionality and tracking.


## Requirements
- **Operating System**: macOS or Linux
- **Device**: Jailbroken iPhone
- **Apple ID**: Recommended to use a spare account to avoid issues with your primary account
- **Cable**: USB-A to Lightning cable (USB-C to Lightning can be unstable)

## Methodology
This tool uses two analytical approaches:
- **Static Analysis**: Identification of known SDKs used by an app
- **Permission Analysis**: Identification of permissions used by the app
- **Dynamic Analysis**: Identification of domains used by an app

For detailed methodology, see [Monitoring infrastructural power: Methodological challenges in studying mobile infrastructures for datafication](https://example.com) by Lomborg, S., Sick Svendsen, K., Flensburg, S., & Sophus Lai, S. (2024)

## Citation
If you use this software, please cite the provided research paper.

## Tech Stack

### Desktop Framework
- [Wails](https://wails.io/) - Lightweight Go-based desktop app framework that bridges the Go backend and the web frontend, enabling native macOS/Linux desktop packaging without Electron.

### Backend (Go)
- **Go** - Core backend language handling analysis logic, device communication, and data processing.
  - `analysis/` - SDK and permission detection engine for both iOS and Android apps
  - `android/` - Android APK parsing and analysis
  - `itunes/` - iOS IPA unpacking and App Store metadata retrieval
  - `report/` - Report generation from analysis results
  - `helpers/` - Shared utility functions
  - `models/` - Shared data models used across the application

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