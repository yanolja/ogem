# Ogem

Ogem is a proxy that allows seamless access to the latest models from OpenAI, Google AI Studio, and Vertex AI using a unified OpenAI-compatible API. You can interact with various models using a single, unified interface.

Ogem consists of two main components:

1. **Pulse Server**: Monitors the status and latency of different AI providers.
2. **Proxy Server**: Handles incoming requests and routes them to the appropriate AI provider.

## Running Ogem locally

Follow these steps to set up and run Ogem on your machine.

1. Ensure you have `Valkey` running locally. Check [https://valkey.io/](https://valkey.io/) for details.

2. For the first run or to update provider status, start the Pulse server by running this command from the repository root:

   ```bash
   go run ./pulse
   ```

3. Start the proxy server by running this command from the repository root:

   ```bash
   OPEN_GEMINI_API_KEY=<your_gemini_api_key> \
   GENAI_STUDIO_API_KEY=<your_genai_studio_api_key> \
   GOOGLE_CLOUD_PROJECT=<your_google_cloud_project_id> \
   OPENAI_API_KEY=<your_openai_api_key> \
   CLAUDE_API_KEY=<your_claude_api_key> \
   VALKEY_ENDPOINT=<your_valkey_endpoint> \
   go run main.go
   ```

   Replace placeholders with your actual API keys and values.

Once Ogem is running locally, you can send requests to it, as shown in the example below:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer OGEM_API_KEY" \
  -d '{
    "model": "gemini-1.5-flash",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'
```

## License

This project is licensed under the terms of the Apache 2.0 license. See the [LICENSE](LICENSE) file for more details.

## Contributing

Before you submit any contributions, please make sure to review and agree to our [Contributor License Agreement](CLA.md).

## Code of Conduct

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before engaging with our community.
