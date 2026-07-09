package service

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// [LITE:DELETED] Helpers formerly colocated with deleted email/identity
// services. They remain pure functions for upstream compatibility only.

func firstEmailLocale(locale []string) string {
	for _, value := range locale {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "zh-CN"
}

func emailRecipientName(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	if idx := strings.IndexByte(email, '@'); idx > 0 {
		return email[:idx]
	}
	return email
}

func generateClientID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "liteclient"
	}
	return hex.EncodeToString(buf[:])
}
