#!/usr/bin/env zsh

set -euo pipefail

cd "${0:A:h:h}"

report_dir="$HOME/Documents/AppMonitor Output"
app_support_dir="$HOME/Library/Application Support/AppMonitor"

rm -rf "$report_dir"
if [[ -d "$app_support_dir" ]]; then
	find "$app_support_dir" -mindepth 1 -maxdepth 1 ! -name settings.json -exec rm -rf {} +
fi

wails build -clean
go build -o build/appmonitor ./cmd
open build/bin/AppMonitor.app
