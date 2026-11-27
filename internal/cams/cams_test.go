package cams

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/tasks"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------------------------

type fakeStore struct {
	configs  map[string]string
	statuses map[int]string
	synced   map[int]string // mountain id -> cams JSON passed to SyncCams
	latest   map[int64]string
	uploads  map[string][]byte
	removed  []string
	inserted []supabase.CamImage
	expired  []supabase.CamImage
	deleted  []int64
	captures map[int64]error
	jobRuns  map[string]error

	insertErr error
	nextCamID int64
	camIDs    map[string]int64 // "<mountain>/<key>" -> id
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		configs: map[string]string{}, statuses: map[int]string{}, synced: map[int]string{},
		latest: map[int64]string{}, uploads: map[string][]byte{}, captures: map[int64]error{},
		jobRuns: map[string]error{}, camIDs: map[string]int64{},
	}
}

func (f *fakeStore) ListConfigSlugs() ([]string, error) {
	var slugs []string
	for s := range f.configs {
		slugs = append(slugs, s)
	}
	return slugs, nil
}

func (f *fakeStore) ReadConfig(slug string) ([]byte, error) {
	c, ok := f.configs[slug]
	if !ok {
		return nil, fmt.Errorf("no config %s", slug)
	}
	return []byte(c), nil
}

func (f *fakeStore) ResortStatuses() (map[int]string, error) { return f.statuses, nil }

// SyncCams mimics sync_resort_cams: every listed cam becomes an active row with a stable id.
func (f *fakeStore) SyncCams(mountainID int, cams json.RawMessage) ([]supabase.Cam, error) {
	f.synced[mountainID] = string(cams)
	var list []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(cams, &list); err != nil {
		return nil, err
	}
	var rows []supabase.Cam
	for _, c := range list {
		k := fmt.Sprintf("%d/%s", mountainID, c.Key)
		if _, ok := f.camIDs[k]; !ok {
			f.nextCamID++
			f.camIDs[k] = f.nextCamID
		}
		rows = append(rows, supabase.Cam{ID: f.camIDs[k], MountainID: mountainID, Key: c.Key, Active: true})
	}
	return rows, nil
}

func (f *fakeStore) LatestImageSHA(camID int64) (string, error) { return f.latest[camID], nil }

func (f *fakeStore) UploadFrame(path string, data []byte) error {
	if _, exists := f.uploads[path]; exists {
		return fmt.Errorf("%s already exists", path)
	}
	f.uploads[path] = data
	return nil
}

func (f *fakeStore) RemoveFrames(paths []string) error {
	f.removed = append(f.removed, paths...)
	for _, p := range paths {
		delete(f.uploads, p)
	}
	return nil
}

func (f *fakeStore) InsertImage(img supabase.CamImage) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.inserted = append(f.inserted, img)
	f.latest[img.CamID] = img.SHA256
	return nil
}

