# Project Poros

Windows desktop client, shipped as **Poros**.

## Based on Throne

Project Poros is a modified version of [Throne](https://github.com/throneproj/Throne), forked from tag `1.4.0-beta.4` in October 2026, and is licensed under GPL-3.0: see [LICENSE](LICENSE). It builds on [sing-box](https://github.com/SagerNet/sing-box) and [Xray-core](https://github.com/XTLS/Xray-core).

## Build

Needs the VS 2022 Build Tools C++ workload and a `toolchain` folder next to the repository (Go, Qt, OpenSSL, CMake, Ninja, protoc).

```powershell
script\build_windows.ps1
```

The output goes to `deployment\windows-amd64`. The build fails if a shipped file's name or properties mention a VPN or Throne, or if a binary contains a local path.

`script\build_windows.ps1 -Debugger` builds the GUI into `build-debug` for a debugger, with optimization off and full source paths. It copies the core from `deployment\windows-amd64`, so it needs a normal build first, and its output is never shipped.

## Interface

Poros opens in one of two modes, and the app menu switches between them at any time without dropping the connection.

- **Simple:** the servers of each subscription, a Start button, a TUN or System switch, ready presets and short settings. The first Start asks whether to use TUN or System.
- **Advanced:** groups and manual profiles, routing rules, DNS, connections, logs, statistics, tools and all the settings.

The first launch runs a short setup: subscription, how to start, interface mode. Every step can be skipped, and it runs again from the app menu.

Administrator rights are asked for only when an operation needs them, and you can always try without. A launch itself asks only when something has to work before the first click: a remembered TUN with `Connect to the last profile`, or the kill switch.

Built-in presets cannot be changed. A copy keeps its preset's rules and adds your own sites and programs ahead of them.

## Left out on purpose

Compared to Throne, Poros has no Poros API or `thronectl`, background service, web panel, Clash API, IP scanner, WebDAV, Tailscale, Extra Core, themes, custom icons, custom inbound, or `Open with` for JSON and YAML files.

## Next to Throne

Poros and Throne can run at the same time: each has its own folder, data, autostart entry and `poros://` or `throne://` links, and Poros keeps clear of Throne's default ports and TUN addresses (mixed port 2090, DNS port 5543, TUN 172.19.1.1/24).

Two things stay shared by Windows. Only one client can carry the traffic of the whole computer, so run TUN in one of them at a time; with TUN in Throne, Poros in System goes out through it. The system proxy is also one per user: the client that set it last holds it, and Poros's kill switch blocks every other client, Throne included.

## Portable

Poros has no installer: put the folder anywhere you can write to and run `Poros.exe`. Profiles, settings, logs, crash reports and temporary files all stay in `config` next to it, so deleting the folder removes Poros. If the folder is read-only, Poros says so and exits.

Windows only offers three features through its own stores, so they exist outside the folder if you turn them on, and stay there if you delete the folder first: Start with system (a Task Scheduler task), and `poros://` links and the Open with entry (`HKCU\Software\Classes`). TUN mode makes Windows install the Wintun driver, and the system proxy setting is restored when Poros stops it.

## Staying close to upstream

`core/` and the engine layers are merged from upstream with `script\merge_upstream.ps1`, so only names the OS or the user can see were changed there. To keep that merge small, parts of upstream that Poros leaves out are filtered in `CMakeLists.txt` rather than cut from its source lists, and settings that left the interface stay as hidden widgets in the settings dialog. The Go module, code identifiers, comments, `THRONE_*` variables and `thronectl` keep their names. `throne://` links and `throne-route-profile` data stay importable.
