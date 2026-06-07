package imaging

import "testing"

func TestBlockedDialAddr(t *testing.T) {
	blocked := []string{
		"127.0.0.1:80",       // loopback
		"[::1]:443",          // loopback v6
		"169.254.169.254:80", // cloud metadata (link-local)
		"10.0.0.5:443",       // RFC1918
		"192.168.1.1:80",     // RFC1918
		"172.16.0.1:80",      // RFC1918
		"0.0.0.0:80",         // unspecified
		"[fe80::1]:80",       // link-local v6
		"not-an-ip:80",       // unresolved
	}
	for _, a := range blocked {
		if !blockedDialAddr(a) {
			t.Errorf("expected %s to be BLOCKED", a)
		}
	}
	allowed := []string{
		"8.8.8.8:443",
		"1.1.1.1:80",
		"[2606:4700:4700::1111]:443", // public v6
	}
	for _, a := range allowed {
		if blockedDialAddr(a) {
			t.Errorf("expected %s to be ALLOWED", a)
		}
	}
}