func (f *fakeStore) ExpiredImages(cutoff time.Time, limit int) ([]supabase.CamImage, error) {
	var out []supabase.CamImage
	for _, img := range f.expired {
		if img.CapturedAt.Before(cutoff) && len(out) < limit {
			out = append(out, img)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteImages(ids []int64) error {
	f.deleted = append(f.deleted, ids...)
	gone := map[int64]bool{}
	for _, id := range ids {
		gone[id] = true
	}
	var keep []supabase.CamImage
	for _, img := range f.expired {
		if !gone[img.ID] {
			keep = append(keep, img)
		}
	}
	f.expired = keep
	return nil
}

func (f *fakeStore) RecordCapture(camID int64, err error) error {
	f.captures[camID] = err
	return nil
}

func (f *fakeStore) RecordJobRun(job string, err error) { f.jobRuns[job] = err }

type fakeQueue struct {
	tasks    []*asynq.Task
	conflict map[string]bool // task ids that already exist
}

func (q *fakeQueue) Enqueue(t *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	for _, o := range opts {
		if o.Type() == asynq.TaskIDOpt && q.conflict[o.Value().(string)] {
			return nil, asynq.ErrTaskIDConflict
		}
	}
	q.tasks = append(q.tasks, t)
	return &asynq.TaskInfo{}, nil
}

func testJPEG(t *testing.T, w, h int, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{shade, uint8(x), uint8(y), 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}))
	return buf.Bytes()
}

func decodeSize(t *testing.T, data []byte) (int, int, string) {
	t.Helper()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	return cfg.Width, cfg.Height, format
}

// ---------------------------------------------------------------------------------------------
// Capture
// ---------------------------------------------------------------------------------------------

var captureTime = time.Date(2026, 11, 27, 18, 50, 0, 0, time.UTC)

func newCapturer(store Store, frame []byte, err error) *Capturer {
	modified := captureTime.Add(-3 * time.Minute)
	return &Capturer{
		Store: store,
		Fetch: func(context.Context, string, string) (*Frame, error) {
			if err != nil {
				return nil, err
			}
			return &Frame{Data: frame, SourceUpdatedAt: &modified}, nil
		},
		Now: func() time.Time { return captureTime },
	}
}

var vailReq = CaptureRequest{CamID: 7, Slug: "vail", Key: "snow-stake", Source: SourceImage, URL: "https://example.test/stake.jpg"}

func TestCaptureStoresOriginalAndThumbnail(t *testing.T) {
	store := newFakeStore()
	src := testJPEG(t, 1920, 1080, 10)

	stored, err := newCapturer(store, src, nil).Capture(context.Background(), vailReq)
	require.NoError(t, err)
	assert.True(t, stored)

	require.Len(t, store.inserted, 1)
	row := store.inserted[0]
	assert.Equal(t, int64(7), row.CamID)
	assert.Equal(t, captureTime, row.CapturedAt)
	assert.Equal(t, captureTime.Add(-3*time.Minute), *row.SourceUpdatedAt)
	assert.Equal(t, 1920, row.Width)
	assert.Equal(t, 1080, row.Height)
	assert.Regexp(t, `^vail/snow-stake/2026-11-27/1850-[0-9a-f]{8}\.jpg$`, row.Path)
	assert.Equal(t, strings.TrimSuffix(row.Path, ".jpg")+"-thumb.jpg", row.ThumbPath)
	assert.Equal(t, row.SHA256[:8], row.Path[len("vail/snow-stake/2026-11-27/1850-"):][:8])

	assert.Equal(t, src, store.uploads[row.Path], "a JPEG source is stored as is")
	w, h, format := decodeSize(t, store.uploads[row.ThumbPath])
	assert.Equal(t, []any{480, 270, "jpeg"}, []any{w, h, format})
}

func TestCaptureSkipsUnchangedFrame(t *testing.T) {
	store := newFakeStore()
	src := testJPEG(t, 640, 360, 10)
	c := newCapturer(store, src, nil)

	_, err := c.Capture(context.Background(), vailReq)
	require.NoError(t, err)
	stored, err := c.Capture(context.Background(), vailReq)
	require.NoError(t, err)

	assert.False(t, stored)
	assert.Len(t, store.inserted, 1)
	assert.Len(t, store.uploads, 2)
}

func TestCaptureConvertsNonJPEGAndKeepsSmallThumbnails(t *testing.T) {
	store := newFakeStore()
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	_, err := newCapturer(store, buf.Bytes(), nil).Capture(context.Background(), vailReq)
	require.NoError(t, err)

	row := store.inserted[0]
	_, _, format := decodeSize(t, store.uploads[row.Path])
	assert.Equal(t, "jpeg", format)
	w, h, _ := decodeSize(t, store.uploads[row.ThumbPath])
	assert.Equal(t, []int{400, 300}, []int{w, h}, "images narrower than the thumbnail aren't upscaled")
}

func TestCaptureRemovesObjectsWhenTheRowFails(t *testing.T) {
	store := newFakeStore()
	store.insertErr = errors.New("db down")

	stored, err := newCapturer(store, testJPEG(t, 640, 360, 10), nil).Capture(context.Background(), vailReq)

	require.ErrorContains(t, err, "db down")
	assert.False(t, stored)
	assert.Empty(t, store.uploads, "no orphaned objects")
	assert.Len(t, store.removed, 2)
}

func TestCaptureRejectsNonImages(t *testing.T) {
	store := newFakeStore()
	_, err := newCapturer(store, []byte("<html>offline</html>"), nil).Capture(context.Background(), vailReq)
	require.ErrorContains(t, err, "decode frame")
	assert.Empty(t, store.uploads)
}

func TestUnsupportedSourceIsPermanent(t *testing.T) {
	_, err := NewFetcher().Fetch(context.Background(), "hls", "https://example.test/a.m3u8")
	assert.True(t, isPermanent(err))
}

// ---------------------------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------------------------

func TestCleanupDeletesObjectsThenRowsInBatches(t *testing.T) {
	store := newFakeStore()
	now := captureTime
	for i := 0; i < 450; i++ {
		store.expired = append(store.expired, supabase.CamImage{
			ID: int64(i + 1), CapturedAt: now.Add(-8 * 24 * time.Hour),
			Path: fmt.Sprintf("a/%d.jpg", i), ThumbPath: fmt.Sprintf("a/%d-thumb.jpg", i),
		})
	}
	recent := supabase.CamImage{ID: 1000, CapturedAt: now.Add(-6 * 24 * time.Hour), Path: "a/new.jpg", ThumbPath: "a/new-thumb.jpg"}
	store.expired = append(store.expired, recent)

	n, err := Cleanup(store, now, Retention)

	require.NoError(t, err)
	assert.Equal(t, 450, n)
	assert.Len(t, store.removed, 900)
	assert.Len(t, store.deleted, 450)
	assert.Equal(t, []supabase.CamImage{recent}, store.expired, "frames inside the retention period stay")
}

// ---------------------------------------------------------------------------------------------
// Queue
// ---------------------------------------------------------------------------------------------

func TestQueueCaptures(t *testing.T) {
	store := newFakeStore()
	store.configs["steamboat"] = `{"id": 1, "cams": [
		{"key": "mid-mountain", "kind": "snow-stake", "label": "Mid", "source": "youtube", "url": "https://camstreamer.com/embed/a", "pageURL": "https://x"},
		{"key": "champagne-powder", "kind": "snow-stake", "label": "Champ", "source": "youtube", "url": "https://camstreamer.com/embed/b", "pageURL": "https://x"}]}`
	store.configs["vail"] = `{"id": 13, "cams": [{"key": "snow-stake", "kind": "snow-stake", "label": "Summit", "source": "image", "url": "https://live9.brownrice.com/cam-images/vailsnowsummit.jpg", "pageURL": "https://x"}]}`
	store.configs["sunlight-mountain"] = `{"id": 7, "cams": [{"key": "snow-stake", "kind": "snow-stake", "label": "S", "source": "image", "url": "https://s.test/a.jpg", "pageURL": "https://x"}]}`
	store.configs["no-cams"] = `{"id": 30}`
	store.configs["broken"] = `{"id": `
	store.statuses = map[int]string{1: "open", 13: "unknown", 7: "closed"}
	q := &fakeQueue{}

	err := QueueCaptures(store, q, captureTime)

	require.ErrorContains(t, err, "broken: parse config")
	assert.Equal(t, "[]", store.synced[30], "a config without cams deactivates the resort's cams")
	assert.Contains(t, store.synced, 7, "closed resorts are still synced")

	var got []CaptureRequest
	for _, task := range q.tasks {
		assert.Equal(t, tasks.TypeCamCapture, task.Type())
		var r CaptureRequest
		require.NoError(t, json.Unmarshal(task.Payload(), &r))
		got = append(got, r)
	}
	assert.ElementsMatch(t, []CaptureRequest{
		{CamID: store.camIDs["1/mid-mountain"], Slug: "steamboat", Key: "mid-mountain", Source: "youtube", URL: "https://camstreamer.com/embed/a"},
		{CamID: store.camIDs["1/champagne-powder"], Slug: "steamboat", Key: "champagne-powder", Source: "youtube", URL: "https://camstreamer.com/embed/b"},
		{CamID: store.camIDs["13/snow-stake"], Slug: "vail", Key: "snow-stake", Source: "image", URL: "https://live9.brownrice.com/cam-images/vailsnowsummit.jpg"},
	}, got, "closed resorts aren't captured")
}

func TestQueueCapturesOncePerCamPerHour(t *testing.T) {
	store := newFakeStore()
	store.configs["vail"] = `{"id": 13, "cams": [{"key": "snow-stake", "source": "image", "url": "https://x/a.jpg"}]}`
	q := &fakeQueue{conflict: map[string]bool{"cam-capture:1:2026112718": true}}

	require.NoError(t, QueueCaptures(store, q, captureTime))
	assert.Empty(t, q.tasks, "an existing task for the hour is not an error")
}

// ---------------------------------------------------------------------------------------------
// Fetch
// ---------------------------------------------------------------------------------------------

func TestFetchImage(t *testing.T) {
	frame := testJPEG(t, 64, 36, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stake.jpg":
			assert.Contains(t, r.Header.Get("User-Agent"), "PowderHound")
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Last-Modified", "Fri, 27 Nov 2026 18:20:14 GMT")
			w.Write(frame)
		case "/offline":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := newFetcher(http.DefaultTransport, "")

	got, err := f.Fetch(context.Background(), SourceImage, srv.URL+"/stake.jpg")
	require.NoError(t, err)
	assert.Equal(t, frame, got.Data)
	assert.Equal(t, time.Date(2026, 11, 27, 18, 20, 14, 0, time.UTC), got.SourceUpdatedAt.UTC())

	_, err = f.Fetch(context.Background(), SourceImage, srv.URL+"/offline")
	assert.ErrorContains(t, err, "not an image")
	_, err = f.Fetch(context.Background(), SourceImage, srv.URL+"/missing")
	assert.ErrorIs(t, err, errNotFound)
}

func TestFetchYouTubeResolvesEmbedRedirectAndFallsBack(t *testing.T) {
	frame := testJPEG(t, 64, 36, 2)
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch r.URL.Path {
		case "/embed/abc":
			http.Redirect(w, r, "https://www.youtube.com/embed/yj-5Q1vBMaA?autoplay=1&mute=1", http.StatusFound)
		case "/vi/yj-5Q1vBMaA/hqdefault_live.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(frame)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := newFetcher(http.DefaultTransport, srv.URL+"/vi")

	got, err := f.Fetch(context.Background(), SourceYouTube, srv.URL+"/embed/abc")

	require.NoError(t, err)
	assert.Equal(t, frame, got.Data)
	assert.Equal(t, []string{"/embed/abc", "/vi/yj-5Q1vBMaA/maxresdefault_live.jpg", "/vi/yj-5Q1vBMaA/hqdefault_live.jpg"}, requested)
}

func TestYouTubeVideoIDFromURL(t *testing.T) {
	f := newFetcher(http.DefaultTransport, "")
	for _, u := range []string{
		"https://www.youtube.com/watch?v=lKc9xwndUK4",
		"https://youtu.be/lKc9xwndUK4",
		"https://www.youtube.com/live/lKc9xwndUK4?si=x",
		"https://www.youtube-nocookie.com/embed/lKc9xwndUK4",
	} {
		id, err := f.youtubeVideoID(context.Background(), u)
		require.NoError(t, err, u)
		assert.Equal(t, "lKc9xwndUK4", id, u)
	}
}

// ---------------------------------------------------------------------------------------------
// TLS with AIA fetching
// ---------------------------------------------------------------------------------------------

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

var serial atomic.Int64

func issue(t *testing.T, tmpl *x509.Certificate, parent *testCA) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl.SerialNumber = big.NewInt(serial.Add(1))
	tmpl.NotBefore = time.Now().Add(-time.Hour)
	tmpl.NotAfter = time.Now().Add(time.Hour)
	signer, parentCert := key, tmpl
	if parent != nil {
		signer, parentCert = parent.key, parent.cert
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parentCert, &key.PublicKey, signer)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &testCA{cert, key}
}

func newCA(t *testing.T, name string, parent *testCA) *testCA {
	return issue(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: name}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}, parent)
}

