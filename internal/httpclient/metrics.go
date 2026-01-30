package httpclient

import (
	"net/http"
	"sync/atomic"
)

// Metrics tracks HTTP client performance metrics
type Metrics struct {
	RequestsTotal       int64
	RequestsHTTP2       int64
	RequestsHTTP1       int64
	ConnectionsActive   int64
	ConnectionsIdle     int64
	ConnectionWaitCount int64
}

var globalMetrics Metrics

// RecordRequest increments request counters
func RecordRequest(isHTTP2 bool) {
	atomic.AddInt64(&globalMetrics.RequestsTotal, 1)
	if isHTTP2 {
		atomic.AddInt64(&globalMetrics.RequestsHTTP2, 1)
	} else {
		atomic.AddInt64(&globalMetrics.RequestsHTTP1, 1)
	}
}

// GetMetrics returns current metrics
func GetMetrics() Metrics {
	return Metrics{
		RequestsTotal:       atomic.LoadInt64(&globalMetrics.RequestsTotal),
		RequestsHTTP2:       atomic.LoadInt64(&globalMetrics.RequestsHTTP2),
		RequestsHTTP1:       atomic.LoadInt64(&globalMetrics.RequestsHTTP1),
		ConnectionsActive:   atomic.LoadInt64(&globalMetrics.ConnectionsActive),
		ConnectionsIdle:     atomic.LoadInt64(&globalMetrics.ConnectionsIdle),
		ConnectionWaitCount: atomic.LoadInt64(&globalMetrics.ConnectionWaitCount),
	}
}

// TransportWrapper wraps an http.RoundTripper to record metrics
type TransportWrapper struct {
	Base http.RoundTripper
}

func (t *TransportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Base.RoundTrip(req)
	if resp != nil {
		isHTTP2 := resp.ProtoMajor == 2
		RecordRequest(isHTTP2)
	}
	return resp, err
}
