package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"powderhoundgo/internal/scraping"
	"powderhoundgo/internal/supabase"
)

func ptr(i int) *int { return &i }

var (
	nov  = time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC) // Wednesday, opening window
	jul  = time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)  // Wednesday, off season
	julM = time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC)  // Monday, off season
)

func TestDecide(t *testing.T) {
	scrape := func(lifts int) *ScrapeResult { return &ScrapeResult{LiftsOpen: lifts, BaseDepth: 18} }
	cases := []struct {
		name string
		o    Observation
		now  time.Time
		want Decision
	}{
		{
			name: "lifts spinning in two scrapes opens a closed resort",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "auto", LiftsOpen: ptr(1)}, Scrape: scrape(2)},
			now:  nov, want: Decision{AutoOpen: true},
		},
		{
			name: "one reading with lifts waits for confirmation",
			o:    Observation{State: supabase.ResortState{Status: "unknown", StatusSource: "auto", LiftsOpen: ptr(0)}, Scrape: scrape(2)},
			now:  nov, want: Decision{},
		},
		{
			name: "already open with lifts does nothing",
			o:    Observation{State: supabase.ResortState{Status: "open", StatusSource: "auto", LiftsOpen: ptr(3)}, Scrape: scrape(3)},
			now:  nov, want: Decision{},
		},
		{
			name: "manual status is never changed automatically",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "manual", LiftsOpen: ptr(3)}, Scrape: scrape(3)},
			now:  nov, want: Decision{},
		},
		{
			name: "open resort with no lifts gets a status read",
			o:    Observation{State: supabase.ResortState{Status: "open", StatusSource: "auto", LiftsOpen: ptr(0)}, Scrape: scrape(0)},
			now:  nov, want: Decision{ReadStatus: true},
		},
		{
			name: "closed resort is read daily in the opening window",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "auto"}, Scrape: scrape(0)},
			now:  nov, want: Decision{ReadStatus: true},
		},
		{
			name: "closed resort is not read midweek off season",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "auto"}, Scrape: scrape(0)},
			now:  jul, want: Decision{},
		},
		{
			name: "closed resort is read on Mondays off season",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "auto"}, Scrape: scrape(0)},
			now:  julM, want: Decision{ReadStatus: true},
		},
		{
			name: "failed scrape on a non-closed resort asks for a fix and a status read",
			o:    Observation{State: supabase.ResortState{Status: "unknown", StatusSource: "auto"}, ScrapeErr: errors.New("no value found for base depth")},
			now:  nov, want: Decision{FixConfig: true, ReadStatus: true, Reason: "scrape failed: no value found for base depth"},
		},
		{
			name: "failed scrape on a closed resort is expected",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "auto"}, ScrapeErr: errors.New("timeout")},
			now:  jul, want: Decision{},
		},
		{
			name: "more lifts open than exist is a broken scrape",
			o:    Observation{State: supabase.ResortState{Status: "open", StatusSource: "auto", LiftsOpen: ptr(5), TotalLifts: ptr(9)}, Scrape: scrape(147)},
			now:  nov, want: Decision{FixConfig: true, Reason: "scraped 147 lifts open but the resort has 9"},
		},
		{
			name: "open trails with lifts at 0 after hours counts as operating",
			o:    Observation{State: supabase.ResortState{Status: "unknown", StatusSource: "auto", RunsOpen: ptr(12), LiftsOpen: ptr(0)}, Scrape: &ScrapeResult{RunsOpen: 12, LiftsOpen: 0}},
			now:  nov, want: Decision{AutoOpen: true},
		},
		{
			name: "open resort with trails but no lifts needs no status read",
			o:    Observation{State: supabase.ResortState{Status: "open", StatusSource: "auto", RunsOpen: ptr(30), LiftsOpen: ptr(0)}, Scrape: &ScrapeResult{RunsOpen: 30, LiftsOpen: 0}},
			now:  nov, want: Decision{},
		},
		{
			name: "more runs open than exist is a broken scrape and not evidence",
			o:    Observation{State: supabase.ResortState{Status: "closed", StatusSource: "auto", RunsOpen: ptr(3), TotalRuns: ptr(147)}, Scrape: &ScrapeResult{RunsOpen: 9000}},
			now:  nov, want: Decision{},
		},
		{
			name: "impossible runs on a non-closed resort asks for a fix only",
			o:    Observation{State: supabase.ResortState{Status: "unknown", StatusSource: "auto", RunsOpen: ptr(3), TotalRuns: ptr(147)}, Scrape: &ScrapeResult{RunsOpen: 9000}},
			now:  nov, want: Decision{FixConfig: true, Reason: "scraped 9000 runs open but the resort has 147"},
		},
		{
			name: "manual status still gets config fixes",
			o:    Observation{State: supabase.ResortState{Status: "open", StatusSource: "manual"}, ScrapeErr: errors.New("boom")},
			now:  nov, want: Decision{FixConfig: true, Reason: "scrape failed: boom"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decide(tc.o, tc.now); got != tc.want {
				t.Errorf("decide() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCompareScrape(t *testing.T) {
	cfg := supabase.ScrapingConfig{}
	expected := ExpectedValues{BaseDepth: 18, Snow24h: 1, Snow48h: 0, SeasonTotal: 7, RunsOpen: 1, LiftsOpen: -1}
	checks := compareScrape(expected, ScrapeResult{BaseDepth: 18, Snow24h: 1, Snow48h: 0, SeasonTotal: 99, RunsOpen: 1, LiftsOpen: 5}, cfg)
	if len(checks) != 4 {
		t.Fatalf("expected 4 checks (season total has no selector, lifts not shown), got %d: %+v", len(checks), checks)
	}
	if !allOK(checks) {
		t.Errorf("expected all OK, got %+v", checks)
	}

	cfg.Conditions.SeasonTotalSelector = ".total"
	checks = compareScrape(expected, ScrapeResult{BaseDepth: 18, Snow24h: 1, Snow48h: 0, SeasonTotal: 99, RunsOpen: 1}, cfg)
	if allOK(checks) {
		t.Error("season total mismatch should fail once its selector is set")
	}
	if !strings.Contains(describeMismatches(checks), "season_total: page shows 7, scrape returned 99") {
		t.Errorf("unexpected mismatch text: %s", describeMismatches(checks))
	}
	if allOK(nil) {
		t.Error("no checks must not count as verified")
	}
}

func TestMergeConfigJSONKeepsUnknownKeysAndNulls(t *testing.T) {
	original := []byte(`{
  "id": 5,
  "name": "A-Basin",
  "closingDate": null,
  "terrainURL": null,
  "conditions": {"conditionsSelector": ".old", "baseDepthSelector": ".old-base"},
  "terrain": {"terrainSelector": ".t", "liftDetailSelector": ""}
}`)
	proposed := supabase.ScrapingConfig{ID: 5, Name: "A-Basin", ConditionsURL: "https://example.com"}
	proposed.Conditions.ConditionsSelector = "#weather"
	proposed.Conditions.BaseDepthSelector = `[class*="Base"]`
	proposed.Terrain.TerrainSelector = ".t2"

	out, err := mergeConfigJSON(original, proposed)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`"closingDate": null`,
		`"terrainURL": null`,
		`"liftDetailSelector": ""`,
		`"conditionsSelector": "#weather"`,
		`"baseDepthSelector": "[class*=\"Base\"]"`,
		`"terrainSelector": ".t2"`,
		`"conditionsURL": "https://example.com"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("merged config missing %s:\n%s", want, s)
		}
	}
	for _, unwanted := range []string{`"clickSelector"`, `"waitForSelector"`, `"countLifts"`} {
		if strings.Contains(s, unwanted) {
			t.Errorf("merged config added zero-valued key %s:\n%s", unwanted, s)
		}
	}
}

func TestProposalCommentCannotBeClosedByPageText(t *testing.T) {
	c, err := proposalComment(StatusProposal{Env: "prod", MountainID: 5, Status: "closed", Evidence: "Closed --> <script>"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(c, "-->") != 1 || !strings.HasSuffix(c, " -->") {
		t.Errorf("evidence escaped the comment: %s", c)
	}
}

// --- fakes for flow tests ---

type fakeStore struct {
	configs  map[string]supabase.ScrapingConfig
	states   map[int]supabase.ResortState
	updates  []supabase.StatusUpdate
	uploaded []string
}

func (f *fakeStore) ListConfigSlugs() ([]string, error) {
	var s []string
	for k := range f.configs {
		s = append(s, k)
	}
	return s, nil
}
func (f *fakeStore) GetConfig(slug string) (supabase.ScrapingConfig, error) {
	return f.configs[slug], nil
}
func (f *fakeStore) GetResortStates() (map[int]supabase.ResortState, error) { return f.states, nil }
func (f *fakeStore) SetResortStatus(u supabase.StatusUpdate) (bool, error) {
	f.updates = append(f.updates, u)
	return true, nil
}
func (f *fakeStore) UploadEvidence(path string, _ []byte, _ string) (string, error) {
	f.uploaded = append(f.uploaded, path)
	return "https://evidence.example/" + path, nil
}

type fakeClaude struct {
	proposals []*ConfigProposal
	reading   *StatusReading
	calls     int
}

func (f *fakeClaude) ProposeConfig(_ context.Context, _ supabase.ScrapingConfig, _ *scraping.PageCapture, _ string, _ *ConfigProposal, _ string) (*ConfigProposal, error) {
	p := f.proposals[f.calls]
	f.calls++
	return p, nil
}
func (f *fakeClaude) ReadStatus(context.Context, string, *scraping.PageCapture, string) (*StatusReading, error) {
	f.calls++
	return f.reading, nil
}

type fakeRepo struct {
	openIssues []Issue
	openPRs    []PullRequest
	created    []Issue
	prs        []struct {
		Branch, Path, Body string
		Content            []byte
	}
}

func (f *fakeRepo) GetFile(context.Context, string, string) ([]byte, string, error) {
	return []byte(`{"id":5,"name":"A-Basin","closingDate":null,"conditions":{},"terrain":{}}`), "sha", nil
}
func (f *fakeRepo) OpenFilePR(_ context.Context, branch, path string, content []byte, _, _, body string, _ []string) (*PullRequest, error) {
	f.prs = append(f.prs, struct {
		Branch, Path, Body string
		Content            []byte
	}{branch, path, body, content})
	return &PullRequest{Number: 1, HTMLURL: "https://github.com/x/y/pull/1"}, nil
}
func (f *fakeRepo) OpenPRsWithMarker(_ context.Context, marker string) ([]PullRequest, error) {
	var out []PullRequest
	for _, pr := range f.openPRs {
		if strings.Contains(pr.Body, marker) {
			out = append(out, pr)
		}
	}
	return out, nil
}
func (f *fakeRepo) OpenIssuesWithMarker(_ context.Context, _ string, marker string) ([]Issue, error) {
	var out []Issue
	for _, i := range f.openIssues {
		if strings.Contains(i.Body, marker) {
			out = append(out, i)
		}
	}
	return out, nil
}
func (f *fakeRepo) CreateIssue(_ context.Context, title, body string, _ []string) (*Issue, error) {
	i := Issue{Number: len(f.created) + 1, Title: title, Body: body}
	f.created = append(f.created, i)
	return &i, nil
}
func (f *fakeRepo) BaseBranch() string { return "dev" }

func newTestMonitor(store *fakeStore, claude *fakeClaude, repo *fakeRepo, scrape func(supabase.ScrapingConfig) (map[string]interface{}, error)) *Monitor {
	return &Monitor{
		Store: store, Claude: claude, Repo: repo, Env: "dev",
		Scrape: scrape,
		Capture: func(url, _ string) (*scraping.PageCapture, error) {
			return &scraping.PageCapture{URL: url, Screenshot: []byte("png")}, nil
		},
		Proof: func(supabase.ScrapingConfig) (*scraping.ProofCapture, error) {
			return &scraping.ProofCapture{Screenshot: []byte("jpg")}, nil
		},
		Now:           func() time.Time { return nov },
		MaxModelCalls: 10,
	}
}

func abasinStore(status string, lifts int) *fakeStore {
	return &fakeStore{
		configs: map[string]supabase.ScrapingConfig{"a-basin": {ID: 5, Name: "A-Basin", ConditionsURL: "https://abasin.example"}},
		states:  map[int]supabase.ResortState{5: {MountainID: 5, Status: status, StatusSource: "auto", LiftsOpen: ptr(lifts), TotalLifts: ptr(9)}},
	}
}

func TestRunAutoOpensFromLiftData(t *testing.T) {
	store := abasinStore("closed", 1)
	claude := &fakeClaude{}
	m := newTestMonitor(store, claude, &fakeRepo{}, func(supabase.ScrapingConfig) (map[string]interface{}, error) {
		return map[string]interface{}{"base_depth": 18, "lifts_open": 2, "runs_open": 1}, nil
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.updates) != 1 || store.updates[0].Status != "open" || store.updates[0].Source != "auto" {
		t.Fatalf("expected one auto open update, got %+v", store.updates)
	}
	if claude.calls != 0 {
		t.Errorf("lift proof should not need Claude, got %d calls", claude.calls)
	}
}

func TestRunProposesStatusChangeAsIssue(t *testing.T) {
	store := abasinStore("open", 0)
	repo := &fakeRepo{}
	claude := &fakeClaude{reading: &StatusReading{Status: "closed", Confidence: "high", Quote: "Closed for the 2026/27 season", Reasoning: "banner"}}
	m := newTestMonitor(store, claude, repo, func(supabase.ScrapingConfig) (map[string]interface{}, error) {
		return map[string]interface{}{"base_depth": 0, "lifts_open": 0}, nil
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.updates) != 0 {
		t.Errorf("a Claude reading must not change status without approval, got %+v", store.updates)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one status issue, got %d", len(repo.created))
	}
	body := repo.created[0].Body
	for _, want := range []string{"Closed for the 2026/27 season", statusMarker("dev", 5), `"status":"closed"`, "/approve"} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing %q:\n%s", want, body)
		}
	}
	if !strings.HasPrefix(repo.created[0].Title, "[dev] ") {
		t.Errorf("dev issues should be tagged, got %q", repo.created[0].Title)
	}

	// A second run with the issue still open must not open another or call Claude again.
	repo.openIssues = repo.created
	calls := claude.calls
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.created) != 1 || claude.calls != calls {
		t.Errorf("duplicate status issue or Claude call: issues=%d calls=%d", len(repo.created), claude.calls)
	}
}

func TestRunOpensVerifiedFixPRAfterRetry(t *testing.T) {
	store := abasinStore("unknown", 0)
	repo := &fakeRepo{}
	bad := &ConfigProposal{Config: supabase.ScrapingConfig{ID: 5, Name: "A-Basin", ConditionsURL: "https://abasin.example"}, Expected: ExpectedValues{BaseDepth: 18, Snow24h: 1, Snow48h: 0, SeasonTotal: -1, RunsOpen: 1, LiftsOpen: 1}}
	bad.Config.Conditions.BaseDepthSelector = ".wrong"
	good := &ConfigProposal{Config: bad.Config, Expected: bad.Expected, Summary: "New widget", Evidence: []FieldEvidence{{Field: "base_depth", Quote: `18" Mid-Mountain Base`}}}
	good.Config.Conditions.BaseDepthSelector = ".right"
	claude := &fakeClaude{proposals: []*ConfigProposal{bad, good}, reading: &StatusReading{Status: "unknown"}}

	m := newTestMonitor(store, claude, repo, func(cfg supabase.ScrapingConfig) (map[string]interface{}, error) {
		switch cfg.Conditions.BaseDepthSelector {
		case ".right":
			return map[string]interface{}{"base_depth": 18, "snow_past_24h": 1, "snow_past_48h": 0, "runs_open": 1, "lifts_open": 1}, nil
		case ".wrong":
			return map[string]interface{}{"base_depth": 0, "snow_past_24h": 1, "snow_past_48h": 0, "runs_open": 1, "lifts_open": 1}, nil
		}
		return nil, errors.New("no value found for base depth")
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.prs) != 1 {
		t.Fatalf("expected one fix PR, got %d (issues: %+v)", len(repo.prs), repo.created)
	}
	pr := repo.prs[0]
	if pr.Path != "supabase/storage/scraping-config/a-basin.json" || !strings.HasPrefix(pr.Branch, "scraper-fix/a-basin-") {
		t.Errorf("unexpected PR target %s on %s", pr.Path, pr.Branch)
	}
	if !strings.Contains(string(pr.Content), `".right"`) || !strings.Contains(string(pr.Content), `"closingDate": null`) {
		t.Errorf("PR content wrong:\n%s", pr.Content)
	}
	for _, want := range []string{"18\" Mid-Mountain Base", "https://evidence.example/dev/a-basin/", fixMarker("dev", "a-basin"), "2 attempt(s)"} {
		if !strings.Contains(pr.Body, want) {
			t.Errorf("PR body missing %q:\n%s", want, pr.Body)
		}
	}
}

func TestRunReportsBrokenScraperWhenUnverified(t *testing.T) {
	store := abasinStore("unknown", 0)
	repo := &fakeRepo{}
	bad := &ConfigProposal{Config: supabase.ScrapingConfig{ID: 5, Name: "A-Basin"}, Expected: ExpectedValues{BaseDepth: 18, Snow24h: -1, Snow48h: -1, SeasonTotal: -1, RunsOpen: -1, LiftsOpen: -1}}
	claude := &fakeClaude{proposals: []*ConfigProposal{bad, bad}, reading: &StatusReading{Status: "unknown"}}
	m := newTestMonitor(store, claude, repo, func(supabase.ScrapingConfig) (map[string]interface{}, error) {
		return nil, errors.New("no value found for base depth")
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.prs) != 0 {
		t.Fatalf("unverified fix must not open a PR")
	}
	if len(repo.created) != 1 || !strings.Contains(repo.created[0].Body, brokenMarker("dev", "a-basin")) {
		t.Fatalf("expected a broken-scraper issue, got %+v", repo.created)
	}
	if !strings.Contains(repo.created[0].Body, "Attempt 2") {
		t.Errorf("issue should list both attempts:\n%s", repo.created[0].Body)
	}
}

func TestRunRespectsModelCallBudget(t *testing.T) {
	store := abasinStore("open", 0)
	claude := &fakeClaude{reading: &StatusReading{Status: "closed", Confidence: "high", Quote: "Closed"}}
	repo := &fakeRepo{}
	m := newTestMonitor(store, claude, repo, func(supabase.ScrapingConfig) (map[string]interface{}, error) {
		return map[string]interface{}{"lifts_open": 0}, nil
	})
	m.MaxModelCalls = 0
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if claude.calls != 0 || len(repo.created) != 0 {
		t.Errorf("budget of 0 must block Claude calls: calls=%d issues=%d", claude.calls, len(repo.created))
	}
}
