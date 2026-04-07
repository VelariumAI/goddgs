package goddgs

import "testing"

func TestNewHTTPClientProxyValidation(t *testing.T) {
	if _, err := NewHTTPClient(0, "://bad"); err == nil {
		t.Fatal("expected error for invalid proxy url")
	}
	if _, err := NewHTTPClient(0, "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("unexpected error for valid proxy: %v", err)
	}
}
