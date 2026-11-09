package monitor

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGitHubAppAuth(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	var minted int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case r.URL.Path == "/repos/o/r/installation" || r.URL.Path == "/app/installations/42/access_tokens":
			checkJWT(t, auth, &key.PublicKey, now)
			if r.URL.Path == "/repos/o/r/installation" {
				w.Write([]byte(`{"id":42}`))
				return
			}
			if r.Method != http.MethodPost {
				t.Errorf("access_tokens method = %s", r.Method)
			}
			minted++
			w.Write([]byte(`{"token":"inst-tok","expires_at":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}`))
		case r.URL.Path == "/repos/o/r/issues":
			if auth != "inst-tok" {
				t.Errorf("issues auth = %q", auth)
			}
			w.Write([]byte(`{"number":1}`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	// Both a PEM key and base64 of it (as the deploy writes it to .env) parse.
	for _, in := range []string{pemKey, base64.StdEncoding.EncodeToString([]byte(pemKey))} {
		if _, err := parseAppKey(in); err != nil {
			t.Fatalf("parseAppKey: %v", err)
		}
	}
	if _, err := parseAppKey("nope"); err == nil {
		t.Error("parseAppKey accepted garbage")
	}

	clock := now
	g := &GitHub{http: srv.Client(), baseURL: srv.URL, repo: "o/r",
		app: &appAuth{appID: "123", key: key, now: func() time.Time { return clock }}}
	for i := 0; i < 2; i++ {
		if _, err := g.CreateIssue(context.Background(), "t", "b", nil); err != nil {
			t.Fatal(err)
		}
	}
	if minted != 1 {
		t.Errorf("minted %d tokens for two calls, want 1", minted)
	}
	// Close to expiry, a fresh token is minted.
	clock = now.Add(56 * time.Minute)
	now = clock
	if _, err := g.CreateIssue(context.Background(), "t", "b", nil); err != nil {
		t.Fatal(err)
	}
	if minted != 2 {
		t.Errorf("minted = %d after nearing expiry, want 2", minted)
	}
}

func checkJWT(t *testing.T, token string, pub *rsa.PublicKey, now time.Time) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("jwt signature: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iat, Exp int64
		Iss      string
	}
	json.Unmarshal(raw, &c)
	if c.Iss != "123" || c.Iat > now.Unix() || c.Exp <= now.Unix() || c.Exp-c.Iat > 600 {
		t.Errorf("jwt claims = %+v", c)
	}
}
