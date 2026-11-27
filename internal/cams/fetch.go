package cams

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	SourceImage   = "image"
	SourceYouTube = "youtube"

	// Frames are at most a few hundred KB; anything near this is not a webcam still.
	maxFrameBytes = 10 << 20
	userAgent     = "Mozilla/5.0 (compatible; PowderHound/1.0; +https://powderhound.io)"
)

// Frame is one fetched cam image.
type Frame struct {
	Data []byte
	// SourceUpdatedAt is the source's Last-Modified, when it sends one.
	SourceUpdatedAt *time.Time
}

// errUnsupported marks a config problem that retrying won't fix.
var errUnsupported = errors.New("unsupported cam source")

// Fetcher downloads frames over HTTP.
type Fetcher struct {
	client *http.Client
	// noRedirect reads redirects instead of following them (camstreamer embeds).
	noRedirect *http.Client
	// youtubeThumbBase is https://i.ytimg.com/vi in production; tests point it at a stub.
	youtubeThumbBase string
}

// NewFetcher returns a Fetcher that verifies TLS with AIA fetching (see tls.go).
func NewFetcher() *Fetcher {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = newAIAVerifier(nil).tlsConfig()
	return newFetcher(transport, "https://i.ytimg.com/vi")
}

func newFetcher(transport http.RoundTripper, youtubeThumbBase string) *Fetcher {
	return &Fetcher{
		client: &http.Client{Transport: transport, Timeout: 30 * time.Second},
		noRedirect: &http.Client{
			Transport:     transport,
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		youtubeThumbBase: strings.TrimRight(youtubeThumbBase, "/"),
	}
}

// Fetch returns the current frame for a cam.
func (f *Fetcher) Fetch(ctx context.Context, source, rawURL string) (*Frame, error) {
	switch source {
	case SourceImage:
		return f.getImage(ctx, rawURL)
	case SourceYouTube:
		id, err := f.youtubeVideoID(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		return f.youtubeThumbnail(ctx, id)
	default:
		return nil, fmt.Errorf("%w %q", errUnsupported, source)
	}
}

// errNotFound lets the YouTube fallback tell a missing thumbnail from other failures.
var errNotFound = errors.New("not found")

func (f *Fetcher) getImage(ctx context.Context, rawURL string) (*Frame, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/*")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("get %s: %w", rawURL, errNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %s", rawURL, resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		return nil, fmt.Errorf("get %s: content type %q is not an image", rawURL, ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFrameBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	if len(data) > maxFrameBytes {
		return nil, fmt.Errorf("get %s: larger than %d bytes", rawURL, maxFrameBytes)
	}
	frame := &Frame{Data: data}
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		frame.SourceUpdatedAt = &t
	}
	return frame, nil
}

var youtubeIDPattern = regexp.MustCompile(`(?:youtube(?:-nocookie)?\.com/(?:embed/|live/|watch\?v=|shorts/)|youtu\.be/)([A-Za-z0-9_-]{11})`)

// youtubeVideoID resolves a cam URL to the live video's id. A YouTube URL carries it directly.
// Anything else is treated as an embed page (camstreamer) that redirects to, or embeds, the
// current YouTube video; the id changes whenever the stream restarts, so it's resolved on
// every capture.
func (f *Fetcher) youtubeVideoID(ctx context.Context, rawURL string) (string, error) {
	if m := youtubeIDPattern.FindStringSubmatch(rawURL); m != nil {
		return m[1], nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "" {
		if m := youtubeIDPattern.FindStringSubmatch(loc); m != nil {
			return m[1], nil
		}
		return "", fmt.Errorf("resolve %s: redirect to %q is not a YouTube video", rawURL, loc)
	}
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if m := youtubeIDPattern.FindSubmatch(body); m != nil {
			return string(m[1]), nil
		}
	}
	return "", fmt.Errorf("resolve %s: no YouTube video found (status %s)", rawURL, resp.Status)
}

// youtubeThumbnail fetches a live video's thumbnail, which YouTube refreshes every few minutes
// while the stream is live. Not every stream has a maxres thumbnail.
func (f *Fetcher) youtubeThumbnail(ctx context.Context, id string) (*Frame, error) {
	var lastErr error
	for _, name := range []string{"maxresdefault_live.jpg", "hqdefault_live.jpg"} {
		frame, err := f.getImage(ctx, fmt.Sprintf("%s/%s/%s", f.youtubeThumbBase, url.PathEscape(id), name))
		if err == nil {
			return frame, nil
		}
		lastErr = err
		if !errors.Is(err, errNotFound) {
			break
		}
	}
	return nil, lastErr
}
