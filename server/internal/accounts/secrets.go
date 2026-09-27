package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	// bcrypt ignores everything past 72 bytes; reject rather than silently
	// truncate.
	maxPasswordBytes = 72
)

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func passwordMatches(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyPasswordHash is compared against when an email has no account, so a
// login attempt takes the same time whether or not the email exists.
var dummyPasswordHash, _ = hashPassword("aycorn-timing-equalizer")

// newToken returns a 256-bit random secret, URL-safe: session cookies and
// emailed one-time links.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// inviteAlphabet drops look-alike characters (0/O, 1/I/L, U) so a code read
// off an email and typed by hand survives the trip.
const inviteAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"

// newInviteCode returns a code like "K7Q2M-9XMFA" (~49 bits of entropy). That
// is plenty for a secret that is also bound to one email, expires, and works
// once.
func newInviteCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, v := range b {
		if i == 5 {
			sb.WriteByte('-')
		}
		sb.WriteByte(inviteAlphabet[int(v)%len(inviteAlphabet)])
	}
	return sb.String(), nil
}

// normalizeInviteCode makes typed codes forgiving: case, spaces, and dashes
// don't matter.
func normalizeInviteCode(code string) string {
	code = strings.ToUpper(code)
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, code)
}

func hashInviteCode(code string) string { return hashSecret(normalizeInviteCode(code)) }
