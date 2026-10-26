package httpapi

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	limiter := newLoginLimiter(2, time.Minute)
	if !limiter.allow("one") || !limiter.allow("one") {
		t.Fatal("expected first two attempts to pass")
	}
	if limiter.allow("one") {
		t.Fatal("expected third attempt to be limited")
	}
	if !limiter.allow("two") {
		t.Fatal("expected other clients to be independent")
	}
}
