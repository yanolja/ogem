package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewManager(t *testing.T) {
	logger := zap.NewNop().Sugar()

	t.Run("creates storage directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		storageDir := filepath.Join(tmpDir, "test-storage")

		manager, err := NewManager(storageDir, logger)
		require.NoError(t, err)
		require.NotNil(t, manager)

		// Verify directory was created
		info, err := os.Stat(storageDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("uses default directory if empty", func(t *testing.T) {
		manager, err := NewManager("", logger)
		require.NoError(t, err)
		require.NotNil(t, manager)
		assert.Equal(t, "./data/files", manager.storageDir)
	})

	t.Run("fails on invalid directory", func(t *testing.T) {
		// Try to create in a file (not a directory)
		tmpFile := filepath.Join(t.TempDir(), "file")
		err := os.WriteFile(tmpFile, []byte("test"), 0644)
		require.NoError(t, err)

		_, err = NewManager(tmpFile, logger)
		assert.Error(t, err)
	})
}

func TestManager_Upload(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("uploads file successfully", func(t *testing.T) {
		content := strings.NewReader("test content")
		file, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")

		require.NoError(t, err)
		assert.NotEmpty(t, file.ID)
		assert.Equal(t, "test.txt", file.Filename)
		assert.Equal(t, "fine-tune", file.Purpose)
		assert.Equal(t, "uploaded", file.Status)
		assert.Equal(t, int64(12), file.Bytes)
		assert.NotZero(t, file.CreatedAt)
	})

	t.Run("validates purpose", func(t *testing.T) {
		content := strings.NewReader("test")
		_, err := manager.Upload(context.Background(), "test.txt", content, "invalid")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid purpose")
	})

	t.Run("accepts valid purposes", func(t *testing.T) {
		purposes := []string{"fine-tune", "assistants", "batch"}
		for _, purpose := range purposes {
			content := strings.NewReader("test")
			file, err := manager.Upload(context.Background(), "test.txt", content, purpose)
			require.NoError(t, err)
			assert.Equal(t, purpose, file.Purpose)
		}
	})
}

func TestManager_List(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("lists empty files", func(t *testing.T) {
		fileList, err := manager.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "list", fileList.Object)
		assert.Empty(t, fileList.Data)
		assert.False(t, fileList.HasMore)
	})

	t.Run("lists uploaded files", func(t *testing.T) {
		// Upload some files
		content1 := strings.NewReader("content1")
		file1, err := manager.Upload(context.Background(), "file1.txt", content1, "fine-tune")
		require.NoError(t, err)

		content2 := strings.NewReader("content2")
		file2, err := manager.Upload(context.Background(), "file2.txt", content2, "assistants")
		require.NoError(t, err)

		// List files
		fileList, err := manager.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, fileList.Data, 2)

		// Check files are in list
		fileIDs := []string{fileList.Data[0].ID, fileList.Data[1].ID}
		assert.Contains(t, fileIDs, file1.ID)
		assert.Contains(t, fileIDs, file2.ID)
	})
}

func TestManager_Retrieve(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("retrieves existing file", func(t *testing.T) {
		content := strings.NewReader("test content")
		uploaded, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		retrieved, err := manager.Retrieve(context.Background(), uploaded.ID)
		require.NoError(t, err)
		assert.Equal(t, uploaded.ID, retrieved.ID)
		assert.Equal(t, uploaded.Filename, retrieved.Filename)
		assert.Equal(t, uploaded.Purpose, retrieved.Purpose)
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		_, err := manager.Retrieve(context.Background(), "non-existent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file not found")
	})
}

func TestManager_Delete(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("deletes existing file", func(t *testing.T) {
		content := strings.NewReader("test content")
		uploaded, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		response, err := manager.Delete(context.Background(), uploaded.ID)
		require.NoError(t, err)
		assert.Equal(t, uploaded.ID, response.ID)
		assert.Equal(t, "file", response.Object)
		assert.True(t, response.Deleted)

		// Verify file is gone
		_, err = manager.Retrieve(context.Background(), uploaded.ID)
		assert.Error(t, err)

		// Verify physical file is gone
		storagePath := filepath.Join(tmpDir, uploaded.ID)
		_, err = os.Stat(storagePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		_, err := manager.Delete(context.Background(), "non-existent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file not found")
	})
}

func TestManager_GetContent(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("gets file content", func(t *testing.T) {
		expectedContent := "test file content"
		content := strings.NewReader(expectedContent)
		uploaded, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		retrievedContent, err := manager.GetContent(context.Background(), uploaded.ID)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(retrievedContent))
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		_, err := manager.GetContent(context.Background(), "non-existent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file not found")
	})
}

func TestManager_ComputeChecksum(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("computes SHA-256 checksum", func(t *testing.T) {
		content := strings.NewReader("test content")
		uploaded, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		checksum, err := manager.ComputeChecksum(context.Background(), uploaded.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, checksum)
		assert.Len(t, checksum, 64) // SHA-256 produces 64 hex characters

		// Verify checksum is consistent
		checksum2, err := manager.ComputeChecksum(context.Background(), uploaded.ID)
		require.NoError(t, err)
		assert.Equal(t, checksum, checksum2)
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		_, err := manager.ComputeChecksum(context.Background(), "non-existent-id")
		assert.Error(t, err)
	})
}

func TestManager_ConcurrentAccess(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("handles concurrent uploads", func(t *testing.T) {
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func(n int) {
				content := strings.NewReader("concurrent content")
				_, err := manager.Upload(context.Background(), "concurrent.txt", content, "fine-tune")
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		fileList, err := manager.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, fileList.Data, 10)
	})
}

func TestManager_DeleteWithMissingPhysicalFile(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("deletes metadata even if physical file is gone", func(t *testing.T) {
		content := strings.NewReader("test content")
		uploaded, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		// Manually delete the physical file
		storagePath := filepath.Join(tmpDir, uploaded.ID)
		err = os.Remove(storagePath)
		require.NoError(t, err)

		// Delete should still succeed
		response, err := manager.Delete(context.Background(), uploaded.ID)
		require.NoError(t, err)
		assert.True(t, response.Deleted)

		// Verify metadata is gone
		_, err = manager.Retrieve(context.Background(), uploaded.ID)
		assert.Error(t, err)
	})
}

func TestManager_GetContentWithMissingFile(t *testing.T) {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, logger)
	require.NoError(t, err)

	t.Run("fails when physical file is missing", func(t *testing.T) {
		content := strings.NewReader("test content")
		uploaded, err := manager.Upload(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		// Manually delete the physical file
		storagePath := filepath.Join(tmpDir, uploaded.ID)
		err = os.Remove(storagePath)
		require.NoError(t, err)

		// GetContent should fail
		_, err = manager.GetContent(context.Background(), uploaded.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})
}

func Test_isValidPurpose(t *testing.T) {
	tests := []struct {
		purpose string
		valid   bool
	}{
		{"fine-tune", true},
		{"assistants", true},
		{"batch", true},
		{"invalid", false},
		{"", false},
		{"FINE-TUNE", false}, // Case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.purpose, func(t *testing.T) {
			result := isValidPurpose(tt.purpose)
			assert.Equal(t, tt.valid, result)
		})
	}
}
