// File: internal/clipboard/storage.go
package clipboard

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const historyFileName = "clipboard_history.json"
const encryptionKeyFileName = "encryption.key" // New constant
const maxStoredHistoryBytes = 256 << 20
const maxHistoryItems = 100000

// PBKDF2 parameters for secure key derivation
const (
	pbkdf2Iterations = 100000 // High iteration count for security
	pbkdf2KeyLen     = 32     // AES-256 key length
	saltLen          = 16     // Salt length in bytes
)

// Storage handles persistence of clipboard history
type Storage struct {
	mu       sync.RWMutex
	filePath string
	key      []byte // New field for encryption key
}

// NewStorage creates a new storage instance
func NewStorage(basePath string) (*Storage, error) {
	if basePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		basePath = homeDir
	}

	// Create directory if it doesn't exist
	configDir := filepath.Join(basePath, ".fyclip")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	s := &Storage{
		filePath: filepath.Join(configDir, historyFileName),
	}

	// --- Encryption Key Handling ---
	keyPath := filepath.Join(configDir, encryptionKeyFileName)
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load or create encryption key: %w", err)
	}
	s.key = key
	// --- End Encryption Key Handling ---

	return s, nil
}

// deriveKeyFromPassword derives a secure encryption key using PBKDF2
func deriveKeyFromPassword(password []byte, salt []byte) []byte {
	return pbkdf2.Key(password, salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
}

// loadOrCreateKey loads the encryption key from path or creates a new one
// Uses secure key derivation with PBKDF2 for new installations
// Maintains backward compatibility with existing hex-encoded keys
func loadOrCreateKey(keyPath string) ([]byte, error) {
	saltPath := keyPath + ".salt"

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		// Key file does not exist, create a new one with secure derivation
		// Use system entropy as a pseudo-password for automatic key generation
		systemEntropy := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, systemEntropy); err != nil {
			return nil, fmt.Errorf("failed to generate system entropy: %w", err)
		}

		// Generate a random salt
		salt := make([]byte, saltLen)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}

		// Derive key using PBKDF2
		key := deriveKeyFromPassword(systemEntropy, salt)
		// Save salt first (needed for key derivation)
		if err := os.WriteFile(saltPath, salt, 0600); err != nil {
			return nil, fmt.Errorf("failed to save salt: %w", err)
		}

		// Save system entropy (pseudo-password) for key derivation
		if err := os.WriteFile(keyPath, systemEntropy, 0600); err != nil {
			return nil, fmt.Errorf("failed to save encryption key: %w", err)
		}

		return key, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat encryption key file: %w", err)
	}

	// Key file exists, check if it's the new format (with salt) or old format
	if _, err := os.Stat(saltPath); err == nil {
		// New format with salt - load and derive
		salt, err := os.ReadFile(saltPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read salt file: %w", err)
		}

		systemEntropy, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}

		// Derive the key using PBKDF2
		key := deriveKeyFromPassword(systemEntropy, salt)
		return key, nil
	} else {
		// Old format - load directly (backward compatibility)
		encodedKey, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read encryption key file: %w", err)
		}
		key, err := hex.DecodeString(string(encodedKey))
		if err != nil {
			return nil, fmt.Errorf("failed to decode encryption key: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid encryption key length: %d bytes, expected 32", len(key))
		}

		return key, nil
	}
}

