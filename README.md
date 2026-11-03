# Project Poros

A VPN and proxy client, shipped as **Poros**. It is being rebuilt as a portable Windows app on Tauri 2, Vue 3 and a Go core, and has no release yet. The earlier Windows client on Qt is no longer developed.

## Based on Throne

Project Poros is a modified version of [Throne](https://github.com/throneproj/Throne), forked from tag `1.4.0-beta.4` in October 2026, and is licensed under GPL-3.0: see [LICENSE](LICENSE). It builds on [sing-box](https://github.com/SagerNet/sing-box) and [Xray-core](https://github.com/XTLS/Xray-core).

## Direction

- The client logic lives in the Go core: subscriptions, links, the sing-box and Xray-core configurations, the connection, server tests.
- The app is a Tauri 2 shell with a Vue 3 interface. It targets Windows as a portable app, and its code is kept ready for Linux and Android.
- Protocols are those of sing-box and Xray-core, VLESS with REALITY and XHTTP among them, plus OpenVPN and OpenConnect later.

## Layout

| Folder | What is in it |
|---|---|
| `src` | The interface: Vue 3, TypeScript, Tailwind CSS, shadcn-vue components, fonts and images |
| `src-tauri` | The Tauri shell in Rust: the window, the data folder, the bridge to the interface |
| `core` | The Go core: the engine and its routing presets, sing-box and Xray-core, server tests, the kill switch guard, the updater |
| `e2e` | Playwright checks of the interface |
| `res` | Tray icons and the sing-box JSON schema |
| `script` | `build_windows.ps1` builds the portable folder, `gen_schema.sh` regenerates `res/schema/sing-box.json` |

## Building

Needs the VS 2022 Build Tools C++ workload and the WebView2 Runtime, plus Go, Rust with the MSVC target and Node.js. The script takes them from one folder: `go\go`, `gopath`, `rust` (with `rustup` and `cargo` inside) and `node`.

```powershell
script\build_windows.ps1 -Toolchain <folder>
```

The output goes to `deployment\windows-amd64`: `Poros.exe`, `PorosCore.exe`, `updater.exe` and `libcronet.dll`. The script runs at below normal priority on 90% of the processors.

## Development

| Command | What it does |
|---|---|
| `npm run dev` | The interface alone in a browser at `http://127.0.0.1:1420`, with the shell stubbed out |
| `npm run tauri dev` | The app window with the interface reloading on every change |
| `npm run typecheck` | `vue-tsc` over the interface |
| `npm test` | Vitest |
| `npm run e2e` | Playwright in headless Chromium against the dev server |
| `cargo clippy --manifest-path src-tauri/Cargo.toml` | Lints the shell |

## Portable

Poros has no installer: put the folder anywhere you can write to and run `Poros.exe`. Its data, the WebView2 profile included, stays in `config` next to it, so deleting the folder removes Poros. If the folder is read-only, Poros says so and exits. Windows needs the WebView2 Runtime, which Windows 11 has built in; without it Poros offers the download page.

## Fonts

The interface uses [Onest](https://github.com/googlefonts/onest), [Unbounded](https://github.com/googlefonts/unbounded) and [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) under the SIL Open Font License 1.1, and shows country flags with a subset of Noto Color Emoji (SIL Open Font License 1.1). The fonts and the license texts of the first three are in `src/assets/fonts`.

## Third-party notices

`THIRD-PARTY-NOTICES.txt` lists the projects Poros includes, with their licenses.

## Upstream

Poros does not follow Throne release by release. Fixes for performance, security and the connection itself are ported by hand. The core is built on the upstream sing-box and Xray-core, without Throne's forks of them.

## The core and the app

`Poros.exe` starts `PorosCore.exe` from its own folder and talks to it over a named pipe it creates with a random name, one instance only. Both speak JSON-RPC 2.0, one message per line, and the core answers nothing before a hello carrying the token it got in its environment. The interface reaches the core only through the shell. The core's methods and events are the Go types in `core/engine/api.go`, and `go tool tygo generate` in `core` writes them to `src/platform/core-types.ts`. For `npm run tauri dev`, `POROS_CORE_PATH` points at a built `PorosCore.exe`.
