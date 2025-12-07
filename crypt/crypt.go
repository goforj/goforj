package crypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// GenerateAppKey generates a random base64 app key prefixed with "base64:"
func GenerateAppKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	return "base64:" + encoded, nil
}

// GetAppKey retrieves the APP_KEY from the environment and parses it
func GetAppKey() ([]byte, error) {
	key := os.Getenv("APP_KEY")
	if key == "" {
		return nil, errors.New("APP_KEY is not set in environment")
	}
	return ReadAppKey(key)
}

// ReadAppKey parses a base64 encoded app key with "base64:" prefix
func ReadAppKey(key string) ([]byte, error) {
	const prefix = "base64:"
	if len(key) < len(prefix) || key[:len(prefix)] != prefix {
		return nil, fmt.Errorf("unsupported or missing key prefix")
	}
	decoded, err := base64.StdEncoding.DecodeString(key[len(prefix):])
	if err != nil {
		return nil, err
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes after decoding")
	}
	return decoded, nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("invalid padding size")
	}
	padding := data[len(data)-1]
	if int(padding) > len(data) || padding == 0 {
		return nil, errors.New("invalid padding")
	}
	for _, b := range data[len(data)-int(padding):] {
		if b != padding {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-int(padding)], nil
}

type EncryptedPayload struct {
	IV    string `json:"iv"`
	Value string `json:"value"`
	MAC   string `json:"mac"`
}

// Encrypt encrypts a plaintext using the APP_KEY from environment
func Encrypt(plaintext string) (string, error) {
	key, err := GetAppKey()
	if err != nil {
		return "", err
	}
	return encryptWithKey(key, plaintext)
}

// Decrypt decrypts an encrypted payload using the APP_KEY from environment
func Decrypt(encodedPayload string) (string, error) {
	key, err := GetAppKey()
	if err != nil {
		return "", err
	}
	return decryptWithKey(key, encodedPayload)
}

// Internal encrypt logic with provided key
func encryptWithKey(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	ivB64 := base64.StdEncoding.EncodeToString(iv)
	valB64 := base64.StdEncoding.EncodeToString(ciphertext)
	mac := computeHMACSHA256(append(iv, ciphertext...), key)
	macB64 := base64.StdEncoding.EncodeToString(mac)

	payload := EncryptedPayload{IV: ivB64, Value: valB64, MAC: macB64}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// Internal decrypt logic with provided key
func decryptWithKey(key []byte, encodedPayload string) (string, error) {
	jsonBytes, err := base64.StdEncoding.DecodeString(encodedPayload)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	var payload EncryptedPayload
	if err := json.Unmarshal(jsonBytes, &payload); err != nil {
		return "", fmt.Errorf("json decode failed: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(payload.IV)
	if err != nil {
		return "", fmt.Errorf("iv decode failed: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload.Value)
	if err != nil {
		return "", fmt.Errorf("value decode failed: %w", err)
	}
	mac, err := base64.StdEncoding.DecodeString(payload.MAC)
	if err != nil {
		return "", fmt.Errorf("mac decode failed: %w", err)
	}

	expectedMAC := computeHMACSHA256(append(iv, ciphertext...), key)
	if !hmac.Equal(expectedMAC, mac) {
		return "", errors.New("HMAC validation failed")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	unpadded, err := pkcs7Unpad(ciphertext)
	if err != nil {
		return "", err
	}

	return string(unpadded), nil
}

func computeHMACSHA256(data []byte, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
