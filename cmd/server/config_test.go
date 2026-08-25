package main

import "testing"

func TestParseConfigAddressPrecedence(t *testing.T) {
	cfg, err := parseConfig(nil, func(key string) string {
		if key == "PORT" {
			return "19123"
		}
		return ""
	})
	if err != nil || cfg.Addr != "127.0.0.1:19123" {
		t.Fatalf("PORT was not applied: %+v %v", cfg, err)
	}
	cfg, err = parseConfig([]string{"-addr=127.0.0.1:19234"}, func(string) string { return "bad" })
	if err != nil || cfg.Addr != "127.0.0.1:19234" {
		t.Fatalf("explicit addr did not win: %+v %v", cfg, err)
	}
}

func TestParseConfigRejectsNonLoopback(t *testing.T) {
	if _, err := parseConfig([]string{"-addr=0.0.0.0:19091"}, func(string) string { return "" }); err == nil {
		t.Fatal("non-loopback address was accepted")
	}
}
