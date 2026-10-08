// Package idgen generates resource identifiers.
package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// UUIDv7 generates RFC 9562 version 7 UUIDs. Their millisecond timestamp
// prefix keeps new primary keys roughly ordered, which suits B-tree indexes.
type UUIDv7 struct{}

// NewID returns a UUIDv7 in canonical 8-4-4-4-12 hex form.
func (UUIDv7) NewID() string {
	var b [16]byte
	var millis [8]byte
	binary.BigEndian.PutUint64(millis[:], uint64(time.Now().UnixMilli())) //nolint:gosec // Unix milliseconds are positive.
	copy(b[:6], millis[2:])
	// crypto/rand.Read never returns an error; it aborts the process if the
	// system random source fails.
	_, _ = rand.Read(b[6:])
	b[6] = b[6]&0x0f | 0x70 // version 7
	b[8] = b[8]&0x3f | 0x80 // RFC 9562 variant

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:], b[10:])
	return string(out[:])
}
