package config

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common/env"

	"github.com/google/uuid"
)

var SystemName = "One API"
var ServerAddress = "http://localhost:3000"
var Footer = ""
var Logo = ""
var TopUpLink = ""
var ChatLink = ""
var QuotaPerUnit = 500 * 1000.0 // $0.002 / 1K tokens
var DisplayInCurrencyEnabled = true
var DisplayTokenStatEnabled = true

// Any options with "Secret", "Token" in its key won't be return by GetOptions

var SessionSecret = uuid.New().String()

// SessionSecure marks the session cookie with the Secure flag so it is only
// sent over HTTPS. Enable it (SESSION_SECURE=true) when one-api is served via
// HTTPS; leave false for plain-HTTP deployments.
var SessionSecure = strings.ToLower(os.Getenv("SESSION_SECURE")) == "true"

var OptionMap map[string]string
var OptionMapRWMutex sync.RWMutex

var ItemsPerPage = 10
var MaxRecentItems = 100

var PasswordLoginEnabled = true
var PasswordRegisterEnabled = true
var EmailVerificationEnabled = false
var GitHubOAuthEnabled = false
var OidcEnabled = false
var WeChatAuthEnabled = false
var TurnstileCheckEnabled = false
var RegisterEnabled = true

var EmailDomainRestrictionEnabled = false
var EmailDomainWhitelist = []string{
	"gmail.com",
	"163.com",
	"126.com",
	"qq.com",
	"outlook.com",
	"hotmail.com",
	"icloud.com",
	"yahoo.com",
	"foxmail.com",
}

var DebugEnabled = strings.ToLower(os.Getenv("DEBUG")) == "true"
var DebugSQLEnabled = strings.ToLower(os.Getenv("DEBUG_SQL")) == "true"
var MemoryCacheEnabled = strings.ToLower(os.Getenv("MEMORY_CACHE_ENABLED")) == "true"

var LogConsumeEnabled = true

var SMTPServer = ""
var SMTPPort = 587
var SMTPAccount = ""
var SMTPFrom = ""
var SMTPToken = ""

var GitHubClientId = ""
var GitHubClientSecret = ""

var LarkClientId = ""
var LarkClientSecret = ""

var OidcClientId = ""
var OidcClientSecret = ""
var OidcWellKnown = ""
var OidcAuthorizationEndpoint = ""
var OidcTokenEndpoint = ""
var OidcUserinfoEndpoint = ""

var WeChatServerAddress = ""
var WeChatServerToken = ""
var WeChatAccountQRCodeImageURL = ""

var MessagePusherAddress = ""
var MessagePusherToken = ""

var TurnstileSiteKey = ""
var TurnstileSecretKey = ""

var QuotaForNewUser int64 = 0
var QuotaForInviter int64 = 0
var QuotaForInvitee int64 = 0
var ChannelDisableThreshold = 5.0
var AutomaticDisableChannelEnabled = false
var AutomaticEnableChannelEnabled = false
var QuotaRemindThreshold int64 = 1000
var PreConsumedQuota int64 = 500
var ApproximateTokenEnabled = false
var RetryTimes = 0

// DefaultAutomaticDisableKeywords 是渠道自动禁用关键字的默认值。
// 当上游返回的错误信息（转小写后）命中其中任意一项时，渠道会被立即禁用，
// 而不再等待 ChannelDisableThreshold 累计的失败次数。
// 参考 New API 的 AutomaticDisableKeywords，并补充了国内服务商常见的余额相关表述。
var DefaultAutomaticDisableKeywords = []string{
	"your credit balance is too low",
	"you exceeded your current quota",
	"this organization has been disabled",
	"organization has been restricted",
	"permission denied",
	"the security token included in the request is invalid",
	"operation not allowed",
	"your account is not authorized",
	"your access was terminated",
	"violation of our policies",
	"insufficient_quota",
	"insufficient quota",
	"insufficient balance",
	"insufficient credits",
	"billing_hard_limit_reached",
	"api key not valid",
	"api key expired",
	"余额不足",
	"额度不足",
	"账户已欠费",
	"已欠费",
}

var AutomaticDisableKeywords = DefaultAutomaticDisableKeywords
var automaticDisableKeywordsRWMutex sync.RWMutex

