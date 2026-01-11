package v1alpha1

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

func ShortHash16(s string) string {
	s = strings.TrimSpace(s)
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}
