package httpclient

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	once     sync.Once
	instance *http.Client
)

func Get() *http.Client {
	once.Do(func() {
		instance = createClient()
	})
	return instance
}

func createClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          DefaultConfig.MaxIdleConns,
		MaxIdleConnsPerHost:   DefaultConfig.MaxIdleConnsPerHost,
		MaxConnsPerHost:       DefaultConfig.MaxConnsPerHost,
		IdleConnTimeout:       DefaultConfig.IdleConnTimeout,
		TLSHandshakeTimeout:   DefaultConfig.TLSHandshakeTimeout,
		ExpectContinueTimeout: DefaultConfig.ExpectContinueTimeout,
		ResponseHeaderTimeout: DefaultConfig.ResponseHeaderTimeout,
		ForceAttemptHTTP2:     true,
		DisableKeepAlives:     false,
		TLSClientConfig: &tls.Config{
			SessionTicketsDisabled: false,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}
}
