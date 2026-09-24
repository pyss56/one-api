package model

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testIntPtr(i int) *int {
	return &i
}

func testInt64Ptr(i int64) *int64 {
	return &i
}

// 默认值：未配置时视为不限制，窗口长度回退为 60 秒
func TestGetRequestLimitDefaults(t *testing.T) {
	channel := &Channel{}
	if channel.GetRequestLimit() != 0 {
		t.Fatalf("expected unlimited(0), got %d", channel.GetRequestLimit())
	}
	if channel.GetRequestLimitDuration() != 60 {
		t.Fatalf("expected default 60s, got %d", channel.GetRequestLimitDuration())
	}
	channel.RequestLimitDuration = testIntPtr(0)
	if channel.GetRequestLimitDuration() != 60 {
		t.Fatalf("expected fallback 60s for non-positive duration, got %d", channel.GetRequestLimitDuration())
	}
	channel.RequestLimit = testIntPtr(100)
	channel.RequestLimitDuration = testIntPtr(3600)
	if channel.GetRequestLimit() != 100 || channel.GetRequestLimitDuration() != 3600 {
		t.Fatalf("unexpected limit config: %d / %d", channel.GetRequestLimit(), channel.GetRequestLimitDuration())
	}
}

// 未配置上限的渠道始终可用，且会被随机分发
func TestSelectSatisfiedChannel_NoLimit(t *testing.T) {
	channels := []*Channel{
		{Id: 1, RequestLimit: testIntPtr(0)},
		{Id: 2, RequestLimit: testIntPtr(0)},
	}
	used := make(map[int]bool)
	for i := 0; i < 50; i++ {
		channel, err := selectSatisfiedChannel(channels, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if channel == nil {
			t.Fatal("expected a channel")
		}
		used[channel.Id] = true
	}
	if len(used) != 2 {
		t.Fatalf("expected both unlimited channels to be used, got %v", used)
	}
}

// 单个渠道达到上限后应返回 ErrAllChannelsRateLimited
func TestSelectSatisfiedChannel_AllLimited(t *testing.T) {
	channels := []*Channel{
		{Id: 201, RequestLimit: testIntPtr(2), RequestLimitDuration: testIntPtr(60)},
	}
	for i := 0; i < 2; i++ {
		if _, err := selectSatisfiedChannel(channels, false); err != nil {
			t.Fatalf("call %d should succeed: %v", i, err)
		}
	}
	if _, err := selectSatisfiedChannel(channels, false); err != ErrAllChannelsRateLimited {
		t.Fatalf("expected ErrAllChannelsRateLimited, got %v", err)
	}
}

// 选渠道必须立即占用名额，否则并发下会超发
func TestSelectSatisfiedChannel_ReservesQuota(t *testing.T) {
	channels := []*Channel{
		{Id: 202, RequestLimit: testIntPtr(3), RequestLimitDuration: testIntPtr(60)},
	}
	for i := 0; i < 3; i++ {
		if _, err := selectSatisfiedChannel(channels, false); err != nil {
			t.Fatalf("selection %d should succeed: %v", i, err)
		}
	}
	if _, err := selectSatisfiedChannel(channels, false); err != ErrAllChannelsRateLimited {
		t.Fatalf("selection should reserve quota, expected ErrAllChannelsRateLimited, got %v", err)
	}
}

// 超限的渠道应被跳过，请求落到未受限的渠道上（B 方案）
func TestSelectSatisfiedChannel_SkipLimitedChannel(t *testing.T) {
	channels := []*Channel{
		{Id: 301, RequestLimit: testIntPtr(1), RequestLimitDuration: testIntPtr(60)},
		{Id: 302, RequestLimit: testIntPtr(0)},
	}
	if !allowChannelRequest(channels[0]) {
		t.Fatal("first request to channel 301 should be allowed")
	}
	if allowChannelRequest(channels[0]) {
		t.Fatal("second request to channel 301 should be rejected")
	}
	channel, err := selectSatisfiedChannel(channels, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel.Id != 302 {
		t.Fatalf("expected fallback to channel 302, got %d", channel.Id)
	}
}

// 优先级分档语义保持不变：优先高档，ignoreFirstPriority 时优先低档
func TestSelectSatisfiedChannel_PriorityTier(t *testing.T) {
	channels := []*Channel{
		{Id: 401, Priority: testInt64Ptr(5), RequestLimit: testIntPtr(0)},
		{Id: 402, Priority: testInt64Ptr(1), RequestLimit: testIntPtr(0)},
	}
	for i := 0; i < 20; i++ {
		channel, err := selectSatisfiedChannel(channels, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if channel.Id != 401 {
			t.Fatalf("expected highest priority channel 401, got %d", channel.Id)
		}
	}
	channel, err := selectSatisfiedChannel(channels, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel.Id != 402 {
		t.Fatalf("expected lower tier channel 402 with ignoreFirstPriority, got %d", channel.Id)
	}
}

// 高优先级档全部打满时应降级到低优先级档
func TestSelectSatisfiedChannel_DegradeToLowerTier(t *testing.T) {
	channels := []*Channel{
		{Id: 501, Priority: testInt64Ptr(5), RequestLimit: testIntPtr(1), RequestLimitDuration: testIntPtr(60)},
		{Id: 502, Priority: testInt64Ptr(1), RequestLimit: testIntPtr(0)},
	}
	if _, err := selectSatisfiedChannel(channels, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	channel, err := selectSatisfiedChannel(channels, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel.Id != 502 {
		t.Fatalf("expected degrade to channel 502, got %d", channel.Id)
	}
}

// 滑动窗口：窗口过后计数应当失效，渠道重新可用
func TestChannelRequestLimit_SlidingWindowExpiry(t *testing.T) {
	channel := &Channel{Id: 601, RequestLimit: testIntPtr(1), RequestLimitDuration: testIntPtr(1)}
	if !allowChannelRequest(channel) {
		t.Fatal("first request should be allowed")
	}
	if allowChannelRequest(channel) {
		t.Fatal("second request within the window should be rejected")
	}
	time.Sleep(1100 * time.Millisecond)
	if !allowChannelRequest(channel) {
		t.Fatal("request should be allowed again after the window elapsed")
	}
}

// 归还名额：选中但未使用的渠道不应永久占用名额
func TestChannelRequestLimit_Release(t *testing.T) {
	channel := &Channel{Id: 801, RequestLimit: testIntPtr(1), RequestLimitDuration: testIntPtr(60)}
	if !allowChannelRequest(channel) {
		t.Fatal("first request should be allowed")
	}
	if allowChannelRequest(channel) {
		t.Fatal("second request should be rejected while occupied")
	}
	ReleaseChannelRequest(channel)
	if !allowChannelRequest(channel) {
		t.Fatal("should be usable again after releasing the reserved quota")
	}
}

// 并发下必须严格不超发：占用与判定在同一把锁内完成
func TestAllowChannelRequest_ConcurrentNoOvershoot(t *testing.T) {
	const limit = 50
	const concurrency = 500
	channel := &Channel{Id: 901, RequestLimit: testIntPtr(limit), RequestLimitDuration: testIntPtr(60)}

	var wg sync.WaitGroup
	var allowed int64
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if allowChannelRequest(channel) {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	wg.Wait()

	if allowed != int64(limit) {
		t.Fatalf("expected exactly %d allowed out of %d concurrent requests, got %d", limit, concurrency, allowed)
	}
}
