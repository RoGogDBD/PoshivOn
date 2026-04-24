package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/metrics"
)

type MarketFeedbackInput struct {
	GarmentType     string                          `json:"garment_type"`
	MaterialType    string                          `json:"material_type"`
	MarketSegment   string                          `json:"market_segment"`
	Urgency         string                          `json:"urgency"`
	Quantity        int                             `json:"quantity"`
	Fittings        int                             `json:"fittings"`
	IsCustomFigure  bool                            `json:"is_custom_figure"`
	IsChild         bool                            `json:"is_child"`
	Comment         string                          `json:"comment"`
	OperationCounts map[string]int                  `json:"operation_counts,omitempty"`
	Calculation     *MarketFeedbackCalculationInput `json:"calculation,omitempty"`
}

type MarketFeedbackCalculationInput struct {
	CalculationMode        string `json:"calculation_mode,omitempty"`
	BasePricePerUnitRUB    int64  `json:"base_price_per_unit_rub,omitempty"`
	CostPricePerUnitRUB    int64  `json:"cost_price_per_unit_rub,omitempty"`
	PriceBeforeDiscountRUB int64  `json:"price_before_discount_rub,omitempty"`
	MinAllowedPriceRUB     int64  `json:"min_allowed_price_rub,omitempty"`
	FinalPricePerUnitRUB   int64  `json:"final_price_per_unit_rub,omitempty"`
	FinalTotalRUB          int64  `json:"final_total_rub,omitempty"`
	DiscountPercent        int64  `json:"discount_percent,omitempty"`
	DiscountAmountRUB      int64  `json:"discount_amount_rub,omitempty"`
	MarketStatus           string `json:"market_status,omitempty"`
}

type MarketFeedbackBand struct {
	Label      string `json:"label"`
	MinRUB     int64  `json:"min_rub"`
	AverageRUB int64  `json:"average_rub"`
	MaxRUB     int64  `json:"max_rub"`
}

type MarketFeedbackResult struct {
	Model                    string              `json:"model"`
	ScenarioSummary          string              `json:"scenario_summary"`
	SuggestedMarketSegment   string              `json:"suggested_market_segment"`
	EstimatedUnitPriceMinRUB int64               `json:"estimated_unit_price_min_rub"`
	EstimatedUnitPriceMidRUB int64               `json:"estimated_unit_price_mid_rub"`
	EstimatedUnitPriceMaxRUB int64               `json:"estimated_unit_price_max_rub"`
	EstimatedTotalMinRUB     int64               `json:"estimated_total_min_rub"`
	EstimatedTotalMidRUB     int64               `json:"estimated_total_mid_rub"`
	EstimatedTotalMaxRUB     int64               `json:"estimated_total_max_rub"`
	SelectedMarketBand       *MarketFeedbackBand `json:"selected_market_band,omitempty"`
	PricePosition            string              `json:"price_position"`
	Confidence               string              `json:"confidence"`
	KeyDrivers               []string            `json:"key_drivers"`
	Risks                    []string            `json:"risks"`
	Recommendations          []string            `json:"recommendations"`
	Reasoning                string              `json:"reasoning"`
	Disclaimer               string              `json:"disclaimer"`
}

type DeepSeekConfig struct {
	APIKey        string
	APIEndpoint   string
	Model         string
	Timeout       time.Duration
	MaxRetries    int
	ConnectTimout time.Duration
}

type DeepSeekClient struct {
	httpClient *http.Client
	apiKey     string
	apiURL     string
	model      string
	maxRetries int
}

type deepSeekChatRequest struct {
	Model          string             `json:"model"`
	Messages       []deepSeekMessage  `json:"messages"`
	Temperature    float64            `json:"temperature"`
	MaxTokens      int                `json:"max_tokens"`
	Stream         bool               `json:"stream"`
	ResponseFormat deepSeekJSONFormat `json:"response_format"`
}

