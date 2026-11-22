# Patched copies

Copies of Go modules with a change Poros needs before the project itself has it. `core/go.mod` points at each with a
`replace` line; once the project has the change, the copy and its line go.

| Copy | Version | Change |
|---|---|---|
| `sing` | `github.com/sagernet/sing` v0.9.7-0.20261023115510-03a6cce64bf7 | `common/network/multi.go`: `DialParallel` lets go of the context of the attempt that won. Before, it stayed a child of the dial's context, and through the TUN that context lives as long as Poros stays connected, so every connection to a server named by a domain with several addresses left a little memory behind. `core/internal/patches` checks it |
| `sing-tun` | `github.com/sagernet/sing-tun` v0.9.7-0.20261023115453-611d83b7a57b | `internal/winsys/helper.go` and `tun_windows.go`: Strict Route permits the program by its app ID wholly in lower case as well. `FwpmGetAppIdFromFileName0` lowers ASCII alone, so from a folder such as `Мои программы` the permit never matched, and the block of DNS stopped the program's own. `core/internal/patches` checks it. The copy has no `.gitignore` of sing-tun, which would hide files of it |
