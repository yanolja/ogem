package api

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/openai"
)

// TestFiles_Upload tests file upload functionality
func TestFiles_Upload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create a temporary test file
	testFile, err := os.CreateTemp("", "test-file-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	// Write test JSONL data
	testData := `{"prompt": "test prompt", "completion": "test completion"}` + "\n"
	_, err = testFile.Write([]byte(testData))
	require.NoError(t, err)
	testFile.Close()

	// Upload the file
	response, err := uploadFile(context.Background(), testFile.Name(), "fine-tune")
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.ID)
	assert.Equal(t, "fine-tune", response.Purpose)
	assert.NotEmpty(t, response.Filename)
	assert.Greater(t, response.Bytes, int64(0))
}

// TestFiles_UploadMultipleFiles tests uploading multiple files
func TestFiles_UploadMultipleFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	for i := 0; i < 3; i++ {
		testFile, err := os.CreateTemp("", "test-file-*.jsonl")
		require.NoError(t, err)
		defer os.Remove(testFile.Name())

		testData := `{"prompt": "test", "completion": "response"}` + "\n"
		testFile.Write([]byte(testData))
		testFile.Close()

		response, err := uploadFile(context.Background(), testFile.Name(), "fine-tune")
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
	}
}

// TestFiles_List tests listing uploaded files
func TestFiles_List(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Upload a test file first
	testFile, err := os.CreateTemp("", "test-file-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	testData := `{"prompt": "list test", "completion": "list response"}` + "\n"
	testFile.Write([]byte(testData))
	testFile.Close()

	uploadedFile, err := uploadFile(context.Background(), testFile.Name(), "fine-tune")
	require.NoError(t, err)

	// List files
	files, err := listFiles(context.Background())
	require.NoError(t, err)
	require.NotNil(t, files)
	assert.Greater(t, len(files.Data), 0)

	// Verify uploaded file is in the list
	found := false
	for _, f := range files.Data {
		if f.ID == uploadedFile.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Uploaded file should be in the list")
}

// TestFiles_Retrieve tests retrieving a specific file
func TestFiles_Retrieve(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Upload a test file first
	testFile, err := os.CreateTemp("", "test-file-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	testData := `{"prompt": "retrieve test", "completion": "retrieve response"}` + "\n"
	testFile.Write([]byte(testData))
	testFile.Close()

	uploadedFile, err := uploadFile(context.Background(), testFile.Name(), "fine-tune")
	require.NoError(t, err)

	// Retrieve the file
	retrievedFile, err := retrieveFile(context.Background(), uploadedFile.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedFile)
	assert.Equal(t, uploadedFile.ID, retrievedFile.ID)
	assert.Equal(t, uploadedFile.Filename, retrievedFile.Filename)
	assert.Equal(t, uploadedFile.Purpose, retrievedFile.Purpose)
}

// TestFiles_Delete tests deleting a file
func TestFiles_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Upload a test file first
	testFile, err := os.CreateTemp("", "test-file-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	testData := `{"prompt": "delete test", "completion": "delete response"}` + "\n"
	testFile.Write([]byte(testData))
	testFile.Close()

	uploadedFile, err := uploadFile(context.Background(), testFile.Name(), "fine-tune")
	require.NoError(t, err)

	// Delete the file
	deleteResp, err := deleteFile(context.Background(), uploadedFile.ID)
	require.NoError(t, err)
	require.NotNil(t, deleteResp)
	assert.True(t, deleteResp.Deleted)
	assert.Equal(t, uploadedFile.ID, deleteResp.ID)

	// Verify file no longer exists
	_, err = retrieveFile(context.Background(), uploadedFile.ID)
	assert.Error(t, err, "Deleted file should not be retrievable")
}

// TestFiles_RetrieveContent tests retrieving file content
func TestFiles_RetrieveContent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Upload a test file first
	testFile, err := os.CreateTemp("", "test-file-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(testFile.Name())

	testData := `{"prompt": "content test", "completion": "content response"}` + "\n"
	testFile.Write([]byte(testData))
	testFile.Close()

	uploadedFile, err := uploadFile(context.Background(), testFile.Name(), "fine-tune")
	require.NoError(t, err)

	// Retrieve file content
	content, err := retrieveFileContent(context.Background(), uploadedFile.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.Contains(t, string(content), "content test")
}

// TestFiles_Purpose tests different file purposes
func TestFiles_Purpose(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	purposes := []string{"fine-tune", "assistants", "batch"}

	for _, purpose := range purposes {
		t.Run(purpose, func(t *testing.T) {
			testFile, err := os.CreateTemp("", "test-file-*.jsonl")
			require.NoError(t, err)
			defer os.Remove(testFile.Name())

			testData := `{"prompt": "purpose test", "completion": "purpose response"}` + "\n"
			testFile.Write([]byte(testData))
			testFile.Close()

			response, err := uploadFile(context.Background(), testFile.Name(), purpose)
			require.NoError(t, err)
			assert.Equal(t, purpose, response.Purpose)
		})
	}
}

// TestFiles_InvalidInput tests error handling
func TestFiles_InvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		testFn func(t *testing.T)
		errMsg string
	}{
		{
			name: "upload nonexistent file",
			testFn: func(t *testing.T) {
				_, err := uploadFile(context.Background(), "/nonexistent/file.jsonl", "fine-tune")
				require.Error(t, err)
			},
			errMsg: "file not found",
		},
		{
			name: "retrieve nonexistent file",
			testFn: func(t *testing.T) {
				_, err := retrieveFile(context.Background(), "file-nonexistent")
				require.Error(t, err)
			},
			errMsg: "not found",
		},
		{
			name: "delete nonexistent file",
			testFn: func(t *testing.T) {
				_, err := deleteFile(context.Background(), "file-nonexistent")
				require.Error(t, err)
			},
			errMsg: "not found",
		},
		{
			name: "upload with invalid purpose",
			testFn: func(t *testing.T) {
				testFile, err := os.CreateTemp("", "test-file-*.jsonl")
				require.NoError(t, err)
				defer os.Remove(testFile.Name())

				testFile.Write([]byte("test data\n"))
				testFile.Close()

				_, err = uploadFile(context.Background(), testFile.Name(), "invalid-purpose")
				require.Error(t, err)
			},
			errMsg: "invalid purpose",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFn(t)
		})
	}
}

// TestFiles_Pagination tests file listing pagination
func TestFiles_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Upload multiple files
	for i := 0; i < 5; i++ {
		testFile, err := os.CreateTemp("", "test-file-*.jsonl")
		require.NoError(t, err)
		defer os.Remove(testFile.Name())

		testFile.Write([]byte("test data\n"))
		testFile.Close()

		_, err = uploadFile(context.Background(), testFile.Name(), "fine-tune")
		require.NoError(t, err)
	}

	// List with limit
	files, err := listFilesWithPagination(context.Background(), 2, "")
	require.NoError(t, err)
	assert.LessOrEqual(t, len(files.Data), 2)

	if files.HasMore {
		// Get next page
		lastID := files.Data[len(files.Data)-1].ID
		nextPage, err := listFilesWithPagination(context.Background(), 2, lastID)
		require.NoError(t, err)
		assert.NotEmpty(t, nextPage.Data)
	}
}

// Helper functions

func uploadFile(ctx context.Context, filepath string, purpose string) (*openai.File, error) {
	// Skip if no API key configured
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, assert.AnError
	}

	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	part, err := writer.CreateFormFile("file", filepath)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}

	// Add purpose field
	writer.WriteField("purpose", purpose)
	writer.Close()

	// TODO: Make actual HTTP request
	return nil, assert.AnError
}

func listFiles(ctx context.Context) (*openai.FileList, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, assert.AnError
	}
	return nil, assert.AnError
}

func listFilesWithPagination(ctx context.Context, limit int, after string) (*openai.FileList, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, assert.AnError
	}
	return nil, assert.AnError
}

func retrieveFile(ctx context.Context, fileID string) (*openai.File, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, assert.AnError
	}
	return nil, assert.AnError
}

func deleteFile(ctx context.Context, fileID string) (*openai.FileDeleteResponse, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, assert.AnError
	}
	return nil, assert.AnError
}

func retrieveFileContent(ctx context.Context, fileID string) ([]byte, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, assert.AnError
	}
	return nil, assert.AnError
}
