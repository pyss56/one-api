package monitor

import (
	"net/http"
	"strings"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/model"
)

// ShouldDisableChannel 判断一次转发的错误是否应当立即禁用渠道。
//
// 这里主要解决「余额/额度」类问题：上游余额耗尽、密钥失效、组织被封禁等错误
// 在短期内不会自愈，继续重试只会浪费配额并拖慢请求，因此命中后直接禁用，
// 而不再等待 ChannelDisableThreshold 所累计的失败次数。
//
// 判定顺序参考 New API：状态码 -> 错误类型 -> 错误码 -> 可配置关键字。
func ShouldDisableChannel(err *model.Error, statusCode int) bool {
	if !config.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	// 鉴权失败通常是密钥失效，不会自愈
	if statusCode == http.StatusUnauthorized {
		return true
	}
	switch err.Type {
	case "insufficient_quota", "authentication_error", "permission_error", "forbidden":
		return true
	}
	// billing_hard_limit_reached 等错误码同样是余额/额度耗尽的明确信号
	switch err.Code {
	case "invalid_api_key", "account_deactivated", "billing_hard_limit_reached":
		return true
	}
	lowerMessage := strings.ToLower(err.Message)
	for _, keyword := range config.GetAutomaticDisableKeywords() {
		if keyword == "" {
			continue
		}
		if strings.Contains(lowerMessage, keyword) {
			return true
		}
	}
	return false
}

func ShouldEnableChannel(err error, openAIErr *model.Error) bool {
	if !config.AutomaticEnableChannelEnabled {
		return false
	}
	if err != nil {
		return false
	}
	if openAIErr != nil {
		return false
	}
	return true
}
