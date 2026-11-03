# Project Poros

A VPN and proxy client, shipped as **Poros**. It is being rebuilt as a cross-platform app on Tauri 2, Vue 3 and a Go core, and has no release yet. The earlier Windows client on Qt is no longer developed.

## Based on Throne

Project Poros is a modified version of [Throne](https://github.com/throneproj/Throne), forked from tag `1.4.0-beta.4` in October 2026, and is licensed under GPL-3.0: see [LICENSE](LICENSE). It builds on [sing-box](https://github.com/SagerNet/sing-box) and [Xray-core](https://github.com/XTLS/Xray-core).

## Direction

- The client logic lives in the Go core: subscriptions, links, the sing-box and Xray-core configurations, the connection, server tests. The same library serves the desktop app and, later, the mobile ones.
- The desktop app is a Tauri 2 shell with a Vue 3 interface: Windows first, then Linux and macOS. Android and iOS come after them.
- Protocols are those of sing-box and Xray-core, VLESS with REALITY and XHTTP among them, plus OpenVPN and OpenConnect.

## Layout

| Folder | What is in it |
|---|---|
| `core` | The Go core: sing-box and Xray-core, server tests, the kill switch guard, the updater |
| `res` | Fonts, images, icons, routing presets and the sing-box JSON schema for the new interface |
| `script` | `build_go.sh` builds the core, `gen_schema.sh` regenerates `res/schema/sing-box.json` |

## Building the core

The core needs Go, protoc and its Go plugins `protoc-gen-go` and `protoc-gen-go-grpc`. From Git Bash, with all of them on `PATH`:

```bash
DEST=<output folder> GOOS=windows GOARCH=amd64 script/build_go.sh
```

The script empties `DEST` first, then writes `PorosCore.exe`, `updater.exe` and `libcronet.dll` there.

## Fonts

The interface is to use [Onest](https://github.com/googlefonts/onest), [Unbounded](https://github.com/googlefonts/unbounded) and [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) under the SIL Open Font License 1.1, and to show emoji with a subset of Noto Color Emoji (SIL Open Font License 1.1) or with Twemoji (graphics under CC BY 4.0). The fonts and the license texts of the first three are in `res/font`.

## Third-party notices

`THIRD-PARTY-NOTICES.txt` lists the projects Poros includes, with their licenses.

## Upstream

Poros does not follow Throne release by release. Fixes for performance, security and the connection itself are ported by hand. The core is built on Throne's forks of sing-box, Xray-core, sing and wireguard-go.
