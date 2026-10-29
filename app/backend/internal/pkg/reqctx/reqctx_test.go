package reqctx

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:8090/api", nil)
	r.Header.Set("X-Forwarded-Host", "192.168.1.10:51880")
	r.Header.Set("X-Forwarded-Proto", "http")
	if o := FromRequest(r); o.Host != "192.168.1.10" || o.Port != 51880 || o.Scheme != "http" {
		t.Fatalf("%+v", o)
	}
	r = httptest.NewRequest("GET", "http://x/", nil)
	r.Host = "live.example.com"
	r.Header.Set("X-Forwarded-Proto", "https")
	if o := FromRequest(r); o.Host != "live.example.com" || o.Port != 443 {
		t.Fatalf("%+v", o)
	}
}
