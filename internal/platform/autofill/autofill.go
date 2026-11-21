// Package autofill gathers public information about a tourist spot from a name so a
// moderator gets a pre-filled form instead of a blank one.
//
// Sources (each one is optional except Wikipedia):
//
//	Wikipedia / Wikimedia Commons  always on, no key   text, coordinates, photos
//	YouTube Data API v3            YOUTUBE_API_KEY      travel videos
//	Google Custom Search (images)  GOOGLE_SEARCH_API_KEY + GOOGLE_SEARCH_CX   more photos
//	Claude (Anthropic API)         AI_AUTOFILL_ENABLED=true + ANTHROPIC_API_KEY
//	                               rewrites the facts into subtitle, description, highlights,
//	                               best time to visit, Bangla title and category
//
// Nothing here is saved; the result is a draft the moderator reviews.
package autofill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	YouTubeAPIKey      string
	GoogleSearchAPIKey string
	GoogleSearchCX     string
	AIEnabled          bool   // needs ANTHROPIC_API_KEY in the environment
	AIModel            string // default claude-opus-5-5
}

type Image struct {
	URL       string
	Caption   string
	SourceURL string
}

type Video struct {
	URL   string
	Title string
}

// Result is everything that could be found. Empty fields mean "not found".
type Result struct {
	Title       string
	BnTitle     string
	Subtitle    string
	Description string
	Highlights  []string
	BestTime    string
	Category    string
	Location    string
	Latitude    *float64
	Longitude   *float64
	SourceURL   string
	Images      []Image
	Videos      []Video
	Sources     []string
	Notes       []string
}

var (
	ErrNotFound = errors.New("autofill: nothing found")
	ErrUpstream = errors.New("autofill: upstream error")
)

type Client struct {
	cfg  Config
	http *http.Client
	ai   *writer // nil when AI is off
}

const userAgent = "CholoBangladesh/1.0 (tourist spot autofill)"

func New(cfg Config) *Client {
	c := &Client{cfg: cfg, http: &http.Client{Timeout: 12 * time.Second}}
	if cfg.AIEnabled {
		c.ai = newWriter(cfg.AIModel)
	}
	return c
}

// Lookup searches every configured source for `name` (a Bangladeshi tourist spot).
func (c *Client) Lookup(ctx context.Context, name string) (*Result, error) {
	res := &Result{Title: strings.TrimSpace(name)}

	wikiErr := c.wikipedia(ctx, name, res)
	if wikiErr != nil && !errors.Is(wikiErr, ErrNotFound) {
		slog.Warn("autofill wikipedia failed", "err", wikiErr)
		res.Notes = append(res.Notes, "Wikipedia could not be reached")
	}

	if c.cfg.YouTubeAPIKey != "" {
		if err := c.youtube(ctx, res.Title, res); err != nil {
			slog.Warn("autofill youtube failed", "err", err)
			res.Notes = append(res.Notes, "YouTube search failed")
		}
	} else {
		res.Notes = append(res.Notes, "YouTube: set YOUTUBE_API_KEY to fetch videos automatically")
	}

	if c.cfg.GoogleSearchAPIKey != "" && c.cfg.GoogleSearchCX != "" {
		if err := c.googleImages(ctx, res.Title, res); err != nil {
			slog.Warn("autofill google images failed", "err", err)
			res.Notes = append(res.Notes, "Google image search failed")
		}
	} else {
		res.Notes = append(res.Notes, "Google images: set GOOGLE_SEARCH_API_KEY and GOOGLE_SEARCH_CX for more photos")
	}

	if c.ai != nil {
		if err := c.ai.write(ctx, name, res); err != nil {
			slog.Warn("autofill ai failed", "err", err)
			res.Notes = append(res.Notes, "AI writing failed, showing source text only")
		} else {
			res.Sources = append(res.Sources, "ai")
		}
	} else {
		res.Notes = append(res.Notes, "AI writing: set AI_AUTOFILL_ENABLED=true and ANTHROPIC_API_KEY for a ready-to-publish description")
	}

	if res.Description == "" && len(res.Images) == 0 && len(res.Videos) == 0 {
		if wikiErr != nil && !errors.Is(wikiErr, ErrNotFound) {
			return nil, ErrUpstream
		}
		return nil, ErrNotFound
	}
	return res, nil
}

// ------------------------------------------------------------------ http helpers

func (c *Client) getJSON(ctx context.Context, rawURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s returned %d", ErrUpstream, req.URL.Host, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(dst)
}

// ------------------------------------------------------------------ Wikipedia

