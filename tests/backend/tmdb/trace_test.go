package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"joycurator/internal/trace"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTraceNeverIncludesCredentialsOrUpstreamBodies(t *testing.T) {
	for _, networkError := range []bool{false, true} {
		client, _ := New(nil, "private-auth-token", "", "zh-CN")
		client.RetryWait = time.Millisecond
		client.Trace = trace.New()
		client.HTTP = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			if networkError {
				proxy := &url.URL{Scheme: "https", Host: "private-proxy", User: url.UserPassword("proxy-user", "proxy-secret"), Path: "/"}
				return nil, errors.New(proxy.String() + " contains private-auth-token")
			}
			return response(401, `{"details":"private-response-body"}`), nil
		})}
		var dst any
		err := client.Get(context.Background(), "/search/movie", url.Values{"query": {"Film"}, "token": {"query-secret"}, "api_key": {"key-secret"}}, &dst)
		if err == nil {
			t.Fatal("expected failure")
		}
		raw, _ := json.Marshal(client.Trace.Events())
		for _, secret := range []string{"private-auth-token", "proxy-user", "proxy-secret", "private-proxy", "private-response-body", "query-secret", "key-secret", "Authorization"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("trace leaked", secret, string(raw))
			}
		}
		if !strings.Contains(string(raw), "Film") || !strings.Contains(string(raw), "statusCode") || !strings.Contains(string(raw), "durationMs") {
			t.Fatal(string(raw))
		}
	}
}
