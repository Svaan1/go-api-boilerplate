package idgen

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestUUIDv7Layout(t *testing.T) {
	before := time.Now().UnixMilli()
	id := UUIDv7{}.NewID()
	after := time.Now().UnixMilli()

	groups := strings.Split(id, "-")
	if len(groups) != 5 || len(groups[0]) != 8 || len(groups[1]) != 4 || len(groups[2]) != 4 || len(groups[3]) != 4 || len(groups[4]) != 12 {
		t.Fatalf("id %q is not canonical 8-4-4-4-12", id)
	}
	raw, err := hex.DecodeString(strings.Join(groups, ""))
	if err != nil {
		t.Fatalf("id %q is not hex: %v", id, err)
	}
	if version := raw[6] >> 4; version != 7 {
		t.Fatalf("version = %d, want 7", version)
	}
	if variant := raw[8] >> 6; variant != 0b10 {
		t.Fatalf("variant bits = %b, want 10", variant)
	}
	var millis int64
	for _, b := range raw[:6] {
		millis = millis<<8 | int64(b)
	}
	if millis < before || millis > after {
		t.Fatalf("timestamp %d outside [%d, %d]", millis, before, after)
	}
}

func TestUUIDv7IsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		id := UUIDv7{}.NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = struct{}{}
	}
}
