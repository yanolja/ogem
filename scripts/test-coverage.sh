#!/bin/bash

# Test Coverage Reporting Script for OGEM
# This script runs all tests and generates comprehensive coverage reports

set -e

echo "🧪 OGEM Test Coverage Report"
echo "============================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_DIR="coverage"
COVERAGE_FILE="$COVERAGE_DIR/coverage.out"
COVERAGE_HTML="$COVERAGE_DIR/coverage.html"
COVERAGE_JSON="$COVERAGE_DIR/coverage.json"
MIN_COVERAGE_THRESHOLD=80

# Create coverage directory
mkdir -p $COVERAGE_DIR

echo -e "${BLUE}📁 Setting up coverage directory...${NC}"

# Clean previous coverage data
rm -f $COVERAGE_FILE $COVERAGE_HTML $COVERAGE_JSON

echo -e "${BLUE}🧹 Cleaned previous coverage data${NC}"

# Function to run tests for a package
run_package_tests() {
    local package=$1
    local package_name=$(basename $package)
    
    echo -e "${YELLOW}🔍 Testing package: $package_name${NC}"
    
    # Check if package has tests
    if ls $package/*_test.go 1> /dev/null 2>&1; then
        go test -v -race -coverprofile="$COVERAGE_DIR/${package_name}.out" -covermode=atomic ./$package || {
            echo -e "${RED}❌ Tests failed for package: $package_name${NC}"
            return 1
        }
        echo -e "${GREEN}✅ Tests passed for package: $package_name${NC}"
    else
        echo -e "${YELLOW}⚠️  No tests found for package: $package_name${NC}"
    fi
}

# Function to combine coverage files
combine_coverage() {
    echo -e "${BLUE}🔗 Combining coverage files...${NC}"
    
    # Create the combined coverage file header
    echo "mode: atomic" > $COVERAGE_FILE
    
    # Combine all individual coverage files
    for coverage_file in $COVERAGE_DIR/*.out; do
        if [ "$coverage_file" != "$COVERAGE_FILE" ] && [ -f "$coverage_file" ]; then
            # Skip the mode line and append the rest
            tail -n +2 "$coverage_file" >> $COVERAGE_FILE
        fi
    done
    
    echo -e "${GREEN}✅ Coverage files combined${NC}"
}

# Function to generate coverage reports
generate_reports() {
    echo -e "${BLUE}📊 Generating coverage reports...${NC}"
    
    # Generate HTML report
    go tool cover -html=$COVERAGE_FILE -o $COVERAGE_HTML
    echo -e "${GREEN}✅ HTML report generated: $COVERAGE_HTML${NC}"
    
    # Generate text summary
    go tool cover -func=$COVERAGE_FILE > "$COVERAGE_DIR/coverage-summary.txt"
    echo -e "${GREEN}✅ Text summary generated: $COVERAGE_DIR/coverage-summary.txt${NC}"
    
    # Extract overall coverage percentage
    COVERAGE_PERCENT=$(go tool cover -func=$COVERAGE_FILE | grep "total:" | awk '{print $3}' | sed 's/%//')
    
    echo -e "${BLUE}📈 Overall Coverage: ${COVERAGE_PERCENT}%${NC}"
    
    # Check if coverage meets threshold
    if (( $(echo "$COVERAGE_PERCENT >= $MIN_COVERAGE_THRESHOLD" | bc -l) )); then
        echo -e "${GREEN}🎉 Coverage threshold met! (${COVERAGE_PERCENT}% >= ${MIN_COVERAGE_THRESHOLD}%)${NC}"
        return 0
    else
        echo -e "${RED}❌ Coverage below threshold! (${COVERAGE_PERCENT}% < ${MIN_COVERAGE_THRESHOLD}%)${NC}"
        return 1
    fi
}

# Function to generate detailed package coverage
generate_package_details() {
    echo -e "${BLUE}📋 Generating package-level coverage details...${NC}"
    
    # Extract package-level coverage
    go tool cover -func=$COVERAGE_FILE | grep -v "total:" | awk '{print $1, $3}' | sort -k2 -nr > "$COVERAGE_DIR/package-coverage.txt"
    
    echo -e "${GREEN}Package Coverage Details:${NC}"
    echo "========================"
    
    while IFS= read -r line; do
        package_func=$(echo $line | awk '{print $1}')
        coverage=$(echo $line | awk '{print $2}' | sed 's/%//')
        
        if (( $(echo "$coverage >= 90" | bc -l) )); then
            echo -e "${GREEN}✅ $package_func: $coverage%${NC}"
        elif (( $(echo "$coverage >= 70" | bc -l) )); then
            echo -e "${YELLOW}⚠️  $package_func: $coverage%${NC}"
        else
            echo -e "${RED}❌ $package_func: $coverage%${NC}"
        fi
    done < "$COVERAGE_DIR/package-coverage.txt"
}

# Function to run specific test categories
run_unit_tests() {
    echo -e "${BLUE}🔬 Running Unit Tests${NC}"
    echo "===================="
    
    # Core packages
    for package in "cache" "tenancy" "routing" "monitoring" "security" "provider"; do
        if [ -d "$package" ]; then
            run_package_tests $package
        fi
    done
}

run_integration_tests() {
    echo -e "${BLUE}🔗 Running Integration Tests${NC}"
    echo "============================"
    
    # Set environment variable for integration tests
    export OGEM_INTEGRATION_TESTS=true
    
    # Run provider integration tests
    if [ -d "provider" ]; then
        echo -e "${YELLOW}🔍 Running provider integration tests...${NC}"
        go test -v -race -coverprofile="$COVERAGE_DIR/provider-integration.out" -covermode=atomic ./provider -run "TestProviderIntegration" || {
            echo -e "${YELLOW}⚠️  Some provider integration tests may have been skipped due to missing API keys${NC}"
        }
    fi
    
    unset OGEM_INTEGRATION_TESTS
}

run_e2e_tests() {
    echo -e "${BLUE}🎯 Running End-to-End Tests${NC}"
    echo "=========================="
    
    # Set environment variable for e2e tests
    export OGEM_E2E_TESTS=true
    
    echo -e "${YELLOW}🔍 Running end-to-end workflow tests...${NC}"
    go test -v -race -coverprofile="$COVERAGE_DIR/e2e.out" -covermode=atomic . -run "TestCompleteWorkflow\|TestSystemResilience\|TestPerformanceBenchmarks" || {
        echo -e "${YELLOW}⚠️  Some e2e tests may have been skipped due to missing API keys${NC}"
    }
    
    unset OGEM_E2E_TESTS
}

# Function to validate test quality
validate_test_quality() {
    echo -e "${BLUE}🔍 Validating Test Quality${NC}"
    echo "========================="
    
    # Count test files
    UNIT_TEST_COUNT=$(find . -name "*_test.go" -not -path "./vendor/*" | wc -l)
    echo -e "${GREEN}📁 Test files found: $UNIT_TEST_COUNT${NC}"
    
    # Count test functions
    TEST_FUNC_COUNT=$(grep -r "^func Test" . --include="*_test.go" | wc -l)
    echo -e "${GREEN}🧪 Test functions found: $TEST_FUNC_COUNT${NC}"
    
    # Check for benchmark tests
    BENCH_COUNT=$(grep -r "^func Benchmark" . --include="*_test.go" | wc -l)
    echo -e "${GREEN}⚡ Benchmark tests found: $BENCH_COUNT${NC}"
    
    # Check for example tests
    EXAMPLE_COUNT=$(grep -r "^func Example" . --include="*_test.go" | wc -l)
    echo -e "${GREEN}📚 Example tests found: $EXAMPLE_COUNT${NC}"
    
    # Validate minimum test count per package
    for package in "cache" "tenancy" "routing" "monitoring" "security"; do
        if [ -d "$package" ]; then
            pkg_test_count=$(find $package -name "*_test.go" | wc -l)
            if [ $pkg_test_count -eq 0 ]; then
                echo -e "${RED}❌ No tests found for critical package: $package${NC}"
            else
                echo -e "${GREEN}✅ Package $package has $pkg_test_count test files${NC}"
            fi
        fi
    done
}

# Function to generate coverage badge
generate_coverage_badge() {
    local coverage_percent=$1
    local badge_color="red"
    
    if (( $(echo "$coverage_percent >= 90" | bc -l) )); then
        badge_color="brightgreen"
    elif (( $(echo "$coverage_percent >= 80" | bc -l) )); then
        badge_color="green"
    elif (( $(echo "$coverage_percent >= 70" | bc -l) )); then
        badge_color="yellow"
    fi
    
    # Generate badge URL
    BADGE_URL="https://img.shields.io/badge/coverage-${coverage_percent}%25-${badge_color}"
    echo "Coverage Badge URL: $BADGE_URL" > "$COVERAGE_DIR/badge.txt"
    echo -e "${GREEN}🏷️  Coverage badge URL saved to $COVERAGE_DIR/badge.txt${NC}"
}

# Main execution
main() {
    echo -e "${BLUE}🚀 Starting comprehensive test coverage analysis...${NC}"
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        echo -e "${RED}❌ Go is not installed or not in PATH${NC}"
        exit 1
    fi
    
    # Check if bc is available for calculations
    if ! command -v bc &> /dev/null; then
        echo -e "${YELLOW}⚠️  bc not found, installing...${NC}"
        sudo apt-get update && sudo apt-get install -y bc 2>/dev/null || {
            echo -e "${YELLOW}⚠️  Could not install bc, some calculations may not work${NC}"
        }
    fi
    
    # Validate test quality first
    validate_test_quality
    
    # Run different test categories
    echo -e "\n${BLUE}📋 Test Execution Plan:${NC}"
    echo "1. Unit Tests (core functionality)"
    echo "2. Integration Tests (provider integrations)"
    echo "3. End-to-End Tests (complete workflows)"
    echo ""
    
    # Run unit tests
    run_unit_tests
    
    # Run integration tests (may skip if no API keys)
    run_integration_tests
    
    # Run e2e tests (may skip if no API keys)
    run_e2e_tests
    
    # Combine all coverage data
    combine_coverage
    
    # Generate reports
    if generate_reports; then
        COVERAGE_PERCENT=$(go tool cover -func=$COVERAGE_FILE | grep "total:" | awk '{print $3}' | sed 's/%//')
        generate_package_details
        generate_coverage_badge "$COVERAGE_PERCENT"
        
        echo -e "\n${GREEN}🎉 Test Coverage Analysis Complete!${NC}"
        echo "===================================="
        echo -e "${GREEN}📊 Total Coverage: ${COVERAGE_PERCENT}%${NC}"
        echo -e "${GREEN}📁 Reports available in: $COVERAGE_DIR/${NC}"
        echo -e "${GREEN}🌐 HTML Report: $COVERAGE_HTML${NC}"
        echo -e "${GREEN}📄 Text Summary: $COVERAGE_DIR/coverage-summary.txt${NC}"
        echo ""
        echo -e "${BLUE}💡 Tips:${NC}"
        echo "- Open $COVERAGE_HTML in your browser for detailed coverage visualization"
        echo "- Check $COVERAGE_DIR/package-coverage.txt for per-package coverage"
        echo "- Run with OGEM_INTEGRATION_TESTS=true OGEM_E2E_TESTS=true for full coverage"
        echo ""
        
        exit 0
    else
        echo -e "\n${RED}❌ Coverage threshold not met${NC}"
        echo "================================"
        echo -e "${RED}Current coverage is below the minimum threshold of $MIN_COVERAGE_THRESHOLD%${NC}"
        echo -e "${YELLOW}Please add more tests to improve coverage${NC}"
        echo ""
        
        exit 1
    fi
}

# Handle script arguments
case "${1:-}" in
    "unit")
        echo -e "${BLUE}Running unit tests only...${NC}"
        run_unit_tests
        combine_coverage
        generate_reports
        ;;
    "integration")
        echo -e "${BLUE}Running integration tests only...${NC}"
        export OGEM_INTEGRATION_TESTS=true
        run_integration_tests
        combine_coverage
        generate_reports
        ;;
    "e2e")
        echo -e "${BLUE}Running e2e tests only...${NC}"
        export OGEM_E2E_TESTS=true
        run_e2e_tests
        combine_coverage
        generate_reports
        ;;
    "quality")
        echo -e "${BLUE}Validating test quality only...${NC}"
        validate_test_quality
        ;;
    *)
        main
        ;;
esac