package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubOpenFilePRAndMarkers(t *testing.T) {
	var calls []string
	var putBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/dev":
			w.Write([]byte(`{"object":{"sha":"base-sha"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/refs":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/o/r/contents/"):
			content := base64.StdEncoding.EncodeToString([]byte(`{"id":5}`))
			w.Write([]byte(`{"content":"` + content[:4] + `\n` + content[4:] + `","encoding":"base64","sha":"file-sha"}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/repos/o/r/contents/"):
			json.NewDecoder(r.Body).Decode(&putBody)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/pulls":
			w.Write([]byte(`{"number":7,"html_url":"https://github.com/o/r/pull/7"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/7/labels":
			w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/issues":
			if r.URL.Query().Get("labels") != LabelResortStatus {
				t.Errorf("labels query = %q", r.URL.Query().Get("labels"))
			}
			w.Write([]byte(`[{"number":1,"body":"x <!-- m1 -->"},{"number":2,"body":"<!-- m1 -->","pull_request":{}},{"number":3,"body":"other"}]`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := &GitHub{http: srv.Client(), baseURL: srv.URL, token: "tok", repo: "o/r", Base: "dev"}

	content, sha, err := g.GetFile(context.Background(), "a.json", "dev")
	if err != nil || string(content) != `{"id":5}` || sha != "file-sha" {
		t.Fatalf("GetFile = %q %q %v", content, sha, err)
	}

	pr, err := g.OpenFilePR(context.Background(), "scraper-fix/a", "a.json", []byte("new"), "msg", "title", "body", []string{LabelScraperFix})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 7 {
		t.Errorf("pr = %+v", pr)
	}
	if putBody["sha"] != "file-sha" || putBody["branch"] != "scraper-fix/a" || putBody["content"] != base64.StdEncoding.EncodeToString([]byte("new")) {
		t.Errorf("unexpected contents PUT: %+v", putBody)
	}

	issues, err := g.OpenIssuesWithMarker(context.Background(), LabelResortStatus, "<!-- m1 -->")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 1 {
		t.Errorf("marker filter should skip PRs and non-matching issues: %+v", issues)
	}
}

func TestGitHubErrorsIncludeStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Reference already exists"}`))
	}))
	defer srv.Close()
	g := &GitHub{http: srv.Client(), baseURL: srv.URL, token: "tok", repo: "o/r", Base: "dev"}
	_, err := g.CreateIssue(context.Background(), "t", "b", nil)
	if err == nil || !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "Reference already exists") {
		t.Errorf("err = %v", err)
	}
}
