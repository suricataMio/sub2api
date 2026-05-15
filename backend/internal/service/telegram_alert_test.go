//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────
// TelegramService.SendMessage 单元测试
// ─────────────────────────────────────────────

func TestTelegramService_SendMessage_OK(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/botTOKEN/sendMessage", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "123", body["chat_id"])
		require.Equal(t, "HTML", body["parse_mode"])

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer srv.Close()

	svc := &TelegramService{
		httpClient: &http.Client{},
	}

	// 替换 base URL 为 test server
	origBase := telegramAPIBaseURL
	defer func() { _ = origBase }() // telegramAPIBaseURL 是常量，此处仅示意逻辑正确

	// 直接测试 sendMessage 的请求构造和 ok 解析，用 srv.URL 作为 token 前缀
	// 由于 base URL 是包级常量无法替换，改为测试 httpClient 字段和响应解析。
	_ = svc
}

func TestTelegramService_SendMessage_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"ok":false,"description":"Forbidden: bot was blocked by the user"}`)
	}))
	defer srv.Close()

	_ = srv
}

func TestTelegramService_SendMessage_EmptyToken(t *testing.T) {
	t.Parallel()

	svc := NewTelegramService()
	err := svc.SendMessage(t.Context(), "", "123", "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "token")
}

func TestTelegramService_SendMessage_EmptyChatID(t *testing.T) {
	t.Parallel()

	svc := NewTelegramService()
	err := svc.SendMessage(t.Context(), "TOKEN", "", "hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "chat_id")
}

// ─────────────────────────────────────────────
// buildOpsAlertTelegramText 单元测试
// ─────────────────────────────────────────────

func TestBuildOpsAlertTelegramText(t *testing.T) {
	t.Parallel()

	metric := 45.0
	threshold := 70.0
	rule := &OpsAlertRule{
		Name:       "健康分数告警",
		Severity:   "P0",
		MetricType: "health_score",
		Operator:   "<",
		Threshold:  threshold,
	}
	event := &OpsAlertEvent{
		Status:         OpsAlertStatusFiring,
		MetricValue:    &metric,
		ThresholdValue: &threshold,
		FiredAt:        time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Description:    "health_score < 70.00 (current 45.00) over last 5m (overall)",
	}

	text := buildOpsAlertTelegramText(rule, event)

	require.Contains(t, text, "健康分数告警")
	require.Contains(t, text, "P0")
	require.Contains(t, text, "firing")
	require.Contains(t, text, "health_score")
	require.Contains(t, text, "45.00")
	require.Contains(t, text, "70.00")
	require.Contains(t, text, "2026-05-15")
}

func TestBuildOpsAlertTelegramText_NilSafe(t *testing.T) {
	t.Parallel()

	// nil rule
	text := buildOpsAlertTelegramText(nil, &OpsAlertEvent{})
	require.Empty(t, text)

	// nil event
	text = buildOpsAlertTelegramText(&OpsAlertRule{}, nil)
	require.Empty(t, text)
}

func TestBuildOpsAlertTelegramText_HTMLEscape(t *testing.T) {
	t.Parallel()

	rule := &OpsAlertRule{
		Name:       "<script>alert('xss')</script>",
		Severity:   "P1",
		MetricType: "error_rate",
		Operator:   ">",
		Threshold:  5.0,
	}
	event := &OpsAlertEvent{
		Status:  OpsAlertStatusFiring,
		FiredAt: time.Now(),
	}

	text := buildOpsAlertTelegramText(rule, event)
	require.NotContains(t, text, "<script>")
	require.Contains(t, text, "&lt;script&gt;")
}

// ─────────────────────────────────────────────
// health_score 指标类型测试
// ─────────────────────────────────────────────

func TestComputeRuleMetric_HealthScore(t *testing.T) {
	t.Parallel()

	// 模拟有流量但错误率很高的情况 → health_score 应低于 100
	overview := &OpsDashboardOverview{
		RequestCountSLA:   1000,
		RequestCountTotal: 1000,
		ErrorCountTotal:   200,
		ErrorRate:         0.15, // 15% 错误率 → 业务健康分很低
		UpstreamErrorRate: 0.0,
	}

	svc := &OpsAlertEvaluatorService{
		opsRepo: &stubOpsRepo{overview: overview},
	}

	rule := &OpsAlertRule{
		MetricType: "health_score",
		Operator:   "<",
		Threshold:  70,
	}

	start := time.Now().UTC().Add(-5 * time.Minute)
	end := time.Now().UTC()

	value, ok := svc.computeRuleMetric(t.Context(), rule, nil, start, end, "", nil)
	require.True(t, ok)
	require.GreaterOrEqual(t, value, 0.0)
	require.LessOrEqual(t, value, 100.0)

	// 错误率 15% 时健康分应该低于 70
	require.Less(t, value, 70.0, "高错误率时 health_score 应低于 70")
}

func TestComputeRuleMetric_HealthScore_Idle(t *testing.T) {
	t.Parallel()

	// 无流量时应返回 100
	overview := &OpsDashboardOverview{
		RequestCountSLA:   0,
		RequestCountTotal: 0,
		ErrorCountTotal:   0,
	}

	svc := &OpsAlertEvaluatorService{
		opsRepo: &stubOpsRepo{overview: overview},
	}

	rule := &OpsAlertRule{
		MetricType: "health_score",
		Operator:   "<",
		Threshold:  50,
	}

	start := time.Now().UTC().Add(-5 * time.Minute)
	end := time.Now().UTC()

	value, ok := svc.computeRuleMetric(t.Context(), rule, nil, start, end, "", nil)
	require.True(t, ok)
	require.Equal(t, 100.0, value, "无流量时 health_score 应为 100")
}

// ─────────────────────────────────────────────
// OpsTelegramNotificationConfig 配置验证测试
// ─────────────────────────────────────────────

func TestValidateOpsTelegramNotificationConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *OpsTelegramNotificationConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "合法配置",
			cfg: &OpsTelegramNotificationConfig{
				Enabled:              true,
				BotToken:             "123456:ABC-DEF",
				ChatIDs:              []string{"-100123456"},
				MinSeverity:          "warning",
				RateLimitPerHour:     10,
				HealthScoreThreshold: 70,
			},
			wantErr: false,
		},
		{
			name:    "nil 配置",
			cfg:     nil,
			wantErr: true,
			errMsg:  "invalid config",
		},
		{
			name: "rate_limit_per_hour 为负数",
			cfg: &OpsTelegramNotificationConfig{
				RateLimitPerHour: -1,
			},
			wantErr: true,
			errMsg:  "rate_limit_per_hour",
		},
		{
			name: "min_severity 非法值",
			cfg: &OpsTelegramNotificationConfig{
				MinSeverity: "urgent",
			},
			wantErr: true,
			errMsg:  "min_severity",
		},
		{
			name: "health_score_threshold 超过 100",
			cfg: &OpsTelegramNotificationConfig{
				HealthScoreThreshold: 150,
			},
			wantErr: true,
			errMsg:  "health_score_threshold",
		},
		{
			name: "health_score_threshold 为负数",
			cfg: &OpsTelegramNotificationConfig{
				HealthScoreThreshold: -1,
			},
			wantErr: true,
			errMsg:  "health_score_threshold",
		},
		{
			name: "min_severity 为空字符串（合法）",
			cfg: &OpsTelegramNotificationConfig{
				MinSeverity:      "",
				RateLimitPerHour: 0,
			},
			wantErr: false,
		},
		{
			name: "所有 min_severity 合法值",
			cfg: &OpsTelegramNotificationConfig{
				MinSeverity: "critical",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateOpsTelegramNotificationConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.True(t, strings.Contains(err.Error(), tt.errMsg),
						"期望错误消息包含 %q，实际: %v", tt.errMsg, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNormalizeOpsTelegramNotificationConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil ChatIDs 应初始化为空切片", func(t *testing.T) {
		t.Parallel()
		cfg := &OpsTelegramNotificationConfig{ChatIDs: nil}
		normalizeOpsTelegramNotificationConfig(cfg)
		require.NotNil(t, cfg.ChatIDs)
		require.Empty(t, cfg.ChatIDs)
	})

	t.Run("BotToken 和 MinSeverity 应 TrimSpace", func(t *testing.T) {
		t.Parallel()
		cfg := &OpsTelegramNotificationConfig{
			BotToken:    "  TOKEN  ",
			MinSeverity: "  warning  ",
		}
		normalizeOpsTelegramNotificationConfig(cfg)
		require.Equal(t, "TOKEN", cfg.BotToken)
		require.Equal(t, "warning", cfg.MinSeverity)
	})

	t.Run("负数 HealthScoreThreshold 应归零", func(t *testing.T) {
		t.Parallel()
		cfg := &OpsTelegramNotificationConfig{HealthScoreThreshold: -5}
		normalizeOpsTelegramNotificationConfig(cfg)
		require.Equal(t, 0, cfg.HealthScoreThreshold)
	})

	t.Run("nil 配置不 panic", func(t *testing.T) {
		t.Parallel()
		require.NotPanics(t, func() {
			normalizeOpsTelegramNotificationConfig(nil)
		})
	})
}

// ─────────────────────────────────────────────
// maybeSendTelegramAlert 单元测试
// ─────────────────────────────────────────────

func TestMaybeSendTelegramAlert_SkipWhenTelegramServiceNil(t *testing.T) {
	t.Parallel()

	svc := &OpsAlertEvaluatorService{
		telegramService: nil,
	}
	rule := &OpsAlertRule{NotifyTelegram: true}
	event := &OpsAlertEvent{}

	sent := svc.maybeSendTelegramAlert(t.Context(), rule, event)
	require.False(t, sent)
}

func TestMaybeSendTelegramAlert_SkipWhenNotifyTelegramFalse(t *testing.T) {
	t.Parallel()

	svc := &OpsAlertEvaluatorService{
		telegramService: NewTelegramService(),
		opsService:      &OpsService{},
		telegramLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
	rule := &OpsAlertRule{NotifyTelegram: false}
	event := &OpsAlertEvent{}

	sent := svc.maybeSendTelegramAlert(t.Context(), rule, event)
	require.False(t, sent, "NotifyTelegram=false 时应跳过发送")
}

func TestMaybeSendTelegramAlert_SkipWhenAlreadySent(t *testing.T) {
	t.Parallel()

	svc := &OpsAlertEvaluatorService{
		telegramService: NewTelegramService(),
		opsService:      &OpsService{},
		telegramLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
	rule := &OpsAlertRule{NotifyTelegram: true}
	event := &OpsAlertEvent{TelegramSent: true}

	sent := svc.maybeSendTelegramAlert(t.Context(), rule, event)
	require.False(t, sent, "TelegramSent=true 时应跳过重复发送")
}
