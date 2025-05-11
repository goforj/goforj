package crypt

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func setTestAppKey(t *testing.T) {
	key, err := GenerateAppKey()
	if err != nil {
		t.Fatalf("GenerateAppKey failed: %v", err)
	}
	err = os.Setenv("APP_KEY", key)
	if err != nil {
		t.Fatalf("os.Setenv failed: %v", err)
	}
}

func TestGenerateAndReadAppKey(t *testing.T) {
	setTestAppKey(t)

	key, err := GetAppKey()
	if err != nil {
		t.Fatalf("GetAppKey failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Expected 32-byte key, got %d bytes", len(key))
	}
}

func TestEncryptAndDecrypt(t *testing.T) {
	setTestAppKey(t)

	plaintext := "This is a secret message."

	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypted result is empty")
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Expected decrypted to equal original. Got: %s", decrypted)
	}
}

func TestDecryptTamperedPayloadFails(t *testing.T) {
	setTestAppKey(t)

	plaintext := "safe data"
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	jsonRaw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	var payload EncryptedPayload
	if err := json.Unmarshal(jsonRaw, &payload); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	macBytes, err := base64.StdEncoding.DecodeString(payload.MAC)
	if err != nil {
		t.Fatalf("mac decode failed: %v", err)
	}
	macBytes[0] ^= 0xFF
	payload.MAC = base64.StdEncoding.EncodeToString(macBytes)

	modifiedJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	tampered := base64.StdEncoding.EncodeToString(modifiedJSON)

	_, err = Decrypt(tampered)
	if err == nil || !strings.Contains(err.Error(), "HMAC validation failed") {
		t.Errorf("Expected HMAC validation to fail, got: %v", err)
	}
}
