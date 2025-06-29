# 🚀 Model Update Plan: Latest AI Models Migration

## 📊 **Current Situation Analysis**

Based on comprehensive codebase analysis, we have **200+ references** to deprecated models across:
- Configuration files (15+ files)
- Test files (25+ files) 
- SDK implementations (Go, JavaScript, Python)
- Provider modules (Claude, OpenAI, Google)
- Documentation and examples
- Cost calculation modules

## 🎯 **Target Model Mapping**

### **OpenAI Models**
```yaml
# DEPRECATED → LATEST
gpt-4                    → gpt-4o              # Current flagship
gpt-4-turbo-preview      → gpt-4o              # Unified model
gpt-4-vision-preview     → gpt-4o              # Vision built-in
gpt-4-32k                → gpt-4o              # Higher context
gpt-3.5-turbo            → gpt-4o-mini         # Fast/cheap alternative
gpt-3.5-turbo-16k        → gpt-4o-mini         # Context upgrade

# NEW MODELS TO ADD
gpt-4o                   # Current flagship (128k context, vision, tools)
gpt-4o-mini              # Fast and cheap (128k context)
gpt-4.1                  # Next generation (when available)
gpt-4.5                  # Advanced version (when available)
o3                       # Reasoning model (when available)
o4                       # Advanced reasoning (when available)
```

### **Anthropic Claude Models**
```yaml
# DEPRECATED → LATEST
claude-3-opus-20240229   → claude-3-5-sonnet-20241022    # Best performance
claude-3-sonnet-20240229 → claude-3-5-sonnet-20241022    # Balanced
claude-3-haiku-20240307  → claude-3.5-haiku-20241022     # Fast/cheap
claude-2.1               → claude-3-5-sonnet-20241022    # Major upgrade
claude-2                 → claude-3-5-sonnet-20241022    # Major upgrade
claude-instant-1.2       → claude-3.5-haiku-20241022     # Fast alternative

# CURRENT MODELS
claude-3-5-sonnet-20241022  # Latest flagship
claude-3.5-haiku-20241022   # Fast and economical
claude-4                    # Next generation (when available)
```

### **Google Gemini Models**
```yaml
# DEPRECATED → LATEST
gemini-pro               → gemini-2.5-pro               # Flagship upgrade
gemini-1.0-pro           → gemini-2.5-pro               # Major upgrade
gemini-1.5-pro           → gemini-2.5-pro               # Latest version
gemini-1.5-pro-002       → gemini-2.5-pro               # Latest stable
gemini-pro-vision        → gemini-2.5-pro               # Vision built-in
gemini-1.5-flash         → gemini-2.5-flash             # Fast upgrade
gemini-1.5-flash-002     → gemini-2.5-flash             # Latest flash
gemini-2.0-flash-exp     → gemini-2.5-flash             # Stable upgrade

# CURRENT MODELS (2025)
gemini-2.5-pro          # Flagship: Most intelligent, 1M context (→2M), leads leaderboards
gemini-2.5-flash        # Workhorse: Fast performance, $0.10/$0.60 per 1M tokens
gemini-2.5-flash-lite   # Economy: Most cost-efficient, high-throughput optimized
gemini-2.0-flash        # Experimental: Low latency, 2x faster than 1.5 Pro
gemini-2.5-pro-deep-think # Experimental: Enhanced reasoning (trusted testers only)
```

## 📋 **Implementation Strategy**

### **Phase 1: Core Configuration Update** ⚡
```bash
# Priority: HIGH - Update core model definitions
1. Update /data/nas-2/seungduk/ogem/config.yaml
2. Update /data/nas-2/seungduk/ogem/cost/cost.go
3. Update provider model mappings
4. Update SDK constants
```

### **Phase 2: Test Suite Modernization** 🧪
```bash
# Priority: HIGH - Ensure all tests use current models
1. Update integration test models
2. Update unit test fixtures
3. Update mock data
4. Update example configurations
```

### **Phase 3: SDK and Documentation** 📚
```bash
# Priority: MEDIUM - Update public interfaces
1. Update SDK examples (Go, JS, Python)
2. Update README.md examples
3. Update API documentation
4. Update .env.template defaults
```

### **Phase 4: Provider Implementations** 🔧
```bash
# Priority: MEDIUM - Update provider-specific code
1. Update model normalization functions
2. Update provider-specific model lists
3. Update cost calculation mappings
4. Update capability mappings
```

