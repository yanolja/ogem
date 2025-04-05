package studio

import (
	"context"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/converter/openaigemini"
)

// A unique identifier for the Gemini Studio provider
const REGION = "studio"

type Endpoint struct {
	client *genai.Client
}

func NewEndpoint(apiKey string) (*Endpoint, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return &Endpoint{client: client}, nil
}

func (ep *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	model, err := openaigeminiconverter.GetModelFromOpenAiRequest(ep.client, openaiRequest)
	if err != nil {
		return nil, err
	}

	chat := model.StartChat()
	var messageToSend *genai.Content
	chat.History, messageToSend, err = openaigeminiconverter.ToGeminiRequest(openaiRequest.Messages)
	if err != nil {
		return nil, err
	}

	geminiResponse, err := chat.SendMessage(ctx, messageToSend.Parts...)
	if err != nil {
		return nil, err
	}

	openaiResponse, err := openaigeminiconverter.ToOpenAiResponse(geminiResponse)
	if err != nil {
		return nil, err
	}
	return openai.FinalizeResponse(ep.Provider(), ep.Region(), openaiRequest.Model, openaiResponse), nil
}

func (ep *Endpoint) Provider() string {
	return "studio"
}

func (ep *Endpoint) Region() string {
	return REGION
}

func (ep *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	genModel := ep.client.GenerativeModel("gemini-1.5-flash")
	genModel.MaxOutputTokens = utils.ToPtr(int32(1))

	start := time.Now()
	_, err := genModel.GenerateContent(ctx, genai.Text("Ping"))
	if err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (ep *Endpoint) Shutdown() error {
	return ep.client.Close()
}
