# Claude Code Guidelines

## Comment Standards

**Remember: Comments must explain WHY, not WHAT**

- Comments should explain reasons, rationale, or provide examples
- Never describe what code does - if it can be inferred by LLMs/coding assistants, don't comment it
- Focus on business constraints using "need BUT constraint" pattern
- Don't prefix with subjects ("CallSomething does..." → "Does...")
- Only comment things that cannot be inferred from code itself
- Explain the reasoning behind design decisions, not the implementation

## Examples of Good Comments
```go
// Rate limiting disabled during batch processing because upstream API 
// has separate quota pools for batch vs real-time requests
client.DisableRateLimit()

// Virtual keys validated before master key to prioritize user permissions
// over administrative access patterns
if virtualKey != nil { ... }
```

## Examples of Bad Comments
```go
// Set headers for SSE streaming
httpResponse.Header().Set("Content-Type", "text/event-stream")

// Create new virtual key
key := &VirtualKey{...}
```