package crypt

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateAndReadAppKey(t *testing.T) {
	appKey, err := GenerateAppKey()
	if err != nil {
		t.Fatalf("GenerateAppKey failed: %v", err)
	}

	if !strings.HasPrefix(appKey, "base64:") {
		t.Errorf("Expected app key to start with 'base64:', got %s", appKey)
	}

	key, err := ReadAppKey(appKey)
	if err != nil {
		t.Fatalf("ReadAppKey failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Expected 32-byte key, got %d bytes", len(key))
	}
}

func TestEncryptAndDecrypt(t *testing.T) {
	appKey, err := GenerateAppKey()
	if err != nil {
		t.Fatalf("GenerateAppKey failed: %v", err)
	}

	plaintext := "This is a secret message."

	encrypted, err := Encrypt(appKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypted result is empty")
	}

	decrypted, err := Decrypt(appKey, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Expected decrypted to equal original. Got: %s", decrypted)
	}
}

func TestDecryptTamperedPayloadFails(t *testing.T) {
	appKey, err := GenerateAppKey()
	if err != nil {
		t.Fatalf("GenerateAppKey failed: %v", err)
	}

	plaintext := "safe data"
	encrypted, err := Encrypt(appKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decode the base64-encoded JSON payload
	jsonRaw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	var payload EncryptedPayload
	if err := json.Unmarshal(jsonRaw, &payload); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	// Corrupt the MAC field (flip a byte safely)
	macBytes, err := base64.StdEncoding.DecodeString(payload.MAC)
	if err != nil {
		t.Fatalf("mac decode failed: %v", err)
	}
	macBytes[0] ^= 0xFF // flip first byte
	payload.MAC = base64.StdEncoding.EncodeToString(macBytes)

	// Re-marshal the modified payload
	modifiedJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	tampered := base64.StdEncoding.EncodeToString(modifiedJSON)

	_, err = Decrypt(appKey, tampered)
	if err == nil || !strings.Contains(err.Error(), "HMAC validation failed") {
		t.Errorf("Expected HMAC validation to fail, got: %v", err)
	}
}
