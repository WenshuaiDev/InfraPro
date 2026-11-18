package config

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidateMFAIssuer(value string) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 64 {
		return fmt.Errorf("mfa_issuer must contain 1–64 characters")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("mfa_issuer cannot contain control characters")
		}
	}
	return nil
}