## 🛠️ **Detailed Implementation Plan**

### **Step 1: Create Model Constants File**
```go
// models/latest.go
package models

const (
    // OpenAI Latest Models
    GPT4O        = "gpt-4o"
    GPT4OMini    = "gpt-4o-mini"
    GPT41        = "gpt-4.1"        // Future
    GPT45        = "gpt-4.5"        // Future
    O3           = "o3"             // Future
    O4           = "o4"             // Future
    
    // Claude Latest Models
    Claude35Sonnet = "claude-3-5-sonnet-20241022"
    Claude35Haiku  = "claude-3.5-haiku-20241022"
    Claude4        = "claude-4"     // Future
    
    // Gemini Latest Models
    Gemini25Pro      = "gemini-2.5-pro"           // Flagship
    Gemini25Flash    = "gemini-2.5-flash"         // Workhorse
    Gemini25FlashLite = "gemini-2.5-flash-lite"   // Economy
    Gemini20Flash    = "gemini-2.0-flash"         // Experimental
)

// Model Categories
var (
    FlagshipModels = []string{GPT4O, Claude35Sonnet, Gemini25Pro}
    FastModels     = []string{GPT4OMini, Claude35Haiku, Gemini25Flash}
    EconomyModels  = []string{GPT4OMini, Claude35Haiku, Gemini25FlashLite}
    ReasoningModels = []string{O3, O4} // Future
)
```

### **Step 2: Update Core Configuration**
```yaml
# config.yaml updates
models:
  openai:
    flagship: "gpt-4o"
    fast: "gpt-4o-mini"
    vision: "gpt-4o"  # Vision built-in
  anthropic:
    flagship: "claude-3-5-sonnet-20241022"
    fast: "claude-3.5-haiku-20241022"
  google:
    flagship: "gemini-2.5-pro"
    fast: "gemini-2.5-flash"
    economy: "gemini-2.5-flash-lite"
    experimental: "gemini-2.0-flash"
```

### **Step 3: Cost Calculation Updates**
```go
// cost/cost.go updates
var ModelPricing = map[string]ModelCost{
    // OpenAI Latest Pricing
    "gpt-4o": {
        InputCost:  0.0025,  // $2.50 per 1M tokens
        OutputCost: 0.01,    // $10.00 per 1M tokens
    },
    "gpt-4o-mini": {
        InputCost:  0.00015, // $0.15 per 1M tokens
        OutputCost: 0.0006,  // $0.60 per 1M tokens
    },
    // Claude Latest Pricing
    "claude-3-5-sonnet-20241022": {
        InputCost:  0.003,   // $3.00 per 1M tokens
        OutputCost: 0.015,   // $15.00 per 1M tokens
    },
    // Gemini Latest Pricing (2025)
    "gemini-2.5-pro": {
        InputCost:  0.00125, // $1.25 per 1M tokens (estimated)
        OutputCost: 0.005,   // $5.00 per 1M tokens (estimated)
    },
    "gemini-2.5-flash": {
        InputCost:  0.0001,  // $0.10 per 1M tokens
        OutputCost: 0.0006,  // $0.60 per 1M tokens
    },
    "gemini-2.5-flash-lite": {
        InputCost:  0.00005, // $0.05 per 1M tokens (estimated)
        OutputCost: 0.0003,  // $0.30 per 1M tokens (estimated)
    },
}
```

### **Step 4: Test Updates**
```go
// Update all test files to use latest models
func TestExampleWithLatestModels(t *testing.T) {
    request := &openai.ChatCompletionRequest{
        Model: "gpt-4o", // Updated from gpt-4
        Messages: []openai.Message{...},
    }
    // ... rest of test
}
```

### **Step 5: SDK Examples Update**
```javascript
// SDK JavaScript example
const response = await ogem.chat.completions.create({
  model: "gpt-4o", // Updated from gpt-4
  messages: [{"role": "user", "content": "Hello!"}]
});
```

## 🔄 **Migration Scripts**

