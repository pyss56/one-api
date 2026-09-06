package monitor

import (
	"net/http"
	"testing"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/stretchr/testify/assert"
)

func TestShouldDisableChannel(t *testing.T) {
	originalEnabled := config.AutomaticDisableChannelEnabled
	originalKeywords := config.GetAutomaticDisableKeywords()
	defer func() {
		config.AutomaticDisableChannelEnabled = originalEnabled
		config.AutomaticDisableKeywordsFromString(joinKeywords(originalKeywords))
	}()

	config.AutomaticDisableChannelEnabled = true
	config.AutomaticDisableKeywordsFromString(config.AutomaticDisableKeywordsToString())

	tests := []struct {
		name       string
		err        *model.Error
		statusCode int
		expected   bool
	}{
		{"nil error", nil, http.StatusOK, false},
		{"unauthorized", &model.Error{Message: "some error"}, http.StatusUnauthorized, true},
		{"insufficient quota type", &model.Error{Type: "insufficient_quota", Message: "no money"}, http.StatusTooManyRequests, true},
		{"billing hard limit code", &model.Error{Code: "billing_hard_limit_reached", Message: "no money"}, http.StatusForbidden, true},
		{"invalid api key code", &model.Error{Code: "invalid_api_key", Message: "bad key"}, http.StatusBadRequest, true},
		{"balance keyword", &model.Error{Message: "Your credit balance is too low"}, http.StatusBadRequest, true},
		{"balance keyword case insensitive", &model.Error{Message: "YOUR CREDIT BALANCE IS TOO LOW"}, http.StatusBadRequest, true},
		{"quota keyword", &model.Error{Message: "You exceeded your current quota"}, http.StatusBadRequest, true},
		{"chinese balance keyword", &model.Error{Message: "账户已欠费，请充值"}, http.StatusBadRequest, true},
		{"irrelevant message", &model.Error{Message: "the model does not exist"}, http.StatusBadRequest, false},
		{"rate limit", &model.Error{Type: "requests", Message: "Rate limit reached"}, http.StatusTooManyRequests, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ShouldDisableChannel(tt.err, tt.statusCode))
		})
	}

	t.Run("disabled globally", func(t *testing.T) {
		config.AutomaticDisableChannelEnabled = false
		assert.False(t, ShouldDisableChannel(&model.Error{Message: "Your credit balance is too low"}, http.StatusBadRequest))
	})

	t.Run("custom keywords", func(t *testing.T) {
		config.AutomaticDisableChannelEnabled = true
		config.AutomaticDisableKeywordsFromString("my provider is down")
		defer config.AutomaticDisableKeywordsFromString(config.AutomaticDisableKeywordsToString())
		assert.True(t, ShouldDisableChannel(&model.Error{Message: "My Provider Is Down"}, http.StatusBadRequest))
		assert.False(t, ShouldDisableChannel(&model.Error{Message: "Your credit balance is too low"}, http.StatusBadRequest))
	})

	t.Run("empty keywords keeps previous config", func(t *testing.T) {
		before := config.GetAutomaticDisableKeywords()
		config.AutomaticDisableKeywordsFromString(" \n \n")
		assert.Equal(t, before, config.GetAutomaticDisableKeywords())
	})
}

// joinKeywords 用于测试结束时恢复原始关键字配置。
func joinKeywords(keywords []string) string {
	result := ""
	for i, keyword := range keywords {
		if i > 0 {
			result += "\n"
		}
		result += keyword
	}
	return result
}
