package engine

import (
	"encoding/json"
	"fmt"

	"github.com/sagernet/bbolt"
)

var (
	bucketSettings = []byte("settings")
	bucketGroups   = []byte("groups")
	// bucketServers holds the servers of each group under its ID: a subscription's in the order the provider gave them.
	bucketServers = []byte("servers")
	// bucketPresets holds the user's copies of the built-in presets under their IDs.
	bucketPresets = []byte("presets")

	keySettings = []byte("settings")
)

func createBuckets(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		for _, name := range [][]byte{bucketSettings, bucketGroups, bucketServers, bucketPresets, bucketTraffic, bucketOTP, bucketEndpoints} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("create bucket %s: %w", name, err)
			}
		}
		return nil
	})
}

// getJSON decodes the value over v, so fields missing in the stored value keep what v had. The secrets of servers,
// subscriptions, endpoints, codes of OTP and the settings come out open.
func getJSON(tx *bbolt.Tx, bucket, key []byte, v any) (bool, error) {
	data := tx.Bucket(bucket).Get(key)
	if data == nil {
		return false, nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("decode %s/%s: %w", bucket, key, err)
	}
	if box := boxOf(tx); box != nil {
		openValue(box, v)
	}
	return true, nil
}

// putJSON stores the value with its secrets sealed; the caller's value stays as it was.
func putJSON(tx *bbolt.Tx, bucket, key []byte, v any) error {
	if box := boxOf(tx); box != nil {
		var err error
		if v, err = sealValue(box, v); err != nil {
			return fmt.Errorf("seal %s/%s: %w", bucket, key, err)
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return tx.Bucket(bucket).Put(key, data)
}
