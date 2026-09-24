package model

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
)

// ErrAllChannelsRateLimited 表示当前所有候选渠道都已被各自的请求频率上限挡住，
// 调用方可以据此向用户返回 429 而不是「无可用渠道」。
var ErrAllChannelsRateLimited = errors.New("all available channels have reached their request limit")

// channelRequestLimiter 是基于滑动窗口的请求计数器，按渠道维度统计。
// 之所以用滑动窗口而非固定窗口：上游（OpenAI/Anthropic/DeepSeek 等）基本都按
// 滑动窗口限流，固定窗口在窗口交界处会放行两倍请求量，依然会触发上游 429。
var channelRequestLimiter common.InMemoryRateLimiter

func channelRequestLimitKey(channelId int) string {
	return fmt.Sprintf("channel_request_limit:%d", channelId)
}

// allowChannelRequest 判断渠道当前是否还能接受一次请求。
// 注意：该函数带有副作用——判定通过时会同时完成「占用」，
// 这样并发请求不会同时通过检查而超发。
func allowChannelRequest(channel *Channel) bool {
	limit := channel.GetRequestLimit()
	if limit <= 0 {
		return true
	}
	// Init 内部做了幂等处理，可安全重复调用
	channelRequestLimiter.Init(config.RateLimitKeyExpirationDuration)
	return channelRequestLimiter.Request(channelRequestLimitKey(channel.Id), limit, channel.GetRequestLimitDuration())
}

// ReleaseChannelRequest 归还一次已占用但未真正使用的名额。
// 典型场景是重试时又随机选到刚刚失败的渠道，此时该渠道会被跳过，
// 若不归还就会白白占掉一个名额。
func ReleaseChannelRequest(channel *Channel) {
	if channel == nil {
		return
	}
	if channel.GetRequestLimit() <= 0 {
		return
	}
	channelRequestLimiter.Init(config.RateLimitKeyExpirationDuration)
	channelRequestLimiter.Release(channelRequestLimitKey(channel.Id))
}

// pickChannel 在 [start, end) 区间内以随机起点轮询，返回第一个未达上限的渠道。
// 区间内渠道都未配置限制时，等价于等概率随机，保持原有的负载均衡语义；
// 部分渠道达上限时自动跳过，即「B 方案：超限换下一个渠道」。
func pickChannel(channels []*Channel, start, end int) *Channel {
	if start >= end {
		return nil
	}
	n := end - start
	offset := rand.Intn(n)
	for i := 0; i < n; i++ {
		channel := channels[start+(offset+i)%n]
		if allowChannelRequest(channel) {
			return channel
		}
	}
	return nil
}

// selectSatisfiedChannel 是渠道选择的统一入口，内存缓存路径与数据库路径共用，
// 保证两种部署形态下行为一致。channels 必须已按优先级降序排列。
func selectSatisfiedChannel(channels []*Channel, ignoreFirstPriority bool) (*Channel, error) {
	if len(channels) == 0 {
		return nil, errors.New("channel not found")
	}
	// 计算最高优先级档的边界，与原实现保持一致：
	// 优先级为 0 的渠道不参与分档，整体视为同一档。
	endIdx := len(channels)
	if channels[0].GetPriority() > 0 {
		for i := range channels {
			if channels[i].GetPriority() != channels[0].GetPriority() {
				endIdx = i
				break
			}
		}
	}
	var channel *Channel
	if ignoreFirstPriority && endIdx < len(channels) {
		// 重试场景：优先使用低优先级档，全部打满时再回退到最高优先级档
		channel = pickChannel(channels, endIdx, len(channels))
		if channel == nil {
			channel = pickChannel(channels, 0, endIdx)
		}
	} else {
		// 常规场景：优先使用最高优先级档，全部打满时降级到低优先级档
		channel = pickChannel(channels, 0, endIdx)
		if channel == nil {
			channel = pickChannel(channels, endIdx, len(channels))
		}
	}
	if channel == nil {
		return nil, ErrAllChannelsRateLimited
	}
	return channel, nil
}
