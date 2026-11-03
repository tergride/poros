package engine

import (
	"encoding/json"
	"fmt"

	"github.com/sagernet/bbolt"
)

var (
	bucketSettings      = []byte("settings")
	bucketSubscriptions = []byte("subscriptions")

	keySettings = []byte("settings")
)

func createBuckets(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		for _, name := range [][]byte{bucketSettings, bucketSubscriptions} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("create bucket %s: %w", name, err)
			}
		}
		return nil
	})
}

// getJSON decodes the value over v, so fields missing in the stored value keep what v had.
func getJSON(tx *bbolt.Tx, bucket, key []byte, v any) (bool, error) {
	data := tx.Bucket(bucket).Get(key)
	if data == nil {
		return false, nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("decode %s/%s: %w", bucket, key, err)
	}
	return true, nil
}

func putJSON(tx *bbolt.Tx, bucket, key []byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return tx.Bucket(bucket).Put(key, data)
}
