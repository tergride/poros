package engine

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
)

var keyDeviceID = []byte("device-id")

// fetchClient downloads subscriptions directly, or through the running connection when the user asks for it.
func (e *Engine) fetchClient() (*http.Client, func()) {
	settings, err := e.Settings()
	box := e.conn.box.Load()
	if err != nil || !settings.SubscriptionViaProxy || box == nil {
		return e.client, func() {}
	}
	client, done := outboundClient(box, config.TagProxy, "", 0)
	client.Timeout = fetchTimeout
	return client, done
}

// deviceHeadersOf tell providers that limit devices which device asks: a random ID of this Poros, kept with its
// data, and the system, or what the group says instead, or what the settings say.
func (e *Engine) deviceHeadersOf(gs GroupSettings, own deviceData) http.Header {
	id := cmp.Or(gs.DeviceID, own.hwid)
	if id == "" {
		var err error
		if id, err = e.deviceID(); err != nil {
			return nil
		}
	}
	headers := http.Header{}
	headers.Set("X-Hwid", id)
	headers.Set("X-Device-Os", cmp.Or(gs.OS, own.os, osName))
	if version := cmp.Or(gs.OSVersion, own.osVersion, osVersion()); version != "" {
		headers.Set("X-Ver-Os", version)
	}
	if model := cmp.Or(gs.DeviceModel, own.model); model != "" {
		headers.Set("X-Device-Model", model)
	}
	return headers
}

// deviceData is what the settings tell providers about the device instead of Poros; "" leaves Poros's own.
type deviceData struct {
	hwid, os, osVersion, model string
}

// parseDeviceData reads "hwid=…, os=…, osVersion=…, model=…": any of the four, in any order and case.
func parseDeviceData(text string) (deviceData, error) {
	var data deviceData
	for part := range strings.SplitSeq(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || value == "" {
			return data, fmt.Errorf("%q is no key=value", part)
		}
		if strings.ContainsFunc(value, unicode.IsControl) {
			return data, fmt.Errorf("the value of %s holds a control character", key)
		}
		switch strings.ToLower(key) {
		case "hwid":
			data.hwid = value
		case "os":
			data.os = value
		case "osversion":
			data.osVersion = value
		case "model":
			data.model = value
		default:
			return data, fmt.Errorf("unknown key %q: hwid, os, osVersion or model", key)
		}
	}
	return data, nil
}

func (e *Engine) deviceID() (id string, err error) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	err = e.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketSettings)
		if stored := bucket.Get(keyDeviceID); stored != nil {
			id = string(stored)
			return nil
		}
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		id = hex.EncodeToString(b[:])
		return bucket.Put(keyDeviceID, []byte(id))
	})
	return id, err
}
