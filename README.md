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

## Portable

Poros has no installer: put the folder anywhere you can write to and run `Poros.exe`. Profiles, settings, logs, crash reports and temporary files all stay in `config` next to it, so deleting the folder removes Poros. If the folder is read-only, Poros says so and exits.

Windows only offers three features through its own stores, so they exist outside the folder if you turn them on, and stay there if you delete the folder first: Start with system (a Task Scheduler task), and `poros://` links and the Open with entry (`HKCU\Software\Classes`). TUN mode makes Windows install the Wintun driver, and the system proxy setting is restored when Poros stops it.

## Staying close to upstream

`core/` and the engine layers are merged from upstream, so only names the OS or the user can see were changed there. The Go module, code identifiers, comments, `THRONE_*` variables and `thronectl` keep their names. `throne://` links and `throne-route-profile` data stay importable.
