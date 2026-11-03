# Third-Party Notices

AppMonitor includes or redistributes the third-party components listed below.
They remain under their own licenses, which are **not** affected by the license
of AppMonitor's own code (see [LICENSE](LICENSE)). Full license texts that are
short enough to include live in [third_party/licenses/](third_party/licenses/).

> Versions reflect the Homebrew builds bundled in the macOS arm64 release.
> The exact staged versions are written by `scripts/build-release.sh` to
> [third_party/bundled-components.txt](third_party/bundled-components.txt) on every run.
> Update this file whenever bundled tools or dependencies change.

## 1. Icons

**Tabler Icons**: MIT, Copyright (c) 2020-2026 Paweł Kuna
<https://github.com/tabler/tabler-icons>. Text: [Tabler-Icons-MIT.txt](third_party/licenses/Tabler-Icons-MIT.txt).

Used in `frontend/src/assets/images/`: api, api-off, brand-android,
brand-apple, brand-github, chart-dots-2, device-mobile, file-report,
file-settings, player-pause, player-play, plug-connected, plug-connected-x,
report-analytics, sdk, search, send, server-2, settings, tool, world.

Notes:
- The Android, Apple and GitHub logos are trademarks of their owners. They are
  used only to refer to those platforms and imply no endorsement.
- `mobile-icon.svg` carries no Tabler header; its origin should be confirmed.

## 2. Fonts

| Font | License | Notes |
|------|---------|-------|
| PT Serif (Regular/Bold/Italic) | SIL OFL 1.1, Copyright (c) 2010 ParaType Ltd. | Embedded in the binaries for PDF reports. Text: [PT-Serif-OFL.txt](third_party/licenses/PT-Serif-OFL.txt) |
| Go fonts (golang.org/x/image/font/gofont) | BSD-3-Clause, Copyright The Go Authors | Text included in [go-modules.txt](third_party/licenses/go-modules.txt) under `golang.org/x/image` |

## 3. Bundled helper tools and libraries (macOS arm64)

These are copied from Homebrew into the build (`assets/bin`, `assets/lib`),
embedded in the app and CLI, and extracted at runtime. Their install names and
rpaths are rewritten with `install_name_tool` and they are ad-hoc re-signed
(see `scripts/build-release.sh`). No source changes are made.

| Component | Version | License | Source |
|-----------|---------|---------|--------|
| ideviceinstaller | 1.2.0 | GPL-2.0-or-later | <https://github.com/libimobiledevice/ideviceinstaller> |
| libimobiledevice (`ideviceinfo`, `idevice_id`) | 1.4.0 | LGPL-2.1-or-later | <https://github.com/libimobiledevice/libimobiledevice> |
| libusbmuxd (`iproxy`) | 2.1.1 | GPL-2.0-or-later AND LGPL-2.1-or-later | <https://github.com/libimobiledevice/libusbmuxd> |
| libplist | 2.7.0 | LGPL-2.1-or-later | <https://github.com/libimobiledevice/libplist> |
| libimobiledevice-glue | 1.3.2 | LGPL-2.1-or-later | <https://github.com/libimobiledevice/libimobiledevice-glue> |
| ldid | 2.1.5 | AGPL-3.0-or-later | <https://git.saurik.com/ldid.git> (Homebrew formula `ldid`) |
| ipatool | 2.7.0 | MIT, Copyright (c) 2021 Majd Alfhaily ([text](third_party/licenses/ipatool-MIT.txt)) | <https://github.com/majd/ipatool> |
| OpenSSL (libssl, libcrypto) | 3.6.x | Apache-2.0 | <https://github.com/openssl/openssl> |
| libzip | 1.11.4 | BSD-3-Clause | <https://libzip.org> |
| xz (liblzma) | 5.8.4 | 0BSD AND GPL-2.0-or-later | <https://github.com/tukaani-project/xz> |
| zstd | 1.5.7 | BSD-3-Clause OR GPL-2.0-only (dual) | <https://github.com/facebook/zstd> |

License texts: [GPL-2.0](https://www.gnu.org/licenses/old-licenses/gpl-2.0.txt),
[LGPL-2.1](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.txt),
[AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.txt),
[Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0.txt).

**Source availability.** The corresponding source for each component and
version above is available from the upstream project and from the Homebrew
formula (`brew edit <formula>`, or <https://github.com/Homebrew/homebrew-core>).
On request via an issue at
<https://github.com/Sicksyg/appmonitor-toolkit/issues> the maintainer will
supply the exact source archives. The libraries are dynamically linked
(`.dylib`) and can be replaced by compatible builds; for the LGPL libraries the
dylibs can be swapped in the extracted lib directory.

## 4. Frida

- **frida-go** v1.1.0: ISC, Copyright (c) 2025 frida-go contributors. Text in
  [go-modules.txt](third_party/licenses/go-modules.txt). It links the Frida
  core library (wxWindows Library Licence 3.1, <https://frida.re>).
- **frida-objc-bridge** 8.0.5: LGPL-2.0 WITH WxWindows-exception-3.1, bundled
  into `assets/frida/_agent.js`. Source: <https://github.com/frida/frida-objc-bridge>.

Confirm Frida's current license terms at <https://frida.re/docs/licensing/>
before each release.

## 5. Go modules

Compiled-in modules and their license texts are in
[go-modules.txt](third_party/licenses/go-modules.txt): goquery, cascadia,
barcode, uax29, go-async, frida-go, uuid, hhrutter/{lzw,pkcs7,tiff},
johnfercher/{go-tree,maroto}, leaanthony/{go-ansi-parser,slicer,u},
go-runewidth, pdfcpu, gofpdf, pkg/errors, uniseg, Wails v2,
golang.org/x/{crypto,image,net,text}, yaml.v2, and howett.net/plist. Regenerate
the file after dependency changes.

## 6. Frontend packages

- Vue 3.5.30, MIT: [vue-MIT.txt](third_party/licenses/vue-MIT.txt)
- Vue Router 4.6.4, MIT: [vue-router-MIT.txt](third_party/licenses/vue-router-MIT.txt)

Their transitive dependencies are bundled by Vite; check `npm ls --prod` and
`frontend/dist` before release for additional notices.

## 7. Data and logos

- `assets/*.json` (SDK, permission and signature lists): provenance and license
  to be confirmed by the maintainer before release.
- `static/erc_logo.png` and `static/ucph_logo.jpg` are logos of their
  respective institutions, used to identify the research institutions (confirm brand rules);
  they are **not** covered by the project license.
