package tmdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestSearchCacheAndToken(t *testing.T) {
	cache := &testCache{}
	client, err := New(cache, "private-token", "", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Fatal("missing token header")
		}
		if r.URL.Query().Get("language") != "zh-CN" || r.URL.Query().Get("year") != "2020" {
			t.Fatal(r.URL.String())
		}
		return response(200, `{"results":[{"id":1,"title":"中文标题","release_date":"2020-01-01"}]}`), nil
	})}
	for i := 0; i < 2; i++ {
		items, err := client.Search(context.Background(), "movie", "标题", 2020)
		if err != nil || len(items) != 1 || items[0].Title != "中文标题" {
			t.Fatalf("%#v %v", items, err)
		}
	}
	if calls != 1 {
		t.Fatal("cache not used", calls)
	}
}
func TestRateLimitAndEmptyResult(t *testing.T) {
	client, _ := New(nil, "token", "", "en-US")
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(429, `{}`), nil
		}
		return response(200, `{"results":[]}`), nil
	})}
	items, err := client.Search(context.Background(), "tv", "Unknown", 0)
	if err != nil || len(items) != 0 || calls != 2 {
		t.Fatalf("%v %v %d", items, err, calls)
	}
}
func TestErrorRedactionAndCancellation(t *testing.T) {
	client, _ := New(nil, "very-secret-token", "", "en-US")
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return response(401, `{"error":"very-secret-token"}`), nil
	})}
	_, err := client.Search(context.Background(), "movie", "title", 0)
	if err == nil || strings.Contains(err.Error(), "very-secret-token") {
		t.Fatal("credentials leaked", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Search(ctx, "movie", "title", 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "dial tcp: i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestTimeoutRetriesThenFails covers a slow upstream: three attempts, then a
// stable error that never exposes the transport details.
func TestTimeoutRetriesThenFails(t *testing.T) {
	client, _ := New(nil, "token", "", "en-US")
	client.RetryWait = time.Millisecond
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, timeoutError{}
	})}
	_, err := client.Search(context.Background(), "movie", "title", 0)
	if err == nil || !strings.Contains(err.Error(), "网络连接失败") {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("attempts %d", calls)
	}
}

type testCache struct{ data []byte }

func (c *testCache) LoadCache(context.Context, string, int64) ([]byte, error) { return c.data, nil }
func (c *testCache) SaveCache(_ context.Context, _ string, b []byte, _ int64) error {
	c.data = append([]byte(nil), b...)
	return nil
}
