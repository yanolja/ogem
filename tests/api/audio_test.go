package api

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/openai"
)

// TestAudioTranscription_Whisper tests basic audio transcription
func TestAudioTranscription_Whisper(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create a test audio file (mock)
	audioFile, err := os.CreateTemp("", "test-audio-*.mp3")
	require.NoError(t, err)
	defer os.Remove(audioFile.Name())

	request := &openai.AudioTranscriptionRequest{
		File:     audioFile.Name(),
		Model:    "whisper-1",
		Language: strPtr("en"),
	}

	response, err := transcribeAudio(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Text)
}

// TestAudioTranscription_MultipleFormats tests different audio formats
func TestAudioTranscription_MultipleFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	formats := []string{"mp3", "mp4", "mpeg", "mpga", "m4a", "wav", "webm"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			audioFile, err := os.CreateTemp("", "test-audio-*."+format)
			require.NoError(t, err)
			defer os.Remove(audioFile.Name())

			request := &openai.AudioTranscriptionRequest{
				File:  audioFile.Name(),
				Model: "whisper-1",
			}

			response, err := transcribeAudio(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
		})
	}
}

// TestAudioTranscription_WithPrompt tests transcription with prompt hint
func TestAudioTranscription_WithPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	audioFile, err := os.CreateTemp("", "test-audio-*.mp3")
	require.NoError(t, err)
	defer os.Remove(audioFile.Name())

	request := &openai.AudioTranscriptionRequest{
		File:   audioFile.Name(),
		Model:  "whisper-1",
		Prompt: strPtr("This is a technical presentation about AI"),
	}

	response, err := transcribeAudio(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Text)
}

// TestAudioTranscription_ResponseFormats tests different response formats
func TestAudioTranscription_ResponseFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	audioFile, err := os.CreateTemp("", "test-audio-*.mp3")
	require.NoError(t, err)
	defer os.Remove(audioFile.Name())

	formats := []string{"json", "text", "srt", "verbose_json", "vtt"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			request := &openai.AudioTranscriptionRequest{
				File:           audioFile.Name(),
				Model:          "whisper-1",
				ResponseFormat: strPtr(format),
			}

			response, err := transcribeAudio(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
		})
	}
}

// TestAudioTranscription_WithTimestamps tests verbose JSON with timestamps
func TestAudioTranscription_WithTimestamps(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	audioFile, err := os.CreateTemp("", "test-audio-*.mp3")
	require.NoError(t, err)
	defer os.Remove(audioFile.Name())

	request := &openai.AudioTranscriptionRequest{
		File:              audioFile.Name(),
		Model:             "whisper-1",
		ResponseFormat:    strPtr("verbose_json"),
		// TimestampGranularities not yet in type definition
	}

	response, err := transcribeAudio(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Text)
	// Verbose JSON should include word-level timestamps
}

// TestAudioTranslation_Languages tests audio translation
func TestAudioTranslation_Languages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	audioFile, err := os.CreateTemp("", "test-audio-*.mp3")
	require.NoError(t, err)
	defer os.Remove(audioFile.Name())

	request := &openai.AudioTranslationRequest{
		File:  audioFile.Name(),
		Model: "whisper-1",
	}

	response, err := translateAudio(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Text)
}

// TestTextToSpeech_Voices tests text-to-speech with different voices
func TestTextToSpeech_Voices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	voices := []string{"alloy", "echo", "fable", "onyx", "nova", "shimmer"}

	for _, voice := range voices {
		t.Run(voice, func(t *testing.T) {
			request := &openai.TextToSpeechRequest{
				Model: "tts-1",
				Input: "Hello, this is a test of text to speech.",
				Voice: voice,
			}

			response, err := generateSpeech(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
			assert.NotEmpty(t, response.Data)
		})
	}
}

// TestTextToSpeech_Models tests different TTS models
func TestTextToSpeech_Models(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	models := []string{"tts-1", "tts-1-hd"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			request := &openai.TextToSpeechRequest{
				Model: model,
				Input: "Testing different TTS models.",
				Voice: "alloy",
			}

			response, err := generateSpeech(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
			assert.NotEmpty(t, response.Data)
		})
	}
}

// TestTextToSpeech_Formats tests different audio output formats
func TestTextToSpeech_Formats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	formats := []string{"mp3", "opus", "aac", "flac", "wav", "pcm"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			request := &openai.TextToSpeechRequest{
				Model:          "tts-1",
				Input:          "Testing audio formats.",
				Voice:          "alloy",
				ResponseFormat: strPtr(format),
			}

			response, err := generateSpeech(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
			assert.NotEmpty(t, response.Data)
		})
	}
}

// TestTextToSpeech_Speed tests speech speed control
func TestTextToSpeech_Speed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	speeds := []float32{0.25, 0.5, 1.0, 2.0, 4.0}

	for _, speed := range speeds {
		t.Run("speed_"+string(rune(speed)), func(t *testing.T) {
			request := &openai.TextToSpeechRequest{
				Model: "tts-1",
				Input: "Testing speech speed.",
				Voice: "alloy",
				Speed: &speed,
			}

			response, err := generateSpeech(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, response)
			assert.NotEmpty(t, response.Data)
		})
	}
}

// TestAudio_InvalidInput tests error handling
func TestAudio_InvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		testFn  func(t *testing.T)
		errMsg  string
	}{
		{
			name: "transcription without file",
			testFn: func(t *testing.T) {
				request := &openai.AudioTranscriptionRequest{
					File:  "",
					Model: "whisper-1",
				}
				_, err := transcribeAudio(context.Background(), request)
				require.Error(t, err)
			},
			errMsg: "file is required",
		},
		{
			name: "tts without input",
			testFn: func(t *testing.T) {
				request := &openai.TextToSpeechRequest{
					Model: "tts-1",
					Input: "",
					Voice: "alloy",
				}
				_, err := generateSpeech(context.Background(), request)
				require.Error(t, err)
			},
			errMsg: "input is required",
		},
		{
			name: "tts invalid voice",
			testFn: func(t *testing.T) {
				request := &openai.TextToSpeechRequest{
					Model: "tts-1",
					Input: "test",
					Voice: "invalid-voice",
				}
				_, err := generateSpeech(context.Background(), request)
				require.Error(t, err)
			},
			errMsg: "invalid voice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFn(t)
		})
	}
}

// TestAudio_RateLimiting tests rate limiting for audio APIs
func TestAudio_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement rate limiting test
	t.Skip("Rate limiting test not yet implemented")
}

// TestAudio_Caching tests caching for audio responses
func TestAudio_Caching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement caching test for TTS
	t.Skip("Audio caching test not yet implemented")
}

