# AI Model Pricing Scraper

This Python script scrapes pricing information from official AI provider websites to keep pricing data up-to-date.

## Supported Providers

- **OpenAI**: https://openai.com/api/pricing/
- **Anthropic**: https://www.anthropic.com/pricing
- **Google Cloud Vertex AI**: https://cloud.google.com/vertex-ai/generative-ai/pricing
- **Azure OpenAI Service**: https://azure.microsoft.com/en-us/pricing/details/cognitive-services/openai-service/

## Installation

```bash
pip install -r requirements.txt
```

## Usage

### Basic Usage

Scrape all providers and output JSON:
```bash
python scrape_pricing.py
```

### Specific Provider

Scrape only OpenAI pricing:
```bash
python scrape_pricing.py --provider openai
```

### Output Formats

Output as YAML:
```bash
python scrape_pricing.py --output yaml
```

Output as Go code for cost.go:
```bash
python scrape_pricing.py --output go
```

### Save to File

Save pricing data to file:
```bash
python scrape_pricing.py --output json --file pricing_data.json
```

### Update Go Pricing Code

Generate Go code and save to file:
```bash
python scrape_pricing.py --output go --file updated_pricing.go
```

## Output Structure

### JSON Format
```json
{
  "scraped_at": "2025-06-20T11:16:02.583914",
  "providers": {
    "openai": {
      "provider": "openai",
      "url": "https://openai.com/api/pricing/",
      "scraped_at": "2025-06-20T11:16:03.148192",
      "models": {
        "gpt-4o": {
          "input_price_per_1m": 2.5,
          "output_price_per_1m": 10.0,
          "source": "openai_api"
        }
      }
    }
  }
}
```

### Go Format
```go
// Auto-generated pricing data
// Generated at: 2025-06-20T11:16:16.175397

var modelPricing = map[string]ModelPricing{
    "gpt-4o": {
        InputTokenPrice:    2.500,
        OutputTokenPrice:   10.000,
    },
}
```

## Features

- **Automatic Rate Limiting**: 2-second delays between provider requests
- **Fallback Data**: Uses known pricing when scraping fails
- **Multiple Formats**: JSON, YAML, and Go code generation
- **Error Handling**: Graceful handling of network errors and parsing failures
- **Reasoning Token Support**: Handles separate reasoning token pricing for models like Gemini 2.5

## Notes

- The script includes fallback pricing data in case web scraping fails
- Some providers may have dynamic content that requires JavaScript rendering
- Pricing data includes both current and legacy model variants
- Azure OpenAI pricing typically differs from standard OpenAI API pricing

## Example Commands

Update pricing in cost.go file:
```bash
# Generate Go pricing code
python scrape_pricing.py --output go > temp_pricing.go

# Review the output and manually merge into cost/cost.go
```

Monitor pricing changes:
```bash
# Save current pricing
python scrape_pricing.py --file current_pricing.json

# Compare with previous pricing file
diff previous_pricing.json current_pricing.json
```