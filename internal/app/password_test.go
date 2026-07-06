// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"strings"
	"testing"
)

// testParams uses the minimum viable argon2id cost so the suite stays
// fast; production always uses DefaultPasswordParams (see auth.go).
var testParams = PasswordParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", testParams)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("hash encoding = %q, want $argon2id$v=19$ prefix", hash)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Errorf("VerifyPassword(correct password) = %v, want nil", err)
	}
	if err := VerifyPassword("wrong password", hash); err != ErrPasswordMismatch {
		t.Errorf("VerifyPassword(wrong password) = %v, want ErrPasswordMismatch", err)
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword("", testParams); err == nil {
		t.Error("HashPassword(\"\") = nil error, want error")
	}
}

func TestHashPasswordUniqueSalts(t *testing.T) {
	h1, err := HashPassword("same password", testParams)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashPassword("same password", testParams)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same password with fresh salts must differ")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash-at-all",
		"$argon2id$v=19$m=8192,t=1,p=1$onlyfourparts",
		"$bcrypt$v=1$m=1,t=1,p=1$c2FsdA$a2V5", // wrong algorithm tag
	}
	for _, c := range cases {
		if err := VerifyPassword("anything", c); err == nil {
			t.Errorf("VerifyPassword with malformed encoding %q = nil error, want error", c)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	weak := PasswordParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	strong := PasswordParams{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLen: 16, KeyLen: 32}

	hash, err := HashPassword("pw", weak)
	if err != nil {
		t.Fatal(err)
	}
	if !NeedsRehash(hash, strong) {
		t.Error("NeedsRehash(weak hash, strong params) = false, want true")
	}
	if NeedsRehash(hash, weak) {
		t.Error("NeedsRehash(weak hash, weak params) = true, want false")
	}

	hash2, err := HashPassword("pw", strong)
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(hash2, strong) {
		t.Error("NeedsRehash(strong hash, strong params) = true, want false")
	}
	if !NeedsRehash(hash2, PasswordParams{MemoryKiB: strong.MemoryKiB, Iterations: strong.Iterations + 1, Parallelism: strong.Parallelism, KeyLen: strong.KeyLen}) {
		t.Error("NeedsRehash should report true when wanted iterations exceed stored")
	}
}

func TestNeedsRehashMalformedAlwaysTrue(t *testing.T) {
	if !NeedsRehash("garbage", DefaultPasswordParams) {
		t.Error("NeedsRehash(unparsable) = false, want true")
	}
}
