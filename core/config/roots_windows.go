package config

import (
	"encoding/pem"
	"errors"
	"slices"
	"unsafe" // also for go:linkname

	_ "github.com/sagernet/sing-box/common/certificate"
	"golang.org/x/sys/windows"
)

// The Mozilla roots built into sing-box, which gives them out only as the store of every TLS client at once.
//
//go:linkname mozillaRoots github.com/sagernet/sing-box/common/certificate.mozillaIncludedPEM
func mozillaRoots() string

// oidServerAuth is the purpose of a certificate of a server.
const oidServerAuth = "1.3.6.1.5.5.7.3.1"

var procCertGetEnhancedKeyUsage = windows.NewLazySystemDLL("crypt32.dll").NewProc("CertGetEnhancedKeyUsage")

// dnsRoots are what a public DNS verifies by, as Windows fetches what it lacks for a chain from the internet, by
// the proxy of the system or the TUN: by Poros, which waits for that very DNS. They are the built-in roots and the
// installed ones of companies and antiviruses, which read TLS on the way.
func dnsRoots() string {
	return mozillaRoots() + "\n" + installedRoots()
}

// installedRoots reads the stores of Windows, which asks nothing from the network, unlike a verification by them.
// The public roots of AuthRoot are left out: the built-in ones stand for them, and Windows takes their trust back
// without deleting them. A store that does not open leaves the built-in roots alone.
func installedRoots() string {
	var stores []windows.Handle
	defer func() {
		for _, store := range stores {
			_ = windows.CertCloseStore(store, 0)
		}
	}()
	for _, name := range []string{"ROOT", "AuthRoot", "Disallowed"} {
		text, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return ""
		}
		store, err := windows.CertOpenSystemStore(0, text)
		if err != nil {
			return ""
		}
		stores = append(stores, store)
	}
	return rootsForServers(stores[0], stores[1:]...)
}

// rootsForServers is the PEM of the roots of a store that are trusted for servers and are in none of the others.
func rootsForServers(store windows.Handle, others ...windows.Handle) string {
	left := map[string]bool{}
	for _, other := range others {
		eachCertificate(other, func(_ *windows.CertContext, der []byte) { left[string(der)] = true })
	}
	var text []byte
	eachCertificate(store, func(cert *windows.CertContext, der []byte) {
		if !left[string(der)] && forServers(cert) {
			left[string(der)] = true
			text = append(text, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
		}
	})
	return string(text)
}

func eachCertificate(store windows.Handle, visit func(cert *windows.CertContext, der []byte)) {
	var cert *windows.CertContext
	for {
		next, err := windows.CertEnumCertificatesInStore(store, cert)
		if err != nil {
			return
		}
		cert = next
		visit(cert, unsafe.Slice(cert.EncodedCert, cert.Length))
	}
}

// forServers tells whether Windows lets a root vouch for servers: its purposes are in the certificate and in the
// properties of the store.
func forServers(cert *windows.CertContext) bool {
	var size uint32
	ok, _, _ := procCertGetEnhancedKeyUsage.Call(uintptr(unsafe.Pointer(cert)), 0, 0, uintptr(unsafe.Pointer(&size)))
	if ok == 0 || uintptr(size) < unsafe.Sizeof(windows.CertEnhKeyUsage{}) {
		return false
	}
	buffer := make([]uint64, size/8+1)
	usage := (*windows.CertEnhKeyUsage)(unsafe.Pointer(&buffer[0]))
	ok, _, err := procCertGetEnhancedKeyUsage.Call(
		uintptr(unsafe.Pointer(cert)), 0, uintptr(unsafe.Pointer(usage)), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return false
	}
	if usage.Length == 0 {
		// No list is every purpose, an empty list is none: only the last error tells them apart.
		return errors.Is(err, windows.Errno(windows.CRYPT_E_NOT_FOUND))
	}
	return slices.ContainsFunc(unsafe.Slice(usage.UsageIdentifiers, usage.Length), func(oid *byte) bool {
		return windows.BytePtrToString(oid) == oidServerAuth
	})
}
