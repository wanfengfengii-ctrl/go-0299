package rules

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Digest produces an immutable design digest. It is a stable hash over a
// canonical, ordered byte encoding so identical inputs always produce the
// same digest regardless of map or slice iteration order.
type DigestFunc func(parts ...[]byte) string

// NewDigest returns a digest function hashing the concatenation of the given
// parts with length prefixes so that delimiters cannot collide.
func NewDigest() DigestFunc {
	return func(parts ...[]byte) string {
		h := sha256.New()
		for _, p := range parts {
			_ = binary.Write(h, binary.BigEndian, int64(len(p)))
			_, _ = h.Write(p)
		}
		return hex.EncodeToString(h.Sum(nil))
	}
}

// AppendInt64 appends a big-endian int64 to a byte slice, for canonical
// digest serialization of numeric fields.
func AppendInt64(b []byte, v int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	return append(b, buf[:]...)
}

// Hash is the package-level digest helper reused across components.
var Hash = NewDigest()
