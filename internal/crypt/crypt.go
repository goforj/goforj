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
)

// GenerateAppKey generates a random app key
func GenerateAppKey() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	return "base64:" + encoded, nil
}

// ReadAppKey reads the app key from the config
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

// pkcs7Pad applies PKCS#7 padding to the given data
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// pkcs7Unpad removes PKCS#7 padding from the given data
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

// Encrypt encrypts a string and returns a base64-encoded payload (JSON with iv, value, mac)
func Encrypt(base64AppKey string, plaintext string) (string, error) {
	key, err := ReadAppKey(base64AppKey)
	if err != nil {
		return "", err
	}

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

	payload := EncryptedPayload{
		IV:    ivB64,
		Value: valB64,
		MAC:   macB64,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// Decrypt decodes, verifies, and decrypts a base64-encoded JSON payload
func Decrypt(base64AppKey string, encodedPayload string) (string, error) {
	key, err := ReadAppKey(base64AppKey)
	if err != nil {
		return "", err
	}

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
