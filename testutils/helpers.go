package testutils

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestApp provides a configured Cargo app for testing
type TestApp struct {
	App      interface{} // Will be *cargo.App in actual tests
	Server   *grpc.Server
	Cleanup  func()
	MongoURI string
	Port     string
}

// NewTestApp creates a new test application instance
func NewTestApp(t *testing.T) *TestApp {
	// This would create a test instance of cargo.App
	// with test-specific configuration
	return &TestApp{
		MongoURI: "mongodb://localhost:27017/cargo_test",
		Port:     ":0", // Random available port
	}
}

// SetupTestMongoDB creates a test MongoDB connection
func SetupTestMongoDB(t *testing.T) (*mongo.Client, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return nil, nil
	}

	// Test connection
	err = client.Ping(ctx, nil)
	if err != nil {
		t.Skipf("MongoDB ping failed: %v", err)
		return nil, nil
	}

	cleanup := func() {
		// Drop test database
		dbName := "cargo_test"
		client.Database(dbName).Drop(context.Background())
		client.Disconnect(context.Background())
	}

	return client, cleanup
}

// CreateTestContext creates a test context with timeout
func CreateTestContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// AssertNoError fails the test if error is not nil
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

// AssertError fails the test if error is nil
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

// AssertEqual compares two values for equality
func AssertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if expected != actual {
		t.Fatalf("Expected %v, got %v", expected, actual)
	}
}

// AssertNotEqual compares two values for inequality
func AssertNotEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if expected == actual {
		t.Fatalf("Expected values to be different, both are %v", expected)
	}
}

// AssertContains checks if a string contains a substring
func AssertContains(t *testing.T, str, substr string) {
	t.Helper()
	if !contains(str, substr) {
		t.Fatalf("Expected %q to contain %q", str, substr)
	}
}

// AssertTrue fails if condition is false
func AssertTrue(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Fatalf("Expected true: %s", message)
	}
}

// AssertFalse fails if condition is true
func AssertFalse(t *testing.T, condition bool, message string) {
	t.Helper()
	if condition {
		t.Fatalf("Expected false: %s", message)
	}
}

// WaitForCondition waits for a condition to be true with timeout
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if condition() {
				return
			}
		case <-timer.C:
			t.Fatalf("Timeout waiting for condition: %s", message)
		}
	}
}

// NewTestGRPCClient creates a test gRPC client connection
func NewTestGRPCClient(t *testing.T, addr string) (*grpc.ClientConn, func()) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to create gRPC client: %v", err)
	}

	cleanup := func() {
		conn.Close()
	}

	return conn, cleanup
}

// contains is a helper function to check if string contains substring
func contains(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestData contains common test data
type TestData struct {
	ValidJWTSecret   string
	InvalidJWTSecret string
	TestUserID       string
	TestEmail        string
	TestPassword     string
	TestTodoTitle    string
	TestTodoDesc     string
}

// NewTestData returns common test data
func NewTestData() *TestData {
	return &TestData{
		ValidJWTSecret:   "test-secret-key-123",
		InvalidJWTSecret: "invalid-secret",
		TestUserID:       "test-user-123",
		TestEmail:        "test@example.com",
		TestPassword:     "password123",
		TestTodoTitle:    "Test Todo",
		TestTodoDesc:     "Test Todo Description",
	}
}

// GenerateTestClaims creates test JWT claims
func GenerateTestClaims() map[string]interface{} {
	return map[string]interface{}{
		"user_id": "test-user-123",
		"email":   "test@example.com",
		"role":    "user",
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
}

// MockTime provides controllable time for testing
type MockTime struct {
	current time.Time
}

// NewMockTime creates a new mock time instance
func NewMockTime(initial time.Time) *MockTime {
	return &MockTime{current: initial}
}

// Now returns the current mock time
func (m *MockTime) Now() time.Time {
	return m.current
}

// Advance moves the mock time forward
func (m *MockTime) Advance(duration time.Duration) {
	m.current = m.current.Add(duration)
}

// Reset resets the mock time to a specific time
func (m *MockTime) Reset(t time.Time) {
	m.current = t
}