func (c *Client) wikipedia(ctx context.Context, name string, res *Result) error {
	var search struct {
		Pages []struct {
			Key   string `json:"key"`
			Title string `json:"title"`
		} `json:"pages"`
	}
	q := url.Values{"q": {name + " Bangladesh"}, "limit": {"3"}}
	if err := c.getJSON(ctx, "https://en.wikipedia.org/w/rest.php/v1/search/page?"+q.Encode(), &search); err != nil {
		return err
	}
	if len(search.Pages) == 0 {
		// retry without the country hint
		q.Set("q", name)
		if err := c.getJSON(ctx, "https://en.wikipedia.org/w/rest.php/v1/search/page?"+q.Encode(), &search); err != nil {
			return err
		}
		if len(search.Pages) == 0 {
			return ErrNotFound
		}
	}
	key := url.PathEscape(search.Pages[0].Key)

	var sum struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Extract     string `json:"extract"`
		Coordinates *struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"coordinates"`
		OriginalImage *struct {
			Source string `json:"source"`
		} `json:"originalimage"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	if err := c.getJSON(ctx, "https://en.wikipedia.org/api/rest_v1/page/summary/"+key+"?redirect=true", &sum); err != nil {
		return err
	}
	if sum.Type == "disambiguation" || sum.Extract == "" {
		return ErrNotFound
	}
	res.Title = sum.Title
	res.Description = sum.Extract
	res.Subtitle = sum.Description
	res.SourceURL = sum.ContentURLs.Desktop.Page
	if sum.Coordinates != nil {
		lat, lng := sum.Coordinates.Lat, sum.Coordinates.Lon
		res.Latitude, res.Longitude = &lat, &lng
	}
	res.Sources = append(res.Sources, "wikipedia")

	seen := map[string]bool{}
	add := func(raw, caption string) {
		u := wikimediaThumb(raw)
		if u == "" || seen[u] || len(res.Images) >= 15 {
			return
		}
		seen[u] = true
		res.Images = append(res.Images, Image{URL: u, Caption: caption, SourceURL: commonsPage(u)})
	}
	if sum.OriginalImage != nil {
		add(sum.OriginalImage.Source, sum.Title)
	}

	var media struct {
		Items []struct {
			Title   string `json:"title"`
			Type    string `json:"type"`
			Caption *struct {
				Text string `json:"text"`
			} `json:"caption"`
			Srcset []struct {
				Src string `json:"src"`
			} `json:"srcset"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "https://en.wikipedia.org/api/rest_v1/page/media-list/"+key+"?redirect=true", &media); err == nil {
		for _, it := range media.Items {
			if it.Type != "image" || len(it.Srcset) == 0 || skipImage.MatchString(it.Title) {
				continue
			}
			caption := ""
			if it.Caption != nil {
				caption = it.Caption.Text
			}
			add(it.Srcset[0].Src, truncate(caption, 300))
		}
	}
	return nil
}

var (
	skipImage   = regexp.MustCompile(`(?i)\.(svg|gif)$|map|locator|logo|flag|location|icon|coat_of_arms|seal`)
	thumbRe     = regexp.MustCompile(`^https://(?:upload|thumb)\.wikimedia\.org/wikipedia/commons/thumb/([0-9a-f]/[0-9a-f]{2})/([^/]+)/\d+px-.+$`)
	originalRe  = regexp.MustCompile(`^https://upload\.wikimedia\.org/wikipedia/commons/([0-9a-f]/[0-9a-f]{2})/([^/]+)$`)
	thumbFileRe = regexp.MustCompile(`/commons/thumb/[0-9a-f]/[0-9a-f]{2}/([^/]+)/`)
)

// wikimediaThumb normalises any Commons URL to a 1280px thumbnail (good for galleries, small download).
func wikimediaThumb(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	raw = strings.SplitN(raw, "?", 2)[0]
	if m := thumbRe.FindStringSubmatch(raw); m != nil {
		return "https://thumb.wikimedia.org/wikipedia/commons/thumb/" + m[1] + "/" + m[2] + "/1280px-" + m[2]
	}
	if m := originalRe.FindStringSubmatch(raw); m != nil {
		return "https://thumb.wikimedia.org/wikipedia/commons/thumb/" + m[1] + "/" + m[2] + "/1280px-" + m[2]
	}
	return ""
}

func commonsPage(thumb string) string {
	if m := thumbFileRe.FindStringSubmatch(thumb); m != nil {
		return "https://commons.wikimedia.org/wiki/File:" + m[1]
	}
	return ""
}

// ------------------------------------------------------------------ YouTube

func (c *Client) youtube(ctx context.Context, name string, res *Result) error {
	q := url.Values{"part": {"snippet"}, "type": {"video"}, "maxResults": {"3"}, "regionCode": {"BD"},
		"q": {name + " Bangladesh travel"}, "key": {c.cfg.YouTubeAPIKey}}
	var out struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "https://www.googleapis.com/youtube/v3/search?"+q.Encode(), &out); err != nil {
		return err
	}
	for _, it := range out.Items {
		if it.ID.VideoID != "" {
			res.Videos = append(res.Videos, Video{URL: "https://www.youtube.com/watch?v=" + it.ID.VideoID, Title: it.Snippet.Title})
		}
	}
	if len(res.Videos) > 0 {
		res.Sources = append(res.Sources, "youtube")
	}
	return nil
}

// ------------------------------------------------------------------ Google Custom Search (images)

func (c *Client) googleImages(ctx context.Context, name string, res *Result) error {
	q := url.Values{"key": {c.cfg.GoogleSearchAPIKey}, "cx": {c.cfg.GoogleSearchCX}, "searchType": {"image"},
		"num": {"10"}, "safe": {"active"}, "imgSize": {"large"}, "q": {name + " Bangladesh"}}
	var out struct {
		Items []struct {
			Link  string `json:"link"`
			Title string `json:"title"`
			Image struct {
				ContextLink string `json:"contextLink"`
			} `json:"image"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "https://www.googleapis.com/customsearch/v1?"+q.Encode(), &out); err != nil {
		return err
	}
	n := 0
	for _, it := range out.Items {
		if !strings.HasPrefix(it.Link, "https://") || len(res.Images) >= 30 {
			continue
		}
		res.Images = append(res.Images, Image{URL: it.Link, Caption: truncate(it.Title, 300), SourceURL: it.Image.ContextLink})
		n++
	}
	if n > 0 {
		res.Sources = append(res.Sources, "google_images")
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
