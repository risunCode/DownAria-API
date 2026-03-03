package util

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

func GenerateRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}
