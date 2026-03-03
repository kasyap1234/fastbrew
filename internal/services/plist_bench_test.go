package services

import "testing"

func BenchmarkPlistParser_Parse(b *testing.B) {
	parser := NewPlistParser()
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>homebrew.mxcl.redis</string>
  <key>Program</key><string>/opt/homebrew/opt/redis/bin/redis-server</string>
  <key>RunAtLoad</key><true/>
  <key>StandardOutPath</key><string>/opt/homebrew/var/log/redis.log</string>
  <key>StandardErrorPath</key><string>/opt/homebrew/var/log/redis.log</string>
  <key>WorkingDirectory</key><string>/opt/homebrew/var</string>
</dict>
</plist>`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info, err := parser.Parse(data, "/tmp/homebrew.mxcl.redis.plist")
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
		if info.Label == "" {
			b.Fatal("parsed service label should not be empty")
		}
	}
}
