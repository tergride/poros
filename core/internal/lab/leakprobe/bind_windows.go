//go:build windows

package main

import (
	"encoding/binary"
	"math/bits"

	"golang.org/x/sys/windows"
)

const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

// bindToInterface sends a socket out of one adapter whatever the routes say (IP_UNICAST_IF and IPV6_UNICAST_IF).
// IPv4 takes the index in network byte order, IPv6 in host order.
func bindToInterface(fd uintptr, network string, index int) error {
	if network == "tcp6" {
		return windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IPV6, ipv6UnicastIf, index)
	}
	big := int(bits.ReverseBytes32(uint32(index)))
	if binary.NativeEndian.Uint16([]byte{0, 1}) == 1 {
		big = index
	}
	return windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, big)
}
