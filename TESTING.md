# Testing Guide - Variably Go SDK

This guide covers how to test the Variably Go SDK in different scenarios.

## 🏃‍♂️ Quick Start

### Unit Tests Only (Fast)
```bash
go test -v ./... -short
```
- Runs mocked unit tests with `-short` flag
- No backend servers required
- Fast feedback for development

### Integration Tests (Full E2E)
```bash
go test -v ./... -run TestIntegration
```
- Tests against real backend services
- Requires local/remote servers running
- Validates actual SDK functionality

## 📋 Available Test Commands

| Command | Description | Requirements | Speed |
|---------|-------------|--------------|-------|
| `go test -v ./... -short` | Unit tests only | None | ⚡ Fast |
| `go test -v ./... -run TestIntegration` | Integration tests only | Backend servers | 🐌 Slow |
| `go test -v ./...` | All tests together | Backend servers | 🐌 Slow |
| `go test -v -cover ./...` | Coverage report | Backend servers | 🐌 Slow |
| `go test -race -v ./...` | Race condition detection | Varies | 🐌 Slow |

## 🔧 Local Development Setup

### Prerequisites
1. **Go 1.19+**
2. **Go Backend Server** (port 8080)
3. **GraphQL Server** (port 4000)
4. **Valid API Key**

### Installation
```bash
# Install dependencies
go mod tidy

# Build without tests
go build ./...
```

### Environment Variables
```bash
export VARIABLY_API_KEY="vb_dev_a639990d4cae487167e9145d0bd6bc9782272d55adb477bd42c2ab6c7cd7b59e"
export VARIABLY_BASE_URL="http://localhost:4000"
export VARIABLY_ENVIRONMENT="development"
```

### Starting Local Servers
```bash
# Terminal 1: Start Go backend
cd ../../go/experimentation-platform
go run cmd/main.go

# Terminal 2: Start GraphQL server  
cd ../../graphql
npm start

# Terminal 3: Run integration tests
cd sdks/go
go test -v ./... -run TestIntegration
```

## 🌐 Production Testing

### Using Live Environment
```bash
export VARIABLY_API_KEY="vb_live_ce1dbda0271bb13ffdc94598bd8466842ee661795a857e48e8bff3ec81839406"
export VARIABLY_BASE_URL="https://graphql.variably.dev"
export VARIABLY_ENVIRONMENT="production"

go test -v ./... -run TestIntegration
```

## 📊 Test Structure

### Unit Tests (`client_test.go`)
- Mock all HTTP dependencies using `httptest`
- Test SDK logic, validation, caching
- Fast execution, no network calls
- Use `-short` flag to skip integration tests

### Integration Tests (`example_test.go`)  
- Real backend communication via GraphQL
- End-to-end functionality validation
- Network calls to actual services
- Use `testing.Short()` to conditionally skip

## 🧪 Test Scenarios

### Flag Evaluation
```go
package main

import (
    "context"
    "github.com/variably/go-sdk"
)

func main() {
    client := variably.NewClient(config)
    ctx := context.Background()
    userContext := variably.UserContext{
        UserID: "user123",
        Email:  "test@example.com",
    }

    // Boolean flags
    isEnabled, err := client.EvaluateFlagBool(ctx, "feature_x", false, userContext)

    // String flags  
    theme, err := client.EvaluateFlagString(ctx, "ui_theme", "light", userContext)

    // Number flags
    limit, err := client.EvaluateFlagNumber(ctx, "rate_limit", 100, userContext)

    // JSON flags
    config, err := client.EvaluateFlagJSON(ctx, "app_config", map[string]interface{}{}, userContext)
}
```

### Event Tracking
```go
// Single event
event := variably.Event{
    Name:   "user_action",
    UserID: "user123",
    Properties: map[string]interface{}{
        "action":  "click",
        "element": "button",
    },
}
err := client.Track(ctx, event)

// Batch events
err := client.TrackBatch(ctx, []variably.Event{event1, event2, event3})
```

### Gate Evaluation
```go
hasAccess, err := client.EvaluateGate(ctx, "premium_features", userContext)
```

## 🐛 Debugging Tests

### Verbose Output
```bash
go test -v ./...
```

### Test Individual Scenarios
```bash
# Run specific test function
go test -v -run TestEvaluateFlagBool

# Run tests matching pattern
go test -v -run "TestFlag.*"

# Run with timeout
go test -v -timeout 30s ./...
```

### Debug Logging
```go
import "log"

func TestWithDebug(t *testing.T) {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    // Test implementation
}
```

## 📈 Performance Testing

### Benchmarks
```bash
# Run benchmarks
go test -bench=. -benchmem

# Run specific benchmark
go test -bench=BenchmarkEvaluateFlag -benchmem

# Profile CPU usage
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Race Detection
```bash
go test -race -v ./...
```

## 🚀 CI/CD Integration

### GitHub Actions Example
```yaml
- name: Setup Go
  uses: actions/setup-go@v4
  with:
    go-version: '1.19'

- name: Cache Go modules
  uses: actions/cache@v3
  with:
    path: ~/go/pkg/mod
    key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

- name: Install dependencies
  run: go mod download

- name: Run Unit Tests
  run: go test -v ./... -short

- name: Run Integration Tests  
  env:
    VARIABLY_API_KEY: ${{ secrets.VARIABLY_TEST_API_KEY }}
    VARIABLY_BASE_URL: ${{ secrets.VARIABLY_BASE_URL }}
  run: go test -v ./... -run TestIntegration

- name: Race Detection
  run: go test -race -short ./...
```

## 📝 Development Workflow

1. **Write/modify code**
2. **Run unit tests** (fast feedback)
   ```bash
   go test -v ./... -short
   ```
3. **Run integration tests** (full validation)
   ```bash
   go test -v ./... -run TestIntegration
   ```
4. **Check coverage** (before PR)
   ```bash
   go test -v -cover ./...
   ```

## 🔍 Test Coverage

### Generate Coverage Report
```bash
# Generate coverage profile
go test -v -coverprofile=coverage.out ./...

# View coverage report
go tool cover -html=coverage.out

# Show coverage percentage
go tool cover -func=coverage.out
```

### Coverage Targets
- **Statements**: > 80%
- **Functions**: > 85%
- **Lines**: > 80%

### Example Test Structure
```go
func TestEvaluateFlagBool(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    tests := []struct {
        name     string
        flagKey  string
        default  bool
        want     bool
        wantErr  bool
    }{
        {"existing flag", "test_flag", false, true, false},
        {"non-existent flag", "missing_flag", false, false, false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := client.EvaluateFlagBool(ctx, tt.flagKey, tt.default, userContext)
            if (err != nil) != tt.wantErr {
                t.Errorf("EvaluateFlagBool() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("EvaluateFlagBool() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## 🎯 Best Practices

1. **Use table-driven tests** for multiple scenarios
2. **Mock external dependencies** in unit tests
3. **Use `testing.Short()`** to separate unit/integration tests
4. **Add benchmarks** for performance-critical functions
5. **Test error conditions** and edge cases
6. **Use contexts** with timeouts for integration tests