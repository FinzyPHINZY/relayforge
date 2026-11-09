package id

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"time"
)

// New returns a UUIDv7 string. The random bytes make concurrent IDs unique.
func New() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	ms := uint64(time.Now().UTC().UnixMilli())
	binary.BigEndian.PutUint64(b[:8], (ms<<16)|(uint64(b[6]&0x0f)<<8)|uint64(b[7]))
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:]), nil
}

func Valid(raw string) bool {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(raw, "-", ""))
	return err == nil
}
