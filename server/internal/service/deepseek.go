package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ImageAnalysisInput struct {
	ImageDataURL  string `json:"image_data_url"`
	GarmentType   string `json:"garment_type"`
	MaterialType  string `json:"material_type"`
	MarketSegment string `json:"market_segment"`
	Urgency       string `json:"urgency"`
	Quantity      int    `json:"quantity"`
	Comment       string `json:"comment"`
}

type ImageAnalysisResult struct {
	Model                    string   `json:"model"`
	ProductSummary           string   `json:"product_summary"`
	SuggestedMarketSegment   string   `json:"suggested_market_segment"`
	EstimatedUnitPriceMinRUB int64    `json:"estimated_unit_price_min_rub"`
	EstimatedUnitPriceMidRUB int64    `json:"estimated_unit_price_mid_rub"`
	EstimatedUnitPriceMaxRUB int64    `json:"estimated_unit_price_max_rub"`
	EstimatedTotalMinRUB     int64    `json:"estimated_total_min_rub"`
	EstimatedTotalMidRUB     int64    `json:"estimated_total_mid_rub"`
	EstimatedTotalMaxRUB     int64    `json:"estimated_total_max_rub"`
	Confidence               string   `json:"confidence"`
	Factors                  []string `json:"factors"`
	Assumptions              []string `json:"assumptions"`
	Reasoning                string   `json:"reasoning"`
	Disclaimer               string   `json:"disclaimer"`
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
	Content any    `json:"content"`
}

type deepSeekContentPart struct {
	Type     string               `json:"type"`
	Text     string               `json:"text,omitempty"`
	ImageURL *deepSeekImageSource `json:"image_url,omitempty"`
}

type deepSeekImageSource struct {
	URL string `json:"url"`
}

type deepSeekChatResponse struct {
	Model   string               `json:"model"`
	Choices []deepSeekChatChoice `json:"choices"`
}

type deepSeekChatChoice struct {
	Message deepSeekChoiceMessage `json:"message"`
}

type deepSeekChoiceMessage struct {
	Content string `json:"content"`
}

type deepSeekImageEstimate struct {
	ProductSummary           string   `json:"product_summary"`
	SuggestedMarketSegment   string   `json:"suggested_market_segment"`
	EstimatedUnitPriceMinRUB int64    `json:"estimated_unit_price_min_rub"`
	EstimatedUnitPriceMidRUB int64    `json:"estimated_unit_price_mid_rub"`
	EstimatedUnitPriceMaxRUB int64    `json:"estimated_unit_price_max_rub"`
	Confidence               string   `json:"confidence"`
	Factors                  []string `json:"factors"`
	Assumptions              []string `json:"assumptions"`
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

func (c *DeepSeekClient) AnalyzeGarmentImage(
	ctx context.Context,
	input ImageAnalysisInput,
	settings UserSettings,
) (ImageAnalysisResult, error) {
	if c == nil {
		return ImageAnalysisResult{}, fmt.Errorf("deepseek integration is not configured: %w", ErrNotFound)
	}
	if strings.TrimSpace(input.ImageDataURL) == "" {
		return ImageAnalysisResult{}, fmt.Errorf("image_data_url is required: %w", ErrInvalidArgument)
	}
	if input.Quantity <= 0 {
		return ImageAnalysisResult{}, fmt.Errorf("quantity should be positive: %w", ErrInvalidArgument)
	}
	if len(input.ImageDataURL) > 12*1024*1024 {
		return ImageAnalysisResult{}, fmt.Errorf("image is too large for analysis: %w", ErrInvalidArgument)
	}

	requestBody := deepSeekChatRequest{
		Model:       c.model,
		Temperature: 0.2,
		MaxTokens:   1200,
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
				Role: "user",
				Content: []deepSeekContentPart{
					{
						Type: "text",
						Text: buildDeepSeekUserPrompt(input, settings),
					},
					{
						Type: "image_url",
						ImageURL: &deepSeekImageSource{
							URL: input.ImageDataURL,
						},
					},
				},
			},
		},
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return ImageAnalysisResult{}, fmt.Errorf("marshal deepseek request: %w", err)
	}

	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		result, retryable, callErr := c.sendAnalysisRequest(ctx, payload, input.Quantity)
		if callErr == nil {
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
			return ImageAnalysisResult{}, ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}

	return ImageAnalysisResult{}, lastErr
}