// Load reads clipboard history from disk
func (s *Storage) Load() ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Item{}, nil
		}
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}
	defer f.Close()
	encryptedData, err := io.ReadAll(io.LimitReader(f, maxStoredHistoryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}
	if len(encryptedData) > maxStoredHistoryBytes {
		return nil, fmt.Errorf("history file exceeds size limit")
	}

	// Decrypt data
	decryptedData, err := decrypt(encryptedData, s.key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt history: %w", err)
	}

	// Decompress data if compressed
	decompressedData, err := decompress(decryptedData)
	if err != nil {
		// If decompression fails, try using the data as-is (for backward compatibility)
		decompressedData = decryptedData
	}
	if len(decompressedData) > maxStoredHistoryBytes {
		return nil, fmt.Errorf("decompressed history exceeds size limit")
	}

	if len(decompressedData) == 0 {
		return nil, fmt.Errorf("empty decrypted data")
	}

	trimmed := strings.TrimLeft(string(decompressedData), " \t\n\r")
	if len(trimmed) > 0 && trimmed[0] != '{' && trimmed[0] != '[' {
		return nil, fmt.Errorf("decrypted data does not appear to be JSON")
	}

	var items []Item
	if err := json.Unmarshal(decompressedData, &items); err != nil {
		return nil, fmt.Errorf("failed to parse history file: %w", err)
	}

	if err := validateItems(items); err != nil {
		return nil, fmt.Errorf("invalid history data: %w", err)
	}

	// Generate thumbnails for items that don't have them (backward compatibility)
	for i := range items {
		items[i].EnsureThumbnail()
	}

	return items, nil
}

func validateItems(items []Item) error {
	if len(items) > maxHistoryItems {
		return fmt.Errorf("history contains too many items")
	}
	for i, item := range items {
		if item.ID == "" {
			return fmt.Errorf("item %d has empty ID", i)
		}
		if item.Type < TypeText || item.Type > TypeFile {
			return fmt.Errorf("item %d has invalid type: %d", i, item.Type)
		}
		if err := ValidateItem(&item); err != nil {
			return fmt.Errorf("item %d: %w", i, err)
		}
	}
	return nil
}

// Save writes clipboard history to disk
func (s *Storage) Save(items []Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateItems(items); err != nil {
		return fmt.Errorf("refusing to save invalid history: %w", err)
	}

	jsonData, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	// Compress data before encryption
	compressedData := compress(jsonData)

	// Encrypt data
	encryptedData, err := encrypt(compressedData, s.key)
	if err != nil {
		return fmt.Errorf("failed to encrypt history: %w", err)
	}

	// Write to temp file first
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, encryptedData, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, s.filePath); err != nil {
		os.Remove(tempFile) // Cleanup
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// GetPath returns the storage file path
func (s *Storage) GetPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filePath
}

// GetDir returns the directory path for storage
func (s *Storage) GetDir() string {
	return filepath.Dir(s.filePath)
}

// Clear removes all stored history
func (s *Storage) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove history file: %w", err)
	}

	return nil
}

// wipeSensitiveData securely wipes sensitive data from memory
func wipeSensitiveData(data []byte) {
	if data == nil {
		return
	}
	// Overwrite with random data multiple times
	for i := 0; i < 3; i++ {
		if _, err := rand.Read(data); err != nil {
			log.Printf("WARNING: failed to read from crypto/rand during secure wipe: %v", err)
		}
	}
	// Final overwrite with zeros
	for i := range data {
		data[i] = 0
	}
}

// encrypt encrypts data using AES-256 GCM.
// The nonce is prepended to the ciphertext.
func encrypt(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-256 GCM.
// The nonce is read from the beginning of the ciphertext.
func decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, encryptedMessage := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encryptedMessage, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// compress compresses data using gzip
// Returns the original data if compression fails (for safety)
func compress(data []byte) []byte {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return data // Return original if compression fails
	}

	_, err = writer.Write(data)
	if err != nil {
		writer.Close()
		return data
	}

	err = writer.Close()
	if err != nil {
		return data
	}

	return buf.Bytes()
}

// decompress decompresses gzip data
// Returns original data if decompression fails (for backward compatibility)
func decompress(data []byte) ([]byte, error) {
	// Check if data is gzip compressed (magic number)
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, fmt.Errorf("not gzip compressed")
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, err
	}
	defer reader.Close()

	return io.ReadAll(io.LimitReader(reader, maxStoredHistoryBytes+1))
}
