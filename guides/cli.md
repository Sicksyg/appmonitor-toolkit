# Standalone CLI usage

AppMonitor's command-line runner analyzes one app or thousands without opening
the GUI. It shares the GUI's analysis pipeline: apps are downloaded with
`ipatool`, patched for the device's iOS version (pruned and demoted through
`ios/ipa.go`), installed, scanned with `am_scanner`, matched against the SDK
signatures and written to a PDF report and `app_database.json`. Android apps are
analyzed through the Exodus API.

## Requirements

- Build or use the AppMonitor CLI executable.
- iOS: a jailbroken iPhone over USB with `am_scanner` installed and SSH
  configured ([device setup](ios-device-setup.md), [scanner](am-scanner.md)).
- iOS downloads: sign in to ipatool once with `ipatool auth login` in a
  terminal (needs your 2FA code). The session is kept in the keychain.
- Android: an Exodus API key in `settings.json`.

## Commands

```sh
appmonitor [analyze] [flags] [bundle-id ...]   # default command
appmonitor report [flags] [bundle-id ...]      # regenerate PDFs from the database
appmonitor uninstall [flags] [bundle-id ...]   # remove apps from the iPhone
```

Earlier invocations such as `-csv apps.csv -a` still work.

## Analyzing in bulk

Targets come from positional bundle IDs, `-b`, and/or an input file; they are
merged and de-duplicated.

```sh
appmonitor com.example.one com.example.two
appmonitor -i apps.csv -a -retries 3 -export results.csv
cat apps.txt | appmonitor -i - -p android
appmonitor ios:com.example.one android:com.example.two
appmonitor -dry-run -i apps.json
```

Input files (`-i`, alias `-csv`/`-c`; use `-` for stdin):

- `.txt`: one bundle ID per line; `#` starts a comment.
- `.csv`: comma, semicolon or tab separated, with or without a header
  (`bundle_id`, `name`, `platform`).
- `.json`: `["id", ...]`, `[{"bundleId": "...", "name": "...", "platform": "ios"}]`
  or `{"apps": [...]}`.

A `ios:` or `android:` prefix (or a platform column) sets the platform per app;
otherwise `-platform` applies. SSH is only started when an iOS target is present.
Reading the list from stdin turns off all prompts.

### What each app goes through

1. Installed apps are analyzed in place. Otherwise the app is downloaded,
   patched for the device and installed (`-manual-download` opens the App Store
   instead).
2. Failures are retried automatically (`-retries`, default 2) with exponential
   backoff starting at `-retry-delay` (default 5s, capped at 2 min). Errors that
   retrying cannot fix are not retried: not signed in to ipatool, the App Store
   install conflict (delete the app and run again), and an iOS version that
   is too old for the app.
3. A PDF report is written to `<output>/ios/reports` or `<output>/android/reports`
   (skip with `-no-report`) and the database is updated.
4. In interactive mode only, a failed app (after its retries) asks to
   retry, skip or quit. Use `-a` for unattended runs.

Progress is printed live as `[i/N]` with the pipeline stages, and a summary
table (status, SDKs, permissions, attempts, time, error) is printed at the end.
Ctrl+C stops after the current step, skips the remaining apps, and still prints
the summary and writes `-export`.

`-export results.json|results.csv` writes the per-app results of the run.
`-dry-run` validates and de-duplicates the list, checks the environment (device,
SSH helper, ipatool session, Exodus key) and prints what would happen without
downloading, installing or analyzing anything.

The exit status is `0` when everything succeeded, `1` if any app failed or the
run was interrupted, and `2` for usage errors.

## Regenerating reports

`report` rebuilds PDFs from `app_database.json` and the saved `am_scanner` JSON,
so report design changes apply to old analyses without a device.

```sh
appmonitor report -all                    # latest version of every app
appmonitor report -all -all-versions -j 8
appmonitor report -all -db /path/to/app_database.json -out ./pdfs   # a specific database
appmonitor report com.example.one -out ./pdfs
```

New paths are written back to the database.

## Options

| Option | Description |
| --- | --- |
| `-input`, `-i`, `-csv`, `-c` | App list file (`.txt`, `.csv`, `.json`) or `-` for stdin. |
| `-bundle`, `-b` | One bundle ID. |
| `-platform`, `-p` | Default platform, `ios` or `android` (default `ios`). |
| `-udid`, `-u` | iOS device UDID; auto-detected if omitted. |
| `-retries` | Automatic retries per app (default 2). |
| `-retry-delay` | First retry delay, doubled each time (default `5s`). |
| `-export` | Write run results to `.json` or `.csv`. |
| `-skip-done` | Skip apps already analyzed today, according to the database. |
| `-skip-within` | Skip apps analyzed within a period, e.g. `72h`. |
| `-dry-run` | Validate and show the plan only. |
| `-no-report` | Skip PDF generation. |
| `-verbose`, `-v` | Print diagnostic logs to the terminal. They are always saved to `logs/cli.log` in the AppMonitor config folder. |
| `-output`, `-o` | Output directory for the database and reports. |
| `-config`, `-f` | Path to `settings.json`. |
| `-manual-download`, `-m` | Open the App Store page on the phone and wait for you to install. |
| `--frida` | Also run Frida analysis after `am_scanner` (iOS only). |
| `-uninstall-after`, `-x` | Uninstall each analyzed iOS app afterwards. |
| `-auto`, `-a`, `-no-interactive`, `-n` | Never prompt. |

`report` options: `-all`, `-bundle`/`-b` (repeatable or comma-separated),
`-all-versions`, `-out`, `-j` (parallel reports, default 4), `-output`, `-config`.

## Output

For iOS the CLI saves the complete `am_scanner` JSON under `ios/am_scanner/` and
class names under `ios/classlogs/`. The scanner command and SSH helper can be
overridden with `APPMONITOR_TRACKERSCAN_COMMAND` and `APPMONITOR_IOS_SSH_SCRIPT`
(the first variable keeps its historical name; the default scanner is
`am_scanner`).

Run `appmonitor -h` for the built-in help.

## Bulk uninstall

```sh
appmonitor uninstall -i apps.csv            # remove every installed app listed in the file
appmonitor uninstall -dry-run -i apps.csv   # only show what would be removed
appmonitor uninstall com.example.app        # positional bundle IDs work too
```

The list uses the same formats as `analyze` (CSV/TSV/JSON, or `-` for stdin; first column is the bundle ID).
Apps that are not installed are skipped. There is no confirmation prompt, so use `-dry-run` first.
The exit status is 1 if any uninstall fails. Flags: `-i/-csv`, `-udid`, `-dry-run`, `-f/-config`.