### **Automated Find & Replace Script**
```bash
#!/bin/bash
# migrate_models.sh

# OpenAI model updates
find . -type f -name "*.go" -o -name "*.js" -o -name "*.py" -o -name "*.yaml" -o -name "*.md" | \
xargs sed -i 's/gpt-4"/gpt-4o"/g' 
xargs sed -i 's/gpt-3.5-turbo/gpt-4o-mini/g'
xargs sed -i 's/gpt-4-vision-preview/gpt-4o/g'

# Claude model updates  
find . -type f -name "*.go" -o -name "*.js" -o -name "*.py" -o -name "*.yaml" -o -name "*.md" | \
xargs sed -i 's/claude-3-opus-20240229/claude-3-5-sonnet-20241022/g'
xargs sed -i 's/claude-3-sonnet-20240229/claude-3-5-sonnet-20241022/g'
xargs sed -i 's/claude-3-haiku-20240307/claude-3.5-haiku-20241022/g'

# Gemini model updates
find . -type f -name "*.go" -o -name "*.js" -o -name "*.py" -o -name "*.yaml" -o -name "*.md" | \
xargs sed -i 's/gemini-pro"/gemini-2.5-pro"/g'
xargs sed -i 's/gemini-1.0-pro/gemini-2.5-pro/g'
xargs sed -i 's/gemini-1.5-pro/gemini-2.5-pro/g'
xargs sed -i 's/gemini-1.5-flash/gemini-2.5-flash/g'
xargs sed -i 's/gemini-2.0-flash-exp/gemini-2.5-flash/g'
```

### **Validation Script**
```bash
#!/bin/bash
# validate_models.sh

echo "🔍 Checking for remaining deprecated models..."

# Check for deprecated OpenAI models
echo "Deprecated OpenAI models:"
grep -r "gpt-4\"" . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git
grep -r "gpt-3.5-turbo" . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git

# Check for deprecated Claude models  
echo "Deprecated Claude models:"
grep -r "claude-3-" . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git
grep -r "claude-2" . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git

# Check for deprecated Gemini models
echo "Deprecated Gemini models:"
grep -r "gemini-pro\"" . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git
grep -r "gemini-1\." . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git
grep -r "gemini-2.0-flash-exp" . --include="*.go" --include="*.js" --include="*.py" --exclude-dir=.git
```

## ✅ **Execution Checklist**

### **Pre-Migration**
- [ ] Backup current codebase
- [ ] Create feature branch: `feature/update-latest-models`
- [ ] Document current model usage patterns
- [ ] Review latest model pricing and capabilities

### **Migration Execution**
- [ ] **Phase 1**: Update core configuration files
- [ ] **Phase 2**: Update cost calculation modules  
- [ ] **Phase 3**: Update provider implementations
- [ ] **Phase 4**: Update test suites
- [ ] **Phase 5**: Update SDK examples and documentation
- [ ] **Phase 6**: Update .env.template defaults

### **Post-Migration Validation**
- [ ] Run full test suite
- [ ] Validate cost calculations
- [ ] Test provider integrations
- [ ] Review documentation accuracy
- [ ] Performance testing with new models

### **Documentation Updates**
- [ ] Update README.md with latest models
- [ ] Update API documentation
- [ ] Update SDK documentation
- [ ] Create migration guide for users

## 🎯 **Success Criteria**

1. **Zero deprecated model references** in codebase
2. **All tests passing** with latest models
3. **Accurate cost calculations** for new models
4. **Updated documentation** and examples
5. **Provider compatibility** validated
6. **SDK examples working** with latest models

## ⚠️ **Risk Mitigation**

1. **Model Availability**: Some future models (4.1, 4.5, o3, o4) may not be available yet
   - **Solution**: Use feature flags and graceful degradation
   
2. **Cost Changes**: New models may have different pricing
   - **Solution**: Update cost calculations and add budget alerts
   
3. **API Changes**: New models may have different capabilities
   - **Solution**: Update capability mappings and validation
   
4. **Breaking Changes**: Some users may depend on deprecated models
   - **Solution**: Provide migration guide and backward compatibility layer

## 🚀 **Next Steps**

1. **Review and approve** this migration plan
2. **Create migration scripts** for automated updates
3. **Execute Phase 1** (core configurations)
4. **Test and validate** each phase
5. **Deploy incrementally** with feature flags
6. **Monitor** performance and costs post-migration

---

**Total Estimated Effort**: 2-3 days
**Risk Level**: Medium (careful testing required)
**Business Impact**: High (improved performance, reduced costs, future-ready)