func (c *DeepSeekClient) sendAnalysisRequest(
	ctx context.Context,
	payload []byte,
	quantity int,
) (ImageAnalysisResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return ImageAnalysisResult{}, false, fmt.Errorf("build deepseek request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ImageAnalysisResult{}, isRetryableError(err.Error()), fmt.Errorf("deepseek request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return ImageAnalysisResult{}, false, fmt.Errorf("read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return ImageAnalysisResult{}, isRetryableStatus(resp.StatusCode), fmt.Errorf("deepseek api request failed: %s", classifyDeepSeekError(resp.StatusCode, message))
	}

	var completion deepSeekChatResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return ImageAnalysisResult{}, false, fmt.Errorf("parse deepseek response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return ImageAnalysisResult{}, false, fmt.Errorf("deepseek api returned no choices")
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return ImageAnalysisResult{}, false, fmt.Errorf("deepseek api returned empty content")
	}

	var estimate deepSeekImageEstimate
	if err := json.Unmarshal([]byte(content), &estimate); err != nil {
		return ImageAnalysisResult{}, false, fmt.Errorf("parse deepseek estimate json: %w; raw=%s", err, content)
	}

	estimate = normalizeImageEstimate(estimate)
	return ImageAnalysisResult{
		Model:                    completion.Model,
		ProductSummary:           estimate.ProductSummary,
		SuggestedMarketSegment:   estimate.SuggestedMarketSegment,
		EstimatedUnitPriceMinRUB: estimate.EstimatedUnitPriceMinRUB,
		EstimatedUnitPriceMidRUB: estimate.EstimatedUnitPriceMidRUB,
		EstimatedUnitPriceMaxRUB: estimate.EstimatedUnitPriceMaxRUB,
		EstimatedTotalMinRUB:     estimate.EstimatedUnitPriceMinRUB * int64(quantity),
		EstimatedTotalMidRUB:     estimate.EstimatedUnitPriceMidRUB * int64(quantity),
		EstimatedTotalMaxRUB:     estimate.EstimatedUnitPriceMaxRUB * int64(quantity),
		Confidence:               estimate.Confidence,
		Factors:                  estimate.Factors,
		Assumptions:              estimate.Assumptions,
		Reasoning:                estimate.Reasoning,
		Disclaimer:               "Оценка построена нейросетью по изображению и факторам, это не коммерческая оферта и не финальный расчёт.",
	}, false, nil
}

func buildDeepSeekSystemPrompt(settings UserSettings) string {
	return fmt.Sprintf(
		`You are a senior garment production estimator for the Russian fashion market.
You analyze one garment image plus user-provided hints and return a conservative price estimate in Russian rubles.
Return valid JSON only. Do not wrap in markdown. Do not add any commentary outside JSON.

Required JSON schema:
{
  "product_summary": "short sentence",
  "suggested_market_segment": "Массмаркет | Средний | Премиум",
  "estimated_unit_price_min_rub": 0,
  "estimated_unit_price_mid_rub": 0,
  "estimated_unit_price_max_rub": 0,
  "confidence": "low | medium | high",
  "factors": ["factor 1", "factor 2"],
  "assumptions": ["assumption 1", "assumption 2"],
  "reasoning": "short paragraph in Russian"
}

Rules:
- Base the estimate on visible construction complexity, silhouette, finishing, probable fabric behavior, mass-production difficulty, urgency, quantity and target market.
- Respond in Russian.
- All price fields must be integers in RUB for one unit.
- estimated_unit_price_min_rub <= estimated_unit_price_mid_rub <= estimated_unit_price_max_rub.
- If the image is ambiguous, lower confidence and state assumptions explicitly.
- Use these user market bands for grounding: %s.
- If the user selected a market segment, keep the estimate near that segment unless the image strongly contradicts it.`,
		formatMarketBandsForPrompt(settings.MarketBands),
	)
}

func buildDeepSeekUserPrompt(input ImageAnalysisInput, settings UserSettings) string {
	return fmt.Sprintf(
		`Оцени изделие на фото для российского рынка.

Факторы:
- Подсказка по изделию: %s
- Подсказка по материалу: %s
- Целевой сегмент: %s
- Срочность: %s
- Размер партии: %d
- Комментарий пользователя: %s

Доступные пресеты пользователя:
- Изделия: %s
- Материалы: %s
- Срочность: %s

Сначала определи тип изделия и его сложность по изображению, затем оцени диапазон цены за единицу для RU рынка.
Ответ должен быть только JSON и содержать слово json implicitly by being valid JSON.`,
		emptyOrUnknown(input.GarmentType),
		emptyOrUnknown(input.MaterialType),
		emptyOrUnknown(input.MarketSegment),
		emptyOrUnknown(input.Urgency),
		input.Quantity,
		emptyOrUnknown(strings.TrimSpace(input.Comment)),
		strings.Join(sortedKeysFromGarments(settings.Garments), ", "),
		strings.Join(sortedKeysFromMaterials(settings.Materials), ", "),
		strings.Join(sortedKeysFromUrgency(settings.Urgency), ", "),
	)
}

func normalizeImageEstimate(item deepSeekImageEstimate) deepSeekImageEstimate {
	item.ProductSummary = strings.TrimSpace(item.ProductSummary)
	item.SuggestedMarketSegment = strings.TrimSpace(item.SuggestedMarketSegment)
	item.Confidence = normalizeConfidence(item.Confidence)
	item.Reasoning = strings.TrimSpace(item.Reasoning)
	item.Factors = compactStrings(item.Factors)
	item.Assumptions = compactStrings(item.Assumptions)

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

func ParseConfidenceLabel(value string) string {
	switch normalizeConfidence(value) {
	case "high":
		return "Высокая"
	case "low":
		return "Низкая"
	default:
		return "Средняя"
	}
}

func FormatRub(value int64) string {
	return strconv.FormatInt(value, 10)
}
