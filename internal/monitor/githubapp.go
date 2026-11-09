package monitor

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// appAuth signs in as a GitHub App installation, so issues and PRs the monitor opens come from
// the app's bot account. GitHub doesn't notify you about your own activity; a bot's issues reach
// the repo owner as normal notifications, and the owner can review the bot's PRs.
type appAuth struct {
	appID string
	key   *rsa.PrivateKey
	// Installation tokens last an hour; one is reused until it's within refreshBefore of expiry.
	mu        sync.Mutex
	token     string
	expiresAt time.Time
	now       func() time.Time
}

const refreshBefore = 5 * time.Minute

// parseAppKey accepts the app's private key as PEM, or as base64 of the PEM (a single line,
// which fits in a .env file).
func parseAppKey(s string) (*rsa.PrivateKey, error) {
	s = strings.TrimSpace(s)
	data := []byte(s)
	if !strings.HasPrefix(s, "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, errors.New("GitHub App private key is neither PEM nor base64 PEM")
		}
		data = decoded
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("GitHub App private key has no PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key isn't RSA")
	}
	return key, nil
}

// jwt is the short-lived app token used only to mint installation tokens (RS256).
func (a *appAuth) jwt() (string, error) {
	now := a.now()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(), // allow for clock drift
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": a.appID,
	})
	if err != nil {
		return "", err
	}
	signing := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

// installationToken returns a token for the app's installation on repo, minting one when needed.
func (a *appAuth) installationToken(ctx context.Context, client *http.Client, baseURL, repo string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && a.now().Add(refreshBefore).Before(a.expiresAt) {
		return a.token, nil
	}
	jwt, err := a.jwt()
	if err != nil {
		return "", err
	}
	var inst struct {
		ID int64 `json:"id"`
	}
	if err := appRequest(ctx, client, http.MethodGet, baseURL+"/repos/"+repo+"/installation", jwt, &inst); err != nil {
		return "", fmt.Errorf("find GitHub App installation on %s: %w", repo, err)
	}
	var tok struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := baseURL + "/app/installations/" + strconv.FormatInt(inst.ID, 10) + "/access_tokens"
	if err := appRequest(ctx, client, http.MethodPost, path, jwt, &tok); err != nil {
		return "", fmt.Errorf("create GitHub App installation token: %w", err)
	}
	if tok.Token == "" {
		return "", errors.New("create GitHub App installation token: empty token")
	}
	a.token, a.expiresAt = tok.Token, tok.ExpiresAt
	return a.token, nil
}

func appRequest(ctx context.Context, client *http.Client, method, url, jwt string, out any) error {
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewReader([]byte("{}"))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%d %s", resp.StatusCode, truncateBody(string(data)))
	}
	return json.Unmarshal(data, out)
}
