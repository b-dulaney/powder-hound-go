// Package cams captures resort snow stake cams: it keeps public.resort_cams in step with the
// scraper configs, stores a new frame per cam each run (skipping unchanged ones) in the public
// resort-cams bucket, and deletes frames older than the retention period.
package cams

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // some cam sources may serve PNG
	"log"
	"time"

	"powderhoundgo/internal/supabase"

	"golang.org/x/image/draw"
)

// Store is the database and storage access cams need (supabase.CamStore in production).
type Store interface {
	ListConfigSlugs() ([]string, error)
	ReadConfig(slug string) ([]byte, error)
	ResortStatuses() (map[int]string, error)
	SyncCams(mountainID int, cams json.RawMessage) ([]supabase.Cam, error)
	LatestImageSHA(camID int64) (string, error)
	UploadFrame(path string, data []byte) error
	RemoveFrames(paths []string) error
	InsertImage(img supabase.CamImage) error
	ExpiredImages(cutoff time.Time, limit int) ([]supabase.CamImage, error)
	DeleteImages(ids []int64) error
	RecordCapture(camID int64, captureErr error) error
	RecordJobRun(job string, runErr error)
}

// CaptureRequest is one cam to capture, the payload of a cam:capture task.
type CaptureRequest struct {
	CamID  int64  `json:"cam_id"`
	Slug   string `json:"slug"`
	Key    string `json:"key"`
	Source string `json:"source"`
	URL    string `json:"url"`
}

const (
	thumbWidth   = 480 // 2x a ~240px filmstrip tile
	thumbQuality = 75
	fullQuality  = 85 // only for sources that aren't already JPEG
)

// Capturer stores one frame per call to Capture.
type Capturer struct {
	Store Store
	Fetch func(ctx context.Context, source, url string) (*Frame, error)
	Now   func() time.Time
}

// Capture fetches the cam's current frame and stores it with a thumbnail, unless it is
// byte-for-byte the frame stored last time. It returns whether a new frame was stored.
func (c *Capturer) Capture(ctx context.Context, req CaptureRequest) (bool, error) {
	frame, err := c.Fetch(ctx, req.Source, req.URL)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(frame.Data)
	hash := hex.EncodeToString(sum[:])

	latest, err := c.Store.LatestImageSHA(req.CamID)
	if err != nil {
		return false, err
	}
	if latest == hash {
		return false, nil
	}

	img, format, err := image.Decode(bytes.NewReader(frame.Data))
	if err != nil {
		return false, fmt.Errorf("decode frame from %s: %w", req.URL, err)
	}
	// Keep the source's JPEG as is (no recompression); convert anything else.
	full := frame.Data
	if format != "jpeg" {
		if full, err = encodeJPEG(img, fullQuality); err != nil {
			return false, err
		}
	}
	thumb, err := encodeJPEG(thumbnail(img, thumbWidth), thumbQuality)
	if err != nil {
		return false, err
	}

	now := c.Now().UTC()
	base := fmt.Sprintf("%s/%s/%s/%s-%s", req.Slug, req.Key, now.Format("2006-01-02"), now.Format("1504"), hash[:8])
	row := supabase.CamImage{
		CamID:           req.CamID,
		CapturedAt:      now,
		SourceUpdatedAt: frame.SourceUpdatedAt,
		Path:            base + ".jpg",
		ThumbPath:       base + "-thumb.jpg",
		Width:           img.Bounds().Dx(),
		Height:          img.Bounds().Dy(),
		SHA256:          hash,
	}

	if err := c.Store.UploadFrame(row.Path, full); err != nil {
		return false, err
	}
	err = c.Store.UploadFrame(row.ThumbPath, thumb)
	if err == nil {
		err = c.Store.InsertImage(row)
	}
	if err != nil {
		// Don't leave objects that no row points to; the retention sweep only sees rows.
		if rmErr := c.Store.RemoveFrames([]string{row.Path, row.ThumbPath}); rmErr != nil {
			log.Printf("[cams] couldn't remove frames after a failed save: %v", rmErr)
		}
		return false, err
	}
	return true, nil
}

// thumbnail scales img to width, keeping its aspect ratio. Smaller images are left as is.
func thumbnail(img image.Image, width int) image.Image {
	b := img.Bounds()
	if b.Dx() <= width {
		return img
	}
	height := max(1, b.Dy()*width/b.Dx())
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// Cleanup deletes frames captured before now-retention: Storage objects first, then rows, so
// a failure part way leaves rows that the next run retries (removing a missing object is fine).
func Cleanup(store Store, now time.Time, retention time.Duration) (int, error) {
	const batch = 200
	cutoff := now.Add(-retention)
	deleted := 0
	for {
		images, err := store.ExpiredImages(cutoff, batch)
		if err != nil {
			return deleted, err
		}
		if len(images) == 0 {
			return deleted, nil
		}
		paths := make([]string, 0, 2*len(images))
		ids := make([]int64, 0, len(images))
		for _, img := range images {
			paths = append(paths, img.Path, img.ThumbPath)
			ids = append(ids, img.ID)
		}
		if err := store.RemoveFrames(paths); err != nil {
			return deleted, err
		}
		if err := store.DeleteImages(ids); err != nil {
			return deleted, err
		}
		deleted += len(images)
		if len(images) < batch {
			return deleted, nil
		}
	}
}

// isPermanent reports errors that retrying the task won't fix.
func isPermanent(err error) bool {
	return errors.Is(err, errUnsupported)
}
