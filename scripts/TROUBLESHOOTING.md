# Troubleshooting Guide

## OpenAI Pricing Page 403 Errors

### Problem
OpenAI's pricing page returns `403 Forbidden` errors when accessed via automated scripts due to aggressive bot detection measures.

### Root Causes
1. **Enhanced Bot Detection**: OpenAI implemented sophisticated anti-bot systems
2. **JavaScript-Rendered Content**: Pricing page uses client-side rendering  
3. **Cloudflare Protection**: Advanced bot protection blocking automated requests
4. **User-Agent Fingerprinting**: Detection of non-browser requests

### Solutions Implemented

#### 1. Multi-Method Approach
The scraper now tries multiple methods in order:
1. **Playwright Browser Automation** - Most effective but requires system dependencies
2. **Enhanced Headers** - Mimics real browser requests
3. **Fallback Data** - Manually maintained pricing as last resort

#### 2. Browser Automation Setup
```bash
# Install Playwright
pip install playwright

# Install Chromium browser
playwright install chromium

# Install system dependencies (requires sudo)
sudo playwright install-deps
# OR manually install missing libs:
sudo apt-get install libasound2
```

#### 3. Enhanced Headers
The scraper uses realistic browser headers including:
- Chrome User-Agent with proper version
- Accept headers matching real browsers
- Security headers (Sec-Fetch-*)
- Cache control and encoding headers

#### 4. Stealth Browser Features
When Playwright is available, the scraper:
- Disables automation detection flags
- Modifies navigator properties
- Uses realistic viewport and user agent
- Waits for JavaScript to render content

### Current Status
- **Playwright**: Requires system dependencies (sudo access needed)
- **Enhanced Headers**: Still blocked by OpenAI (403 error)
- **Fallback Data**: ✅ Working - provides current pricing from June 2025

### Fallback Pricing Data
The scraper includes manually maintained pricing that's updated monthly:
- GPT-4o: $2.5/$10.0 (input/output per 1M tokens)
- GPT-4o Mini: $0.15/$0.6
- GPT-4 Turbo: $10.0/$30.0
- O1 Preview: $15.0/$60.0
- And more...

### Recommendations

#### For Development Environments
1. Use fallback data for consistent results
2. Update fallback pricing monthly from official sources
3. Monitor scraping success rates

#### For Production Use
1. Set up dedicated scraping infrastructure with:
   - Residential proxies
   - Rotating user agents
   - Proper rate limiting
2. Consider manual verification of critical pricing changes
3. Implement alerts when scraping consistently fails

#### Alternative Solutions
1. **OpenAI API**: Check if OpenAI provides pricing via API endpoints
2. **Third-party Services**: Use pricing aggregation services
3. **Manual Process**: Set up monthly manual pricing updates
4. **Community Sources**: Use open-source pricing datasets

### Testing Commands

Test individual providers:
```bash
# Test with fallback (always works)
python scrape_pricing.py --provider openai

# Test all providers
python scrape_pricing.py

# Test with Playwright (if dependencies installed)
python scrape_pricing.py --provider openai --output json
```

### Error Messages and Solutions

| Error | Cause | Solution |
|-------|-------|----------|
| `403 Forbidden` | Bot detection | Use fallback data |
| `Playwright dependencies missing` | System libs not installed | Install with `sudo playwright install-deps` |
| `ImportError: playwright` | Playwright not installed | `pip install playwright` |
| `Timeout` | Network/server issues | Retry or use fallback |

### Future Improvements
1. Add proxy rotation support
2. Implement CAPTCHA solving
3. Use headless browsers with stealth plugins
4. Add monitoring and alerting for pricing changes
5. Create automated fallback data updates