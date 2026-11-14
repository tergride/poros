# Patched copies

Copies of Go modules with a change Poros needs before the project itself has it. `core/go.mod` points at each with a
`replace` line; once the project has the change, the copy and its line go.

| Copy | Version | Change |
|---|---|---|
| `sing` | `github.com/sagernet/sing` v0.9.7-0.20261023115510-03a6cce64bf7 | `common/network/multi.go`: `DialParallel` lets go of the context of the attempt that won. Before, it stayed a child of the dial's context, and through the TUN that context lives as long as Poros stays connected, so every connection to a server named by a domain with several addresses left a little memory behind. `core/internal/patches` checks it |
