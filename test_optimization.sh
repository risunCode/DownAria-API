#!/bin/bash

# Download Flow Optimization - Test Script
# This script runs benchmarks and validates the optimization implementation

set -e

echo "=================================================="
echo "DownAria-API Download Flow Optimization Test"
echo "=================================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Change to API directory
cd "$(dirname "$0")"

echo -e "${YELLOW}Step 1: Checking Go installation...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go version: $(go version)${NC}"
echo ""

echo -e "${YELLOW}Step 2: Running go mod tidy...${NC}"
go mod tidy
echo -e "${GREEN}✓ Dependencies updated${NC}"
echo ""

echo -e "${YELLOW}Step 3: Running unit tests...${NC}"
go test ./internal/infra/network/ -v -run TestStreamingDownloader
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Unit tests passed${NC}"
else
    echo -e "${RED}✗ Unit tests failed${NC}"
    exit 1
fi
echo ""

echo -e "${YELLOW}Step 4: Running benchmarks (V1 vs V2)...${NC}"
echo ""
echo "=== Small File (1MB) ==="
go test -bench=BenchmarkStreamingDownloader.*SmallFile -benchmem ./internal/infra/network/ | grep -E "Benchmark|ns/op|MB/s"
echo ""

echo "=== Medium File (50MB) ==="
go test -bench=BenchmarkStreamingDownloader.*MediumFile -benchmem ./internal/infra/network/ | grep -E "Benchmark|ns/op|MB/s"
echo ""

echo "=== Large File (500MB) ==="
go test -bench=BenchmarkStreamingDownloader.*LargeFile -benchmem ./internal/infra/network/ | grep -E "Benchmark|ns/op|MB/s"
echo ""

echo -e "${YELLOW}Step 5: Running concurrent download benchmarks...${NC}"
go test -bench=BenchmarkConcurrentDownloads -benchmem ./internal/infra/network/ | grep -E "Benchmark|ns/op|MB/s"
echo ""

echo -e "${YELLOW}Step 6: Running memory usage benchmarks...${NC}"
go test -bench=BenchmarkMemoryUsage -benchmem ./internal/infra/network/ | grep -E "Benchmark|B/op|allocs/op"
echo ""

echo -e "${YELLOW}Step 7: Running throughput benchmarks...${NC}"
go test -bench=BenchmarkThroughput -benchmem ./internal/infra/network/ | grep -E "Benchmark|MB/s"
echo ""

echo -e "${YELLOW}Step 8: Testing adaptive buffer sizing...${NC}"
go test -bench=BenchmarkBufferPoolAdaptive -benchmem ./internal/infra/network/ | grep -E "Benchmark|ns/op"
echo ""

echo "=================================================="
echo -e "${GREEN}All tests completed successfully!${NC}"
echo "=================================================="
echo ""
echo "Next steps:"
echo "1. Review benchmark results above"
echo "2. Set USE_OPTIMIZED_STREAMING=true in .env.local"
echo "3. Start the server: go run ./cmd/server"
echo "4. Monitor metrics and performance"
echo ""
echo "For detailed documentation, see OPTIMIZATION_SUMMARY.md"
