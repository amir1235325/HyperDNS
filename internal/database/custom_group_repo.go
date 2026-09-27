package database

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

// CustomPolicyGroup is an operator-defined named policy: a bundle of domains that
// resolve to one action (proxy/direct/block), toggled as a unit and editable at
// runtime. It is the structured evolution of the flat Custom Proxied/Blocked/
// Direct lists — same data class (no credential, no subscriber), so it is stored
// as plain JSON in its own bucket for the same reason policies are (see
// policy_repo.go's note on why policy data is deliberately not sealed).
type CustomPolicyGroup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Action    string    `json:"action"` // "proxy" | "direct" | "block"
	Domains   []string  `json:"domains"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrCustomGroupNotFound is returned when a group ID names no stored record.
var ErrCustomGroupNotFound = errors.New("custom policy group not found")

// ListCustomGroups returns every stored group, ordered by creation time so the
// dashboard list is stable across reloads.
func (db *DB) ListCustomGroups() ([]CustomPolicyGroup, error) {
	var out []CustomPolicyGroup
	err := db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketCustomGroups)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var g CustomPolicyGroup
			if err := json.Unmarshal(v, &g); err != nil {
				return err
			}
			out = append(out, g)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// SaveCustomGroup stores or replaces a group by ID.
func (db *DB) SaveCustomGroup(g CustomPolicyGroup) error {
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketCustomGroups)
		if err != nil {
			return err
		}
		return b.Put([]byte(g.ID), data)
	})
}

// DeleteCustomGroup removes a group by ID. A missing ID is reported so the API
// can answer 404 rather than a silent 200.
func (db *DB) DeleteCustomGroup(id string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketCustomGroups)
		if b == nil {
			return ErrCustomGroupNotFound
		}
		if b.Get([]byte(id)) == nil {
			return ErrCustomGroupNotFound
		}
		// The uploaded icon is part of the group's identity; a deleted group
		// must not leave an orphan blob behind in the icons bucket.
		if ib := tx.Bucket(bucketGroupIcons); ib != nil {
			_ = ib.Delete([]byte(id))
		}
		return b.Delete([]byte(id))
	})
}

// GroupIcon is one uploaded icon: the raw bytes plus the content type the
// validator determined (image/svg+xml or image/png). Like the group record
// itself it is non-credential data, so it is stored unsealed in its own bucket
// and rides along with every data.db backup/restore.
type GroupIcon struct {
	Kind string `json:"kind"` // "svg" | "png"
	Data []byte `json:"data"`
}

// SaveCustomGroupIcon stores (or replaces) the icon for a group id.
func (db *DB) SaveCustomGroupIcon(id string, icon GroupIcon) error {
	if id == "" {
		return errors.New("custom group icon: empty id")
	}
	data, err := json.Marshal(icon)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketGroupIcons)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), data)
	})
}

// GetCustomGroupIcon returns the stored icon for a group id; ok is false when
// the group has no icon (or an unreadable record, which is treated as none).
func (db *DB) GetCustomGroupIcon(id string) (GroupIcon, bool) {
	var icon GroupIcon
	err := db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroupIcons)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &icon)
	})
	if err != nil || len(icon.Data) == 0 {
		return GroupIcon{}, false
	}
	return icon, true
}

// DeleteCustomGroupIcon removes a group's icon; deleting a non-existent one is
// a no-op (the dashboard's "remove icon" is idempotent by nature).
func (db *DB) DeleteCustomGroupIcon(id string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroupIcons)
		if b == nil {
			return nil
		}
		return b.Delete([]byte(id))
	})
}