// GetAutomaticDisableKeywords 返回关键字列表的快照。
// 后台可随时修改该列表，因此需要加锁以避免请求线程读取时发生数据竞争。
func GetAutomaticDisableKeywords() []string {
	automaticDisableKeywordsRWMutex.RLock()
	defer automaticDisableKeywordsRWMutex.RUnlock()
	return AutomaticDisableKeywords
}

// AutomaticDisableKeywordsToString 将关键字列表序列化为换行分隔的字符串，用于持久化与后台展示。
func AutomaticDisableKeywordsToString() string {
	automaticDisableKeywordsRWMutex.RLock()
	defer automaticDisableKeywordsRWMutex.RUnlock()
	return strings.Join(AutomaticDisableKeywords, "\n")
}

// AutomaticDisableKeywordsFromString 解析换行分隔的关键字，忽略空白行并统一转为小写。
// 解析结果为空时保留原有配置，避免后台误清空后彻底失去保护。
func AutomaticDisableKeywordsFromString(s string) {
	keywords := make([]string, 0)
	for _, keyword := range strings.Split(s, "\n") {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	if len(keywords) == 0 {
		return
	}
	automaticDisableKeywordsRWMutex.Lock()
	defer automaticDisableKeywordsRWMutex.Unlock()
	AutomaticDisableKeywords = keywords
}

var RootUserEmail = ""

var IsMasterNode = os.Getenv("NODE_TYPE") != "slave"

var requestInterval, _ = strconv.Atoi(os.Getenv("POLLING_INTERVAL"))
var RequestInterval = time.Duration(requestInterval) * time.Second

var SyncFrequency = env.Int("SYNC_FREQUENCY", 10*60) // unit is second

var BatchUpdateEnabled = false
var BatchUpdateInterval = env.Int("BATCH_UPDATE_INTERVAL", 5)

var RelayTimeout = env.Int("RELAY_TIMEOUT", 0) // unit is second

var GeminiSafetySetting = env.String("GEMINI_SAFETY_SETTING", "BLOCK_NONE")

var Theme = env.String("THEME", "default")
var ValidThemes = map[string]bool{
	"default": true,
	"berry":   true,
	"air":     true,
}

// All duration's unit is seconds
// Shouldn't larger then RateLimitKeyExpirationDuration
var (
	GlobalApiRateLimitNum            = env.Int("GLOBAL_API_RATE_LIMIT", 480)
	GlobalApiRateLimitDuration int64 = 3 * 60

	GlobalWebRateLimitNum            = env.Int("GLOBAL_WEB_RATE_LIMIT", 240)
	GlobalWebRateLimitDuration int64 = 3 * 60

	UploadRateLimitNum            = 10
	UploadRateLimitDuration int64 = 60

	DownloadRateLimitNum            = 10
	DownloadRateLimitDuration int64 = 60

	CriticalRateLimitNum            = 20
	CriticalRateLimitDuration int64 = 20 * 60
)

var RateLimitKeyExpirationDuration = 20 * time.Minute

var EnableMetric = env.Bool("ENABLE_METRIC", false)
var MetricQueueSize = env.Int("METRIC_QUEUE_SIZE", 10)
var MetricSuccessRateThreshold = env.Float64("METRIC_SUCCESS_RATE_THRESHOLD", 0.8)
var MetricSuccessChanSize = env.Int("METRIC_SUCCESS_CHAN_SIZE", 1024)
var MetricFailChanSize = env.Int("METRIC_FAIL_CHAN_SIZE", 128)

var InitialRootToken = os.Getenv("INITIAL_ROOT_TOKEN")

var InitialRootAccessToken = os.Getenv("INITIAL_ROOT_ACCESS_TOKEN")

var GeminiVersion = env.String("GEMINI_VERSION", "v1")

var OnlyOneLogFile = env.Bool("ONLY_ONE_LOG_FILE", false)

var RelayProxy = env.String("RELAY_PROXY", "")
var UserContentRequestProxy = env.String("USER_CONTENT_REQUEST_PROXY", "")
var UserContentRequestTimeout = env.Int("USER_CONTENT_REQUEST_TIMEOUT", 30)

var EnforceIncludeUsage = env.Bool("ENFORCE_INCLUDE_USAGE", false)
var TestPrompt = env.String("TEST_PROMPT", "Output only your specific model name with no additional text.")
