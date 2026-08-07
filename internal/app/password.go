// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/crypto/argon2"
)

// PasswordParams tunes the argon2id adaptive hash (SYS-091: "credentials
// SHALL be stored only as modern adaptive hashes"). Defaults follow the
// OWASP Password Storage Cheat Sheet's argon2id baseline for
// server-side, interactive-login hashing (as of 2026): 19 MiB memory,
// 2 iterations, 1 degree of parallelism, 16-byte salt, 32-byte key.
type PasswordParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

// DefaultPasswordParams is the current adaptive-hash configuration. Bumping
// any field here is a non-breaking, forward-compatible change: the encoded
// hash embeds the parameters it was created with, and VerifyPassword always
// verifies against the embedded parameters — NeedsRehash then reports
// whether a login should upgrade to DefaultPasswordParams (SYS-091
// "modern" is a moving target, so upgrade-on-verify keeps stored hashes
// current without a forced mass reset).
var DefaultPasswordParams = PasswordParams{
	MemoryKiB:   19 * 1024,
	Iterations:  2,
	Parallelism: 1,
	SaltLen:     16,
	KeyLen:      32,
}

// ErrMalformedHash means the stored encoding could not be parsed.
var ErrMalformedHash = errors.New("malformed password hash")

// ErrPasswordMismatch means the password did not match the stored hash.
var ErrPasswordMismatch = errors.New("password does not match")

// MinPasswordLength is the shared minimum-length policy (SYS-091) for every
// password a human types into this system: first-run bootstrap and account
// creation (internal/web/meets.go's setup flow), an admin-issued temporary
// password (TASK-053's ResetPassword), and the forced change-password step
// that follows one (ChangePassword) — "the existing setup-page policy",
// applied everywhere a new password is set rather than re-derived per form.
const MinPasswordLength = 8

// ErrPasswordTooShort means a submitted password is under MinPasswordLength.
var ErrPasswordTooShort = errors.New("password must be at least 8 characters")

// ValidatePasswordPolicy checks password against MinPasswordLength. It does
// not check emptiness separately: any password under 8 characters is also
// non-empty-invalid, so ErrPasswordTooShort already covers "".
func ValidatePasswordPolicy(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

const hashFormatVersion = 19 // argon2.Version, embedded for forward compatibility

// HashPassword encodes an argon2id hash in the PHC-like string format
// `$argon2id$v=19$m=...,t=...,p=...$<salt-b64>$<key-b64>`, self-describing
// so verification and parameter upgrades never need out-of-band metadata.
func HashPassword(password string, params PasswordParams) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	salt := make([]byte, params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, params.KeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		hashFormatVersion, params.MemoryKiB, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// decodedHash is a parsed encoding, used both for verification and for
// deciding whether re-hashing on next login is warranted.
type decodedHash struct {
	params PasswordParams
	salt   []byte
	key    []byte
}

func decodeHash(encoded string) (decodedHash, error) {
	parts := strings.Split(encoded, "$")
	// parts[0] is empty (leading '$'); ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return decodedHash{}, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return decodedHash{}, fmt.Errorf("%w: version: %v", ErrMalformedHash, err)
	}

	var p PasswordParams
	var m, t uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &par); err != nil {
		return decodedHash{}, fmt.Errorf("%w: params: %v", ErrMalformedHash, err)
	}
	p.MemoryKiB, p.Iterations, p.Parallelism = m, t, par

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return decodedHash{}, fmt.Errorf("%w: salt: %v", ErrMalformedHash, err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return decodedHash{}, fmt.Errorf("%w: key: %v", ErrMalformedHash, err)
	}
	// Bounds-checked before each narrowing conversion: a malformed or
	// adversarial encoding could in principle decode to a slice longer than
	// uint32 can represent, which would silently wrap below.
	saltLen, keyLen := len(salt), len(key)
	if saltLen > math.MaxUint32 {
		return decodedHash{}, ErrMalformedHash
	}
	p.SaltLen = uint32(saltLen)
	if keyLen > math.MaxUint32 {
		return decodedHash{}, ErrMalformedHash
	}
	p.KeyLen = uint32(keyLen)
	return decodedHash{params: p, salt: salt, key: key}, nil
}

// VerifyPassword reports whether password matches the stored encoding, using
// a constant-time comparison of the derived key (timing-attack resistant).
func VerifyPassword(password, encoded string) error {
	dh, err := decodeHash(encoded)
	if err != nil {
		return err
	}
	candidate := argon2.IDKey([]byte(password), dh.salt, dh.params.Iterations, dh.params.MemoryKiB, dh.params.Parallelism, dh.params.KeyLen)
	if subtle.ConstantTimeCompare(candidate, dh.key) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// NeedsRehash reports whether encoded was produced with weaker parameters
// than want, so a caller can transparently re-hash on next successful
// verification (SYS-091 "modern adaptive hashes" without a forced reset).
func NeedsRehash(encoded string, want PasswordParams) bool {
	dh, err := decodeHash(encoded)
	if err != nil {
		return true // unparsable encodings are always due for replacement
	}
	p := dh.params
	return p.MemoryKiB < want.MemoryKiB ||
		p.Iterations < want.Iterations ||
		p.Parallelism < want.Parallelism ||
		p.KeyLen < want.KeyLen
}