type deepSeekJSONFormat struct {
	Type string `json:"type"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type deepSeekChatResponse struct {
	Model   string               `json:"model"`
	Choices []deepSeekChatChoice `json:"choices"`
	Usage   deepSeekUsage        `json:"usage"`
}

type deepSeekChatChoice struct {
	Message deepSeekChoiceMessage `json:"message"`
}

type deepSeekChoiceMessage struct {
	Content string `json:"content"`
}

type deepSeekFeedbackPayload struct {
	ScenarioSummary          string   `json:"scenario_summary"`
	SuggestedMarketSegment   string   `json:"suggested_market_segment"`
	EstimatedUnitPriceMinRUB int64    `json:"estimated_unit_price_min_rub"`
	EstimatedUnitPriceMidRUB int64    `json:"estimated_unit_price_mid_rub"`
	EstimatedUnitPriceMaxRUB int64    `json:"estimated_unit_price_max_rub"`
	Confidence               string   `json:"confidence"`
	KeyDrivers               []string `json:"key_drivers"`
	Risks                    []string `json:"risks"`
	Recommendations          []string `json:"recommendations"`
	Reasoning                string   `json:"reasoning"`
}

func NewDeepSeekClient(cfg DeepSeekConfig) (*DeepSeekClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, nil
	}
	if strings.TrimSpace(cfg.APIEndpoint) == "" {
		return nil, fmt.Errorf("deepseek api endpoint is required: %w", ErrInvalidArgument)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	if cfg.ConnectTimout <= 0 {
		cfg.ConnectTimout = 10 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	transport := &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   cfg.ConnectTimout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &DeepSeekClient{
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
		apiKey:     cfg.APIKey,
		apiURL:     cfg.APIEndpoint,
		model:      cfg.Model,
		maxRetries: cfg.MaxRetries,
	}, nil
}

func (c *DeepSeekClient) AnalyzeMarketFeedback(
	ctx context.Context,
	input MarketFeedbackInput,
	settings UserSettings,
) (MarketFeedbackResult, error) {
	if c == nil {
		return MarketFeedbackResult{}, fmt.Errorf("deepseek integration is not configured: %w", ErrNotFound)
	}
	if input.Quantity <= 0 {
		return MarketFeedbackResult{}, fmt.Errorf("quantity should be positive: %w", ErrInvalidArgument)
	}

	requestBody := deepSeekChatRequest{
		Model:       c.model,
		Temperature: 0.2,
		MaxTokens:   1400,
		Stream:      false,
		ResponseFormat: deepSeekJSONFormat{
			Type: "json_object",
		},
		Messages: []deepSeekMessage{
			{
				Role:    "system",
				Content: buildDeepSeekSystemPrompt(settings),
			},
			{
				Role:    "user",
				Content: buildDeepSeekUserPrompt(input, settings),
			},
		},
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return MarketFeedbackResult{}, fmt.Errorf("marshal deepseek request: %w", err)
	}

	start := time.Now()
	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		result, retryable, callErr := c.sendFeedbackRequest(ctx, payload, input, settings)
		if callErr == nil {
			metrics.AIRequestDuration.Observe(time.Since(start).Seconds())
			metrics.AIRequestsTotal.WithLabelValues("success").Inc()
			return result, nil
		}
		lastErr = callErr
		if !retryable || attempt == c.maxRetries {
			break
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return MarketFeedbackResult{}, ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}

	metrics.AIRequestDuration.Observe(time.Since(start).Seconds())
	metrics.AIRequestsTotal.WithLabelValues("error").Inc()
	return MarketFeedbackResult{}, lastErr
}

func (c *DeepSeekClient) sendFeedbackRequest(
	ctx context.Context,
	payload []byte,
	input MarketFeedbackInput,
	settings UserSettings,
) (MarketFeedbackResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return MarketFeedbackResult{}, false, fmt.Errorf("build deepseek request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return MarketFeedbackResult{}, isRetryableError(err.Error()), fmt.Errorf("deepseek request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return MarketFeedbackResult{}, false, fmt.Errorf("read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return MarketFeedbackResult{}, isRetryableStatus(resp.StatusCode), fmt.Errorf("deepseek api request failed: %s", classifyDeepSeekError(resp.StatusCode, message))
	}

	var completion deepSeekChatResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return MarketFeedbackResult{}, false, fmt.Errorf("parse deepseek response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return MarketFeedbackResult{}, false, fmt.Errorf("deepseek api returned no choices")
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return MarketFeedbackResult{}, false, fmt.Errorf("deepseek api returned empty content")
	}

	var feedback deepSeekFeedbackPayload
	if err := json.Unmarshal([]byte(content), &feedback); err != nil {
		return MarketFeedbackResult{}, false, fmt.Errorf("parse deepseek feedback json: %w; raw=%s", err, content)
	}

	if completion.Usage.PromptTokens > 0 {
		metrics.AITokensTotal.WithLabelValues("prompt").Add(float64(completion.Usage.PromptTokens))
	}
	if completion.Usage.CompletionTokens > 0 {
		metrics.AITokensTotal.WithLabelValues("completion").Add(float64(completion.Usage.CompletionTokens))
	}

	feedback = normalizeFeedbackPayload(feedback)
	selectedBand := resolveMarketBand(settings.MarketBands, input.MarketSegment)
	return MarketFeedbackResult{
		Model:                    completion.Model,
		ScenarioSummary:          feedback.ScenarioSummary,
		SuggestedMarketSegment:   feedback.SuggestedMarketSegment,
		EstimatedUnitPriceMinRUB: feedback.EstimatedUnitPriceMinRUB,
		EstimatedUnitPriceMidRUB: feedback.EstimatedUnitPriceMidRUB,
		EstimatedUnitPriceMaxRUB: feedback.EstimatedUnitPriceMaxRUB,
		EstimatedTotalMinRUB:     feedback.EstimatedUnitPriceMinRUB * int64(input.Quantity),
		EstimatedTotalMidRUB:     feedback.EstimatedUnitPriceMidRUB * int64(input.Quantity),
		EstimatedTotalMaxRUB:     feedback.EstimatedUnitPriceMaxRUB * int64(input.Quantity),
		SelectedMarketBand:       selectedBand,
		PricePosition:            detectFeedbackMarketPosition(selectedBand, feedback.EstimatedUnitPriceMidRUB),
		Confidence:               feedback.Confidence,
		KeyDrivers:               feedback.KeyDrivers,
		Risks:                    feedback.Risks,
		Recommendations:          feedback.Recommendations,
		Reasoning:                feedback.Reasoning,
		Disclaimer:               "AI-фидбек опирается на ваши параметры и пользовательские пресеты. Это аналитическая подсказка, а не финальная коммерческая оферта.",
	}, false, nil
}

func buildDeepSeekSystemPrompt(settings UserSettings) string {
	return fmt.Sprintf(`You are a senior garment production and pricing analyst for the Russian apparel market.
You receive structured production parameters, user pricing presets and, when available, the final manual calculation produced by the system. Your task is to assess market fit, give a realistic unit-price range in RUB, explain the drivers, outline risks, and recommend next actions.
Return valid JSON only. Do not wrap in markdown. No prose outside JSON.

Required JSON schema:
{
  "scenario_summary": "short Russian summary",
  "suggested_market_segment": "Массмаркет | Средний | Премиум",
  "estimated_unit_price_min_rub": 0,
  "estimated_unit_price_mid_rub": 0,
  "estimated_unit_price_max_rub": 0,
  "confidence": "low | medium | high",
  "key_drivers": ["driver 1", "driver 2"],
  "risks": ["risk 1", "risk 2"],
  "recommendations": ["action 1", "action 2"],
  "reasoning": "short Russian paragraph"
}

Rules:
- Respond only in Russian.
- All price fields are integer RUB per unit.
- min <= mid <= max.
- Use the user pricing presets as your source of production logic.
- If a completed manual calculation is provided, treat it as the main quote under review and explicitly evaluate whether it is justified by the scenario.
- Use these user market bands for grounding: %s.
- Take into account quantity, urgency, fittings, custom figure, child product and chosen operations.
- Keep recommendations practical for RU fashion manufacturing.`, formatMarketBandsForPrompt(settings.MarketBands))
}

func buildDeepSeekUserPrompt(input MarketFeedbackInput, settings UserSettings) string {
	return fmt.Sprintf(`Проанализируй параметры производственного расчёта и дай фидбек для рынка РФ.

Сценарий:
- Изделие: %s
- Материал: %s
- Целевой сегмент: %s
- Срочность: %s
- Размер партии: %d
- Примерки: %d
- Нестандартная фигура: %t
- Детское изделие: %t
- Выбранные операции: %s
- Комментарий: %s

Пользовательские пресеты:
- Правила ценообразования: %s
- Изделия: %s
- Материалы: %s
- Срочность: %s
- Рыночные сегменты: %s

Готовый расчет системы:
%s

Нужно:
1. Оценить реалистичный диапазон цены за единицу для рынка РФ.
2. Сказать, попадает ли готовый расчет в выбранный сегмент или тяготеет к другому.
3. Назвать ключевые драйверы цены.
4. Отметить риски и практические рекомендации.
Ответ только JSON.`,
		emptyOrUnknown(input.GarmentType),
		emptyOrUnknown(input.MaterialType),
		emptyOrUnknown(input.MarketSegment),
		emptyOrUnknown(input.Urgency),
		input.Quantity,
		input.Fittings,
		input.IsCustomFigure,
		input.IsChild,
		formatOperationCountsForPrompt(input.OperationCounts),
		emptyOrUnknown(strings.TrimSpace(input.Comment)),
		formatPricingRulesForPrompt(settings.PricingRules),
		strings.Join(sortedKeysFromGarments(settings.Garments), ", "),
		strings.Join(sortedKeysFromMaterials(settings.Materials), ", "),
		strings.Join(sortedKeysFromUrgency(settings.Urgency), ", "),
		formatMarketBandsForPrompt(settings.MarketBands),
		formatCalculationForPrompt(input.Calculation),
	)
}

func normalizeFeedbackPayload(item deepSeekFeedbackPayload) deepSeekFeedbackPayload {
	item.ScenarioSummary = strings.TrimSpace(item.ScenarioSummary)
	item.SuggestedMarketSegment = strings.TrimSpace(item.SuggestedMarketSegment)
	item.Confidence = normalizeConfidence(item.Confidence)
	item.KeyDrivers = compactStrings(item.KeyDrivers)
	item.Risks = compactStrings(item.Risks)
	item.Recommendations = compactStrings(item.Recommendations)
	item.Reasoning = strings.TrimSpace(item.Reasoning)

	if item.EstimatedUnitPriceMinRUB < 0 {
		item.EstimatedUnitPriceMinRUB = 0
	}
	if item.EstimatedUnitPriceMidRUB < item.EstimatedUnitPriceMinRUB {
		item.EstimatedUnitPriceMidRUB = item.EstimatedUnitPriceMinRUB
	}
	if item.EstimatedUnitPriceMaxRUB < item.EstimatedUnitPriceMidRUB {
		item.EstimatedUnitPriceMaxRUB = item.EstimatedUnitPriceMidRUB
	}
	return item
}

func resolveMarketBand(items map[string]MarketBand, name string) *MarketFeedbackBand {
	key := strings.TrimSpace(name)
	if key == "" {
		return nil
	}
	band, ok := items[key]
	if !ok {
		return nil
	}
	return &MarketFeedbackBand{
		Label:      key,
		MinRUB:     band.MinPricePerUnit,
		AverageRUB: band.AveragePricePerUnit,
		MaxRUB:     band.MaxPricePerUnit,
	}
}

func detectFeedbackMarketPosition(band *MarketFeedbackBand, estimatedMid int64) string {
	if band == nil || estimatedMid <= 0 {
		return "unknown"
	}
	if estimatedMid < band.MinRUB {
		return "below_market"
	}
	if estimatedMid > band.MaxRUB {
		return "above_market"
	}
	return "in_market"
}

func normalizeConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "medium"
	}
}

func compactStrings(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func emptyOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "не указано"
	}
	return strings.TrimSpace(value)
}

func formatOperationCountsForPrompt(items map[string]int) string {
	if len(items) == 0 {
		return "нет"
	}
	keys := make([]string, 0, len(items))
	for key, count := range items {
		if count > 0 {
			keys = append(keys, fmt.Sprintf("%s x%d", key, count))
		}
	}
	if len(keys) == 0 {
		return "нет"
	}
	sortStrings(keys)
	return strings.Join(keys, ", ")
}

func formatPricingRulesForPrompt(rules PricingRules) string {
	return fmt.Sprintf(
		"ставка минуты %d RUB; налоги %.2f%%; накладные %.2f%%; логистика %d RUB/шт; маржа %.2f%%; мин. маржа %.2f%%; включено примерок %d; доп. примерка %d мин",
		rules.LaborMinuteRate,
		rules.PayrollTaxesPercent,
		rules.OverheadPercent,
		rules.LogisticsCostPerUnit,
		rules.MarginPercent,
		rules.MinMarginPercent,
		rules.IncludedFittings,
		rules.ExtraFittingMinutes,
	)
}

func formatCalculationForPrompt(item *MarketFeedbackCalculationInput) string {
	if item == nil {
		return "- готовый расчет не передан"
	}

	return fmt.Sprintf(
		"- Режим: %s\n- База/минимум за единицу: %d RUB\n- Себестоимость за единицу: %d RUB\n- Цена до скидки за единицу: %d RUB\n- Минимально допустимая цена за единицу: %d RUB\n- Финальная цена за единицу: %d RUB\n- Финальная сумма за партию: %d RUB\n- Скидка: %d%% (%d RUB)\n- Статус относительно рынка по внутренней логике: %s",
		emptyOrUnknown(item.CalculationMode),
		item.BasePricePerUnitRUB,
		item.CostPricePerUnitRUB,
		item.PriceBeforeDiscountRUB,
		item.MinAllowedPriceRUB,
		item.FinalPricePerUnitRUB,
		item.FinalTotalRUB,
		item.DiscountPercent,
		item.DiscountAmountRUB,
		emptyOrUnknown(item.MarketStatus),
	)
}

func formatMarketBandsForPrompt(items map[string]MarketBand) string {
	keys := sortedKeysFromMarketBands(items)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		band := items[key]
		parts = append(parts, fmt.Sprintf("%s: %d-%d RUB, средняя %d RUB", key, band.MinPricePerUnit, band.MaxPricePerUnit, band.AveragePricePerUnit))
	}
	return strings.Join(parts, "; ")
}

func sortedKeysFromGarments(items map[string]GarmentConfig) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortedKeysFromMaterials(items map[string]MaterialConfig) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortedKeysFromUrgency(items map[string]UrgencyRule) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortedKeysFromMarketBands(items map[string]MarketBand) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(items []string) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if strings.Compare(items[i], items[j]) > 0 {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func classifyDeepSeekError(statusCode int, body string) string {
	switch statusCode {
	case http.StatusUnauthorized:
		return "invalid_api_key"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "timeout"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		return strings.TrimSpace(body)
	}
}

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func isRetryableError(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "temporary") ||
		strings.Contains(lower, "503") ||
		strings.Contains(lower, "502") ||
		strings.Contains(lower, "504") ||
		strings.Contains(lower, "429")
}
