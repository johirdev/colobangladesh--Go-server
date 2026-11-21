package autofill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// writer turns the gathered facts into publish-ready copy with Claude.
// The API key is read from ANTHROPIC_API_KEY by the SDK.
type writer struct {
	client anthropic.Client
	model  string
}

func newWriter(model string) *writer {
	if model == "" {
		model = "claude-opus-5-5"
	}
	return &writer{
		client: anthropic.NewClient(option.WithRequestTimeout(90 * time.Second)),
		model:  model,
	}
}

type aiCopy struct {
	BnTitle      string   `json:"bn_title"`
	Subtitle     string   `json:"subtitle"`
	Description  string   `json:"description"`
	Highlights   []string `json:"highlights"`
	BestTime     string   `json:"best_time_to_visit"`
	Category     string   `json:"category"`
	LocationName string   `json:"location_name"`
}

var aiSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"bn_title", "subtitle", "description", "highlights", "best_time_to_visit", "category", "location_name"},
	"properties": map[string]any{
		"bn_title":           map[string]any{"type": "string", "description": "Name in Bangla script"},
		"subtitle":           map[string]any{"type": "string", "description": "One catchy line, max 160 characters"},
		"description":        map[string]any{"type": "string", "description": "3-4 short paragraphs separated by blank lines"},
		"highlights":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "4-6 things to see or do"},
		"best_time_to_visit": map[string]any{"type": "string", "description": "Month range, e.g. November – March"},
		"category": map[string]any{"type": "string", "enum": []string{"beach", "island", "hill", "forest", "waterfall",
			"lake", "river", "wetland", "heritage", "religious", "park", "city", "other"}},
		"location_name": map[string]any{"type": "string", "description": "Upazila/thana and district, e.g. Teknaf, Cox's Bazar"},
	},
}

const systemPrompt = `You write listings for CholoBangladesh, a travel marketplace where tourists book local guides in Bangladesh.
Write in clear, friendly English for foreign and local travellers. Stay factual: use the source text you are given and
well-established public knowledge about the place; do not invent prices, phone numbers, opening hours or events.
If the sources do not cover something, keep that field short and general rather than guessing specifics.`

func (w *writer) write(ctx context.Context, name string, res *Result) error {
	var facts strings.Builder
	fmt.Fprintf(&facts, "Tourist spot: %s\n", name)
	if res.Title != "" && !strings.EqualFold(res.Title, name) {
		fmt.Fprintf(&facts, "Wikipedia title: %s\n", res.Title)
	}
	if res.Subtitle != "" {
		fmt.Fprintf(&facts, "Short description: %s\n", res.Subtitle)
	}
	if res.Description != "" {
		fmt.Fprintf(&facts, "\nSource text (Wikipedia):\n%s\n", res.Description)
	}
	if res.Latitude != nil && res.Longitude != nil {
		fmt.Fprintf(&facts, "\nCoordinates: %.5f, %.5f\n", *res.Latitude, *res.Longitude)
	}
	facts.WriteString("\nWrite the listing fields for this tourist spot.")

	ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	msg, err := w.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(w.model),
		MaxTokens: 4000,
		System:    []anthropic.BetaTextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(facts.String())),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortLow, // short copywriting task
			Format: anthropic.BetaJSONOutputFormatParam{Schema: aiSchema},
		},
		// If a safety classifier declines, the request is re-served by a fallback model.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	})
	if err != nil {
		return err
	}
	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		return errors.New("model declined the request")
	}
	var text strings.Builder
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out aiCopy
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return fmt.Errorf("parse ai output: %w", err)
	}

	set := func(dst *string, v string, max int) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = truncate(v, max)
		}
	}
	set(&res.BnTitle, out.BnTitle, 150)
	set(&res.Subtitle, out.Subtitle, 300)
	set(&res.Description, out.Description, 20000)
	set(&res.BestTime, out.BestTime, 120)
	set(&res.Category, out.Category, 30)
	set(&res.Location, out.LocationName, 200)
	for _, h := range out.Highlights {
		if h = truncate(h, 150); len(h) >= 2 && len(res.Highlights) < 20 {
			res.Highlights = append(res.Highlights, h)
		}
	}
	return nil
}