// leafOnlyServer serves an image over TLS presenting only a leaf for cam.test (no
// intermediate), like Brownrice's streamer hosts. The leaf's AIA points at aiaURL.
func leafOnlyServer(t *testing.T, issuer *testCA, aiaURL string) *httptest.Server {
	leaf := issue(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "cam.test"}, DNSNames: []string{"cam.test"},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IssuingCertificateURL: []string{aiaURL},
	}, issuer)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpeg"))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leaf.cert.Raw}, PrivateKey: leaf.key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// clientFor returns a fetcher trusting root whose connections to any host reach srv.
func clientFor(root *testCA, srv *httptest.Server) (*Fetcher, *aiaVerifier) {
	pool := x509.NewCertPool()
	pool.AddCert(root.cert)
	v := newAIAVerifier(pool)
	transport := &http.Transport{
		TLSClientConfig: v.tlsConfig(),
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
	}
	return newFetcher(transport, ""), v
}

func TestAIAFetchesMissingIntermediate(t *testing.T) {
	root := newCA(t, "Test Root", nil)
	inter := newCA(t, "Test Intermediate", root)
	var aiaHits atomic.Int32
	aia := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiaHits.Add(1)
		w.Write(inter.cert.Raw)
	}))
	defer aia.Close()
	srv := leafOnlyServer(t, inter, aia.URL+"/inter.crt")
	f, _ := clientFor(root, srv)

	for i := 0; i < 2; i++ {
		_, err := f.Fetch(context.Background(), SourceImage, "https://cam.test/stake.jpg")
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), aiaHits.Load(), "the intermediate is cached")

	_, err := f.Fetch(context.Background(), SourceImage, "https://other.test/stake.jpg")
	assert.ErrorContains(t, err, "certificate is valid for cam.test", "the hostname is still checked")

	_, err = f.Fetch(context.Background(), SourceImage, "https://127.0.0.1/stake.jpg")
	assert.ErrorContains(t, err, "no server name")
}

func TestAIAFailsWithoutTheIntermediate(t *testing.T) {
	root := newCA(t, "Test Root", nil)
	inter := newCA(t, "Test Intermediate", root)
	aia := httptest.NewServer(http.NotFoundHandler())
	defer aia.Close()
	f, _ := clientFor(root, leafOnlyServer(t, inter, aia.URL+"/inter.crt"))

	_, err := f.Fetch(context.Background(), SourceImage, "https://cam.test/stake.jpg")
	assert.ErrorContains(t, err, "unknown authority")
}

func TestAIADoesNotTrustAnUntrustedChain(t *testing.T) {
	trusted := newCA(t, "Trusted Root", nil)
	rogueRoot := newCA(t, "Rogue Root", nil)
	rogueInter := newCA(t, "Rogue Intermediate", rogueRoot)
	aia := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(rogueInter.cert.Raw)
	}))
	defer aia.Close()
	f, _ := clientFor(trusted, leafOnlyServer(t, rogueInter, aia.URL+"/inter.crt"))

	_, err := f.Fetch(context.Background(), SourceImage, "https://cam.test/stake.jpg")
	assert.ErrorContains(t, err, "unknown authority")
}
