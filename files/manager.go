package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/yanolja/ogem/openai"
)

// Manager handles file storage and retrieval
type Manager struct {
	storageDir string
	metadata   map[string]*openai.File
	mutex      sync.RWMutex
	logger     *zap.SugaredLogger
}

// NewManager creates a new file manager
func NewManager(storageDir string, logger *zap.SugaredLogger) (*Manager, error) {
	if storageDir == "" {
		storageDir = "./data/files"
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %v", err)
	}

	return &Manager{
		storageDir: storageDir,
		metadata:   make(map[string]*openai.File),
		logger:     logger,
	}, nil
}

// Upload stores a file and returns metadata
func (m *Manager) Upload(ctx context.Context, filename string, content io.Reader, purpose string) (*openai.File, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Validate purpose
	if !isValidPurpose(purpose) {
		return nil, fmt.Errorf("invalid purpose: %s. Must be one of: fine-tune, assistants, batch", purpose)
	}

	// Generate unique file ID
	fileID := fmt.Sprintf("file-%s", uuid.New().String())

	// Create storage path
	storagePath := filepath.Join(m.storageDir, fileID)

	// Write file content
	file, err := os.Create(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	bytesWritten, err := io.Copy(file, content)
	if err != nil {
		os.Remove(storagePath) // Cleanup on error
		return nil, fmt.Errorf("failed to write file: %v", err)
	}

	// Create metadata
	metadata := &openai.File{
		ID:        fileID,
		Object:    "file",
		Bytes:     bytesWritten,
		CreatedAt: time.Now().Unix(),
		Filename:  filepath.Base(filename),
		Purpose:   purpose,
		Status:    "uploaded",
	}

	// Store metadata
	m.metadata[fileID] = metadata

	m.logger.Infow("File uploaded", "file_id", fileID, "filename", filename, "size", bytesWritten)

	return metadata, nil
}

// List returns a list of files
func (m *Manager) List(ctx context.Context) (*openai.FileList, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	files := make([]openai.File, 0, len(m.metadata))
	for _, file := range m.metadata {
		files = append(files, *file)
	}

	return &openai.FileList{
		Object:  "list",
		Data:    files,
		HasMore: false,
	}, nil
}

// Retrieve returns file metadata
func (m *Manager) Retrieve(ctx context.Context, fileID string) (*openai.File, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	file, exists := m.metadata[fileID]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}

	return file, nil
}

// Delete removes a file
func (m *Manager) Delete(ctx context.Context, fileID string) (*openai.FileDeleteResponse, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.metadata[fileID]; !exists {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}

	// Remove file from storage
	storagePath := filepath.Join(m.storageDir, fileID)
	if err := os.Remove(storagePath); err != nil {
		m.logger.Warnw("Failed to delete file from storage", "file_id", fileID, "error", err)
	}

	// Remove metadata
	delete(m.metadata, fileID)

	m.logger.Infow("File deleted", "file_id", fileID)

	return &openai.FileDeleteResponse{
		ID:      fileID,
		Object:  "file",
		Deleted: true,
	}, nil
}

// GetContent returns file content
func (m *Manager) GetContent(ctx context.Context, fileID string) ([]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if _, exists := m.metadata[fileID]; !exists {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}

	storagePath := filepath.Join(m.storageDir, fileID)
	content, err := os.ReadFile(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	return content, nil
}

// ComputeChecksum computes SHA-256 checksum of file content
func (m *Manager) ComputeChecksum(ctx context.Context, fileID string) (string, error) {
	content, err := m.GetContent(ctx, fileID)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// Helper functions

func isValidPurpose(purpose string) bool {
	validPurposes := map[string]bool{
		"fine-tune":  true,
		"assistants": true,
		"batch":      true,
	}
	return validPurposes[purpose]
}
