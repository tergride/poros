#!/bin/bash
set -e

TAGS="with_quic,with_wireguard,with_utls,with_dhcp,with_openvpn,with_openconnect,badlinkname,tfogo_checklinkname0,with_purego,with_naive_outbound"

rm -rf $DEST
mkdir -p $DEST

export CGO_ENABLED=0

pushd core
pushd gen
protoc -I . --go_out=. --go-grpc_out=. libcore.proto
popd
# The lib module ships the DLL, so it is always the binding's generation.
CRONET_LIB=github.com/sagernet/cronet-go/lib/windows_$GOARCH
go mod download $CRONET_LIB
install -m 644 "$(go list -m -f '{{.Dir}}' $CRONET_LIB)/libcronet.dll" $DEST/
VERSION_SINGBOX=$(go list -m -f '{{.Version}}' github.com/sagernet/sing-box)
go build -v -o "$DEST/PorosCore.exe" -trimpath -ldflags "-w -s -X 'github.com/sagernet/sing-box/constant.Version=${VERSION_SINGBOX}' -X 'internal/godebug.defaultGODEBUG=multipathtcp=0' -checklinkname=0" -tags "$TAGS"
go build -v -o "$DEST/updater.exe" -trimpath -ldflags "-w -s -H=windowsgui" ./cmd/updater
popd
