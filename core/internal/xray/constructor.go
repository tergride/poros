package xray

import (
	"bytes"
	"sync"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
)

// newMu runs one core.New at a time: each instance sets the DNS and the outbounds of Xray's system dialer, globals
// of xray-core, and the tests of servers and a connection create instances at once.
var newMu sync.Mutex

func CreateXrayInstance(config string) (*core.Instance, error) {
	r := bytes.NewReader([]byte(config))
	conf, err := serial.DecodeJSONConfig(r)
	if err != nil {
		return nil, err
	}

	b, err := conf.Build()
	if err != nil {
		return nil, err
	}

	newMu.Lock()
	server, err := core.New(b)
	newMu.Unlock()
	if err != nil {
		return nil, err
	}

	return server, nil
}

// Stops short of core.New on purpose: bulk validation runs concurrently while a live Xray instance is up.
func CheckXrayConfig(config string) error {
	r := bytes.NewReader([]byte(config))
	conf, err := serial.DecodeJSONConfig(r)
	if err != nil {
		return err
	}
	_, err = conf.Build()
	return err
}
