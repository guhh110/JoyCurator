package tmdb

import (
	"context"
	"joycurator/internal/trace"
	"testing"
	"time"
)

func TestPoolSharesTransportAndLimiterWithoutSharingRequestContext(t *testing.T) {
	pool := &Pool{}
	defer pool.Close()
	first, err := pool.Client(nil, "first-secret", "", "zh-CN", trace.New())
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Client(nil, "second-secret", "", "en-US", trace.New())
	if err != nil {
		t.Fatal(err)
	}
	if first.HTTP != second.HTTP || first.limiter != second.limiter {
		t.Fatal("transport or throttle duplicated")
	}
	if first.Token == second.Token || first.Language == second.Language || first.Trace == second.Trace {
		t.Fatal("request snapshots shared")
	}
	first.limiter.next = time.Now().Add(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := second.throttle(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	old := first.HTTP
	next, err := pool.Client(nil, "third-secret", "http://localhost:9876", "en-US", nil)
	if err != nil || next.HTTP == old || next.limiter != first.limiter {
		t.Fatal("proxy rotation lost shared limiter", err)
	}
	if first.HTTP != old || first.Token != "first-secret" {
		t.Fatal("existing snapshot changed")
	}
	if _, err := pool.Client(nil, "x", "file:///tmp/proxy", "", nil); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}
