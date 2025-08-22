# MCP Server Validation Tests

Simple, standalone tests that validate the OpenCost MCP (Model Context Protocol) server functionality.

## Overview

These tests prove that the MCP server works correctly by:
- Testing JSON-RPC 2.0 protocol compliance
- Validating MCP tool functionality
- Ensuring HTTP endpoint behavior
- Verifying server performance

## Prerequisites

1. **OpenCost MCP Server Running**: The server must be running on `http://localhost:9003`
2. **Go Environment**: Go 1.19+ installed
3. **Network Access**: Ability to make HTTP requests to localhost:9003

## Test Structure

```
test/integration/mcp/
├── mcp_test.go                 # Comprehensive test suite
├── README.md                   # This documentation
└── run_tests.sh               # Test runner script
```

## Test Scenarios

### 1. MCP Server Status Test (`TestMCPServerStatus`)
- Verifies server is running and responsive
- Tests ping endpoint functionality
- Validates response structure

### 2. JSON-RPC Protocol Tests (`TestJSONRPCProtocol`) 
- **Initialize**: Tests server initialization handshake
- **Tools List**: Validates tool discovery and metadata
- **Invalid Method**: Ensures proper error handling

### 3. MCP Tool Functionality Tests (`TestMCPToolFunctionality`)
- **query_allocations**: Tests Kubernetes allocation cost queries
- **query_assets**: Tests asset and utilization queries  
- **query_cloud_costs**: Tests cloud provider cost queries
- **Invalid Tool**: Tests error handling for unknown tools

### 4. HTTP Protocol Compliance (`TestHTTPProtocolCompliance`)
- **Content-Type**: Validates JSON content type requirements
- **HTTP Method**: Ensures only POST requests are accepted
- **JSON-RPC Version**: Tests version validation

### 5. Performance Tests (`TestMCPServerPerformance`)
- **Response Time**: Measures request/response latency
- **Concurrent Requests**: Tests server under concurrent load

### 6. Integration Test (`TestMCPIntegration`)
- **Full Workflow**: End-to-end test of complete MCP interaction

## Running the Tests

### Option 1: Using the Test Script
```bash
./run_tests.sh
```

### Option 2: Direct Go Test Command
```bash
# Run all tests
go test -v ./test/integration/mcp/

# Run specific test
go test -v ./test/integration/mcp/ -run TestMCPServerStatus

# Run with timeout
go test -v ./test/integration/mcp/ -timeout 60s
```

### Option 3: From Project Root
```bash
# From opencost-integration-tests directory
go test -v ./test/integration/mcp/
```

## Test Configuration

The tests use these default settings:
- **Server URL**: `http://localhost:9003`
- **MCP Endpoint**: `/mcp`
- **Request Timeout**: 30 seconds
- **Protocol Version**: `2024-11-05`

To modify these, edit the constants in `mcp_test.go`:
```go
const (
    MCPServerURL = "http://localhost:9003"
    MCPEndpoint  = "/mcp"
    Timeout      = 30 * time.Second
)
```

## Expected Output

### Successful Test Run
```
=== RUN   TestMCPServerStatus
    mcp_test.go:87: ✓ MCP server status test passed
--- PASS: TestMCPServerStatus (0.12s)
=== RUN   TestJSONRPCProtocol
=== RUN   TestJSONRPCProtocol/Initialize
    mcp_test.go:137: ✓ Initialize test passed
=== RUN   TestJSONRPCProtocol/ToolsList
    mcp_test.go:179: ✓ Tools list test passed - found 3 tools
=== RUN   TestJSONRPCProtocol/InvalidMethod
    mcp_test.go:192: ✓ Invalid method test passed
--- PASS: TestJSONRPCProtocol (0.45s)
...
PASS
ok      opencost-integration-tests/test/integration/mcp    2.847s
```

### Server Not Running
```
=== RUN   TestMCPServerStatus
    mcp_test.go:72: MCP server is not running - start the server and run tests again
--- SKIP: TestMCPServerStatus (0.00s)
```

## Troubleshooting

### Server Connection Issues
1. **Check server is running**: `curl http://localhost:9003/health`
2. **Verify port**: Ensure MCP server is on port 9003
3. **Check logs**: Review server logs for startup errors

### Test Failures
1. **Timeout errors**: Increase timeout in test configuration
2. **JSON parsing errors**: Check server response format
3. **Protocol errors**: Verify server implements MCP correctly

### Common Issues
- **Port conflicts**: Change server port if 9003 is occupied
- **Firewall blocking**: Allow localhost:9003 access
- **Go version**: Ensure Go 1.19+ is installed

## Test Data Requirements

The tests use minimal test data:
- Simple query strings (no complex data required)
- Basic time windows (1h, 1d, 7d)
- Generic aggregation levels (namespace, type, service)

The server should handle these gracefully even without actual cost data.

## Validation Criteria

Tests verify:
✅ **Protocol Compliance**: JSON-RPC 2.0 adherence  
✅ **Tool Discovery**: All expected tools are listed  
✅ **Error Handling**: Proper error responses  
✅ **Response Structure**: Valid MCP response format  
✅ **Performance**: Reasonable response times  
✅ **Concurrency**: Multiple simultaneous requests  

## Success Metrics

- **All tests pass**: Server is functioning correctly
- **Response times < 5s**: Acceptable performance
- **Proper error codes**: Robust error handling
- **Valid JSON responses**: Correct data format
- **Tool availability**: All 3 cost analysis tools work

## Integration with BATS (Optional)

To integrate with existing BATS tests, create `test.bats`:
```bash
#!/usr/bin/env bats

@test "MCP server functionality" {
    run go test ./test/integration/mcp/ -v
    [ "$status" -eq 0 ]
}
```

This allows running MCP tests as part of the broader test suite while maintaining their standalone nature.