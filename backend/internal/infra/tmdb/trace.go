package tmdb

import (
	"net/url"
	"time"
)

// Deliberately whitelist query parameters. Never record headers, the full URL,
// proxy configuration, transport errors or untrusted upstream response bodies.
func (c *Client) traceRequest(path string, q url.Values, started time.Time, cacheHit bool, attempts int, err error) {
	if c.Trace == nil {
		return
	}
	query := map[string]string{}
	for _, key := range []string{"query", "year", "first_air_date_year", "language", "include_adult", "append_to_response"} {
		if value := q.Get(key); value != "" {
			query[key] = value
		}
	}
	status, message := "info", "TMDB 请求完成"
	if cacheHit {
		message = "命中 TMDB 数据库缓存，未发起网络请求"
	}
	if err != nil {
		status, message = "failed", err.Error()
	}
	c.Trace.Add("tmdb", "request", status, message, map[string]any{"path": path, "query": query, "cacheHit": cacheHit, "attempts": attempts, "durationMs": float64(time.Since(started).Microseconds()) / 1000})
}
func (c *Client) traceAttempt(path string, attempt, statusCode int, started time.Time) {
	if c.Trace == nil {
		return
	}
	status, message := "info", "TMDB 网络请求返回"
	if statusCode == 0 {
		status, message = "failed", "TMDB 网络请求失败"
	} else if statusCode != 200 {
		status = "failed"
	}
	c.Trace.Add("tmdb", "http-attempt", status, message, map[string]any{"path": path, "attempt": attempt, "statusCode": statusCode, "durationMs": float64(time.Since(started).Microseconds()) / 1000})
}
