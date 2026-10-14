package google

import "testing"

func TestHomeEmailThreadTokenIsPurposeScopedAndStable(t *testing.T) {
	first := homeEmailThreadToken("test-secret-with-enough-entropy", "connection-a", "thread-a")
	if first == "" || first != homeEmailThreadToken("test-secret-with-enough-entropy", "connection-a", "thread-a") {
		t.Fatal("same inputs must produce a stable opaque thread token")
	}
	if first == homeEmailThreadToken("test-secret-with-enough-entropy", "connection-b", "thread-a") {
		t.Fatal("tokens must be scoped to the provider connection")
	}
	if first == homeEmailThreadToken("different-secret-with-enough-entropy", "connection-a", "thread-a") {
		t.Fatal("tokens must change when the root secret changes")
	}
	if first == "thread-a" {
		t.Fatal("token must not expose the raw Gmail thread ID")
	}
}
