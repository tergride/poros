package engine

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"net/http"

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
// data, and the system, or what the group says instead.
func (e *Engine) deviceHeadersOf(gs GroupSettings) http.Header {
	id := gs.DeviceID
	if id == "" {
		var err error
		if id, err = e.deviceID(); err != nil {
			return nil
		}
	}
	headers := http.Header{}
	headers.Set("X-Hwid", id)
	headers.Set("X-Device-Os", cmp.Or(gs.OS, osName))
	if version := cmp.Or(gs.OSVersion, osVersion()); version != "" {
		headers.Set("X-Ver-Os", version)
	}
	if gs.DeviceModel != "" {
		headers.Set("X-Device-Model", gs.DeviceModel)
	}
	return headers
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
