package engine

import (
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

// deviceHeaders tell providers that limit devices which device asks, when the user agreed to it: a random ID of
// this Poros, kept with its data, and the system.
func (e *Engine) deviceHeaders() http.Header {
	settings, err := e.Settings()
	if err != nil || !settings.SendDeviceInfo {
		return nil
	}
	id, err := e.deviceID()
	if err != nil {
		return nil
	}
	headers := http.Header{}
	headers.Set("X-Hwid", id)
	headers.Set("X-Device-Os", osName)
	if version := osVersion(); version != "" {
		headers.Set("X-Ver-Os", version)
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
