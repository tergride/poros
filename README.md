# Project Poros

Windows desktop client, shipped as **Poros**.

## Based on Throne

Project Poros is a modified version of [Throne](https://github.com/throneproj/Throne), forked from tag `1.4.0-beta.4` in October 2026, and is licensed under GPL-3.0: see [LICENSE](LICENSE). It builds on [sing-box](https://github.com/SagerNet/sing-box) and [Xray-core](https://github.com/XTLS/Xray-core).

## Build

Needs the VS 2022 Build Tools C++ workload, Go, static Qt 6, OpenSSL, CMake, Ninja and protoc. The script takes the tools from one folder:

```powershell
script\build_windows.ps1 -Toolchain <folder>
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

## Updates and releases

Unless automatic updates are turned off in the settings, Poros checks for a new version at launch and every four hours, in `latest.json` of the newest release of this repository. It downloads the archive, verifies its size and SHA-256, and offers to install it. With automatic updates off, this happens only when you press Check. `updater.exe` replaces the program files after Poros exits and starts it again, and the `config` folder is not touched. If the install fails, the old files go back. A local build has no version, so it does not check.

To release, add a section to `CHANGELOG.md` with `<!-- ru -->` and `<!-- en -->` parts, push `main`, and run `script\release_windows.ps1`. It builds the version, uploads the archive and `latest.json` as a draft, and publishes only when both are there. With `-DryRun` it builds and publishes nothing.

## Fonts

The interface uses [Onest](https://github.com/googlefonts/onest), [Unbounded](https://github.com/googlefonts/unbounded) and [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) under the SIL Open Font License 1.1. The fonts and their license texts are in `res/font`.

## Upstream

Poros does not follow Throne release by release. Fixes for performance, security and the connection itself are ported by hand, while the interface and the feature set are its own. The Go module, code identifiers, comments and `THRONE_*` variables keep their names for now. `throne://` links and `throne-route-profile` data stay importable.
