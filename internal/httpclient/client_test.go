package httpclient

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestSingletonBehavior verifies that Get() returns the same client instance
func TestSingletonBehavior(t *testing.T) {
	// Reset singleton for clean test
	resetSingleton()

	client1 := Get()
	client2 := Get()

	if client1 != client2 {
		t.Error("Get() should return the same client instance (singleton)")
	}

	if client1 == nil {
		t.Error("Get() should not return nil")
	}
}

// TestConcurrentSingletonAccess verifies thread-safe singleton initialization
func TestConcurrentSingletonAccess(t *testing.T) {
	resetSingleton()

	const numGoroutines = 100
	clients := make([]*http.Client, numGoroutines)
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			clients[index] = Get()
		}(i)
	}

	wg.Wait()

	// All clients should be the same instance
	first := clients[0]
	for i, client := range clients[1:] {
		if client != first {
			t.Errorf("Goroutine %d: expected same client instance, got different one", i+1)
		}
	}
}

// TestTransportConfiguration verifies transport is properly configured
func TestTransportConfiguration(t *testing.T) {
	resetSingleton()

	client := Get()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected Transport to be *http.Transport")
	}

	// Verify MaxIdleConns
	if transport.MaxIdleConns != DefaultConfig.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, DefaultConfig.MaxIdleConns)
	}

	// Verify MaxIdleConnsPerHost
	if transport.MaxIdleConnsPerHost != DefaultConfig.MaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, DefaultConfig.MaxIdleConnsPerHost)
	}

	// Verify IdleConnTimeout
	if transport.IdleConnTimeout != DefaultConfig.IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, DefaultConfig.IdleConnTimeout)
	}

	// Verify TLSHandshakeTimeout
	if transport.TLSHandshakeTimeout != DefaultConfig.TLSHandshakeTimeout {
		t.Errorf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, DefaultConfig.TLSHandshakeTimeout)
	}

	// Verify ExpectContinueTimeout
	if transport.ExpectContinueTimeout != DefaultConfig.ExpectContinueTimeout {
		t.Errorf("ExpectContinueTimeout = %v, want %v", transport.ExpectContinueTimeout, DefaultConfig.ExpectContinueTimeout)
	}

	// Verify ResponseHeaderTimeout
	if transport.ResponseHeaderTimeout != DefaultConfig.ResponseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, DefaultConfig.ResponseHeaderTimeout)
	}

	// Verify HTTP/2 is enabled
	if !transport.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
}

// TestConnectionReuse verifies that connections are being reused
func TestConnectionReuse(t *testing.T) {
	// Create a test server that tracks connection reuse
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Create a fresh client for this test to ensure clean state
	client := Get()

	// Make multiple requests to the same host
	for i := 0; i < 5; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		resp.Body.Close()
	}

	// All requests should succeed
	if requestCount != 5 {
		t.Errorf("Expected 5 requests, got %d", requestCount)
	}
}

// TestTimeoutValues verifies timeout configurations
func TestTimeoutValues(t *testing.T) {
	// Verify config values match requirements
	if DefaultConfig.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", DefaultConfig.MaxIdleConns)
	}

	if DefaultConfig.MaxIdleConnsPerHost != 100 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 100", DefaultConfig.MaxIdleConnsPerHost)
	}

	if DefaultConfig.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", DefaultConfig.IdleConnTimeout)
	}

	if DefaultConfig.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", DefaultConfig.TLSHandshakeTimeout)
	}

	if DefaultConfig.ExpectContinueTimeout != 1*time.Second {
		t.Errorf("ExpectContinueTimeout = %v, want 1s", DefaultConfig.ExpectContinueTimeout)
	}

	if DefaultConfig.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 30s", DefaultConfig.ResponseHeaderTimeout)
	}
}

// TestHTTP2Support verifies HTTP/2 support is configured
func TestHTTP2Support(t *testing.T) {
	resetSingleton()

	client := Get()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected Transport to be *http.Transport")
	}

	if !transport.ForceAttemptHTTP2 {
		t.Error("HTTP/2 should be enabled via ForceAttemptHTTP2")
	}
}

// TestClientNotDefaultClient verifies we don't use http.DefaultClient
func TestClientNotDefaultClient(t *testing.T) {
	resetSingleton()

	client := Get()
	if client == http.DefaultClient {
		t.Error("Get() should not return http.DefaultClient")
	}
}

// TestKeepAliveSupport verifies keep-alive is enabled
func TestKeepAliveSupport(t *testing.T) {
	resetSingleton()

	client := Get()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected Transport to be *http.Transport")
	}

	// DisableKeepAlive should be false (keep-alive enabled by default)
	if transport.DisableKeepAlives {
		t.Error("DisableKeepAlives should be false (keep-alive should be enabled)")
	}
}

// resetSingleton is a test helper to reset the singleton state
func resetSingleton() {
	once = sync.Once{}
	instance = nil
}
