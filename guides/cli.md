# Standalone CLI usage

AppMonitor's standalone command-line runner processes one app or a CSV list
without opening the GUI. Its default iOS analysis uses `am_scanner`; Android
analysis uses the Android CLI workflow.

## Requirements

- Build or use the AppMonitor CLI executable.
- For iOS: connect a jailbroken iPhone over USB, install `am_scanner`, and
  configure SSH as described in the [device setup guide](ios-device-setup.md)
  and [scanner guide](am-scanner.md).
- For iOS download/install: configure AppMonitor's Apple ID settings as
  applicable. If automatic download is unavailable, use interactive manual
  download.

## Examples

From the repository root:

```sh
go run ./cmd -bundle com.example.app
go run ./cmd -csv apps.csv -platform ios
go run ./cmd -bundle com.example.android -platform android -auto
```

CSV files may include columns for bundle ID, app name, and platform. Without
a bundle or CSV argument, the CLI prompts for a target.

## Options

| Option | Description |
| --- | --- |
| `-bundle`, `-b` | Analyze one bundle ID. |
| `-csv`, `-c` | Read app targets from a CSV file. |
| `-platform`, `-p` | Target platform, `ios` or `android` (default `ios`). |
| `-udid`, `-u` | iOS device UDID; auto-detected if omitted. |
| `-output`, `-o` | Custom output directory for analysis data/database. |
| `-config`, `-f` | Custom path to `settings.json`. |
| `-manual-download`, `-m` | Open the App Store link on the phone over SSH with `uiopen`, then wait for manual installation. |
| `--frida` | Also run Frida analysis after `am_scanner` (iOS only). |
| `-uninstall-after`, `-x` | Uninstall each analyzed iOS app after processing. |
| `-auto`, `-a` | Non-interactive mode. |
| `-no-interactive`, `-n` | Alias for non-interactive mode. |

The CLI checks whether an iOS target is installed. If it is not, it attempts
automatic download/install when configured; otherwise it uses the manual
download flow. Manual download requires interactive mode.

## Output

For iOS analysis, the CLI saves the complete `am_scanner` JSON under
`ios/trackerscan/` and saves class names as a newline-separated text file
under `ios/classlogs/`. It also updates `app_database.json`.

The scanner command and SSH helper can be overridden with
`APPMONITOR_TRACKERSCAN_COMMAND` and `APPMONITOR_IOS_SSH_SCRIPT`. Although the
first variable retains the historical name, the default scanner executable
is `am_scanner`.

Use `-h` or `--help` for the current command-line help.
