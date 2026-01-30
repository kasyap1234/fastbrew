package httpclient

import "time"

type ClientConfig struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ResponseHeaderTimeout time.Duration
	HTTP2ReadIdleTimeout  time.Duration
	HTTP2PingTimeout      time.Duration
}

var DefaultConfig = ClientConfig{
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   100,
	MaxConnsPerHost:       100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	HTTP2ReadIdleTimeout:  10 * time.Second,
	HTTP2PingTimeout:      5 * time.Second,
}
