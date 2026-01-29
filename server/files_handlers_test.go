package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/yanolja/ogem/config"
	"github.com/yanolja/ogem/files"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/state"
)

func setupTestServer(t *testing.T) *ModelProxy {
	logger := zap.NewNop().Sugar()
	tmpDir := t.TempDir()

	// Create minimal config
	cfg := &config.Config{
		FileStorageDir: tmpDir,
		RetryInterval:  "1m",
		PingInterval:   "1h",
	}

	// Create state manager
	stateManager, cleanup := state.NewMemoryManager(1000)

	// Create file manager
	fileManager, err := files.NewManager(tmpDir, logger)
	require.NoError(t, err)

	server := &ModelProxy{
		config:      cfg,
		logger:      logger,
		stateManager: stateManager,
		cleanup:     cleanup,
		fileManager: fileManager,
	}

	return server
}

func TestHandleFilesUpload(t *testing.T) {
	server := setupTestServer(t)

	t.Run("uploads file successfully", func(t *testing.T) {
		// Create multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, err := writer.CreateFormFile("file", "test.jsonl")
		require.NoError(t, err)
		part.Write([]byte("test content"))

		writer.WriteField("purpose", "fine-tune")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		server.HandleFilesUpload(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response openai.File
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "test.jsonl", response.Filename)
		assert.Equal(t, "fine-tune", response.Purpose)
	})

	t.Run("uses default purpose", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		part.Write([]byte("test"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		server.HandleFilesUpload(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response openai.File
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "fine-tune", response.Purpose) // Default
	})

	t.Run("fails on missing file", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		writer.WriteField("purpose", "fine-tune")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		server.HandleFilesUpload(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("fails on invalid purpose", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		part.Write([]byte("test"))

		writer.WriteField("purpose", "invalid-purpose")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		server.HandleFilesUpload(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("fails on invalid form", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/files", strings.NewReader("not a form"))
		req.Header.Set("Content-Type", "multipart/form-data")
		w := httptest.NewRecorder()

		server.HandleFilesUpload(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleFilesList(t *testing.T) {
	server := setupTestServer(t)

	t.Run("lists uploaded files", func(t *testing.T) {
		// Upload a file first
		content := strings.NewReader("test content")
		_, err := server.uploadFile(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/v1/files", nil)
		w := httptest.NewRecorder()

		server.HandleFilesList(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response openai.FileList
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "list", response.Object)
		assert.Len(t, response.Data, 1)
		assert.False(t, response.HasMore)
	})

	t.Run("returns empty list", func(t *testing.T) {
		freshServer := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/files", nil)
		w := httptest.NewRecorder()

		freshServer.HandleFilesList(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response openai.FileList
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Empty(t, response.Data)
	})
}

func TestHandleFilesRetrieve(t *testing.T) {
	server := setupTestServer(t)

	t.Run("retrieves file metadata", func(t *testing.T) {
		// Upload a file first
		content := strings.NewReader("test content")
		uploaded, err := server.uploadFile(context.Background(), "test.txt", content, "assistants")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/v1/files/"+uploaded.ID, nil)
		w := httptest.NewRecorder()

		server.HandleFilesRetrieve(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response openai.File
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, uploaded.ID, response.ID)
		assert.Equal(t, "test.txt", response.Filename)
		assert.Equal(t, "assistants", response.Purpose)
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files/nonexistent", nil)
		w := httptest.NewRecorder()

		server.HandleFilesRetrieve(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("fails on missing file ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files/", nil)
		w := httptest.NewRecorder()

		server.HandleFilesRetrieve(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}

func TestHandleFilesDelete(t *testing.T) {
	server := setupTestServer(t)

	t.Run("deletes file successfully", func(t *testing.T) {
		// Upload a file first
		content := strings.NewReader("test content")
		uploaded, err := server.uploadFile(context.Background(), "test.txt", content, "batch")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/v1/files/"+uploaded.ID, nil)
		w := httptest.NewRecorder()

		server.HandleFilesDelete(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response openai.FileDeleteResponse
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, uploaded.ID, response.ID)
		assert.True(t, response.Deleted)

		// Verify file is gone
		_, err = server.retrieveFile(context.Background(), uploaded.ID)
		assert.Error(t, err)
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/files/nonexistent", nil)
		w := httptest.NewRecorder()

		server.HandleFilesDelete(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("fails on missing file ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/files/", nil)
		w := httptest.NewRecorder()

		server.HandleFilesDelete(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}

func TestHandleFilesContent(t *testing.T) {
	server := setupTestServer(t)

	t.Run("downloads file content", func(t *testing.T) {
		expectedContent := "test file content"
		content := strings.NewReader(expectedContent)
		uploaded, err := server.uploadFile(context.Background(), "test.txt", content, "fine-tune")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/v1/files/"+uploaded.ID+"/content", nil)
		w := httptest.NewRecorder()

		server.HandleFilesContent(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
		assert.Equal(t, expectedContent, w.Body.String())
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files/nonexistent/content", nil)
		w := httptest.NewRecorder()

		server.HandleFilesContent(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("fails on missing file ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files//content", nil)
		w := httptest.NewRecorder()

		server.HandleFilesContent(w, req)

		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}

func TestFileManagerMethods(t *testing.T) {
	server := setupTestServer(t)
	ctx := context.Background()

	t.Run("uploadFile", func(t *testing.T) {
		content := strings.NewReader("test content")
		file, err := server.uploadFile(ctx, "test.txt", content, "fine-tune")
		require.NoError(t, err)
		assert.NotEmpty(t, file.ID)
		assert.Equal(t, "test.txt", file.Filename)
	})

	t.Run("listFiles", func(t *testing.T) {
		fileList, err := server.listFiles(ctx)
		require.NoError(t, err)
		assert.NotNil(t, fileList)
		assert.NotEmpty(t, fileList.Data)
	})

	t.Run("retrieveFile", func(t *testing.T) {
		// Get first file from list
		fileList, err := server.listFiles(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, fileList.Data)

		file, err := server.retrieveFile(ctx, fileList.Data[0].ID)
		require.NoError(t, err)
		assert.Equal(t, fileList.Data[0].ID, file.ID)
	})

	t.Run("getFileContent", func(t *testing.T) {
		expectedContent := "content to retrieve"
		content := strings.NewReader(expectedContent)
		uploaded, err := server.uploadFile(ctx, "retrieve.txt", content, "batch")
		require.NoError(t, err)

		retrieved, err := server.getFileContent(ctx, uploaded.ID)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(retrieved))
	})

	t.Run("deleteFile", func(t *testing.T) {
		content := strings.NewReader("to delete")
		uploaded, err := server.uploadFile(ctx, "delete.txt", content, "fine-tune")
		require.NoError(t, err)

		response, err := server.deleteFile(ctx, uploaded.ID)
		require.NoError(t, err)
		assert.True(t, response.Deleted)
		assert.Equal(t, uploaded.ID, response.ID)
	})
}
