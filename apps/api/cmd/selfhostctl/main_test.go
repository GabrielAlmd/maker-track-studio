package main

import "testing"

func TestOriginValidation(t *testing.T) {
	for _, origin := range []string{"https://maker.local:8443", "https://192.168.1.2", "https://[::1]:8443"} {
		if _, err := parseOrigin(origin); err != nil {
			t.Errorf("%s: %v", origin, err)
		}
	}
	for _, origin := range []string{"http://maker.local", "https://", "https://:8443", "https://user@maker.local", "https://maker.local/", "https://maker.local?", "https://maker.local?q=x", "https://maker.local#x", "https://maker.local:0", "https://maker.local:65536", "https://bad;host"} {
		if _, err := parseOrigin(origin); err == nil {
			t.Errorf("accepted %s", origin)
		}
	}
}
