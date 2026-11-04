package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// statusLine matches page text that can say whether a resort is open: opening/closing
// announcements, season notes, and lift/trail status lines.
var statusLine = regexp.MustCompile(`(?i)\b(open|opens|opened|opening|closed|closes|closing|close|season|operat\w*|lifts?|trails?|runs?|terrain)\b`)

// clockTime strips times of day ("6:02 AM", "14:30") so "updated at" stamps don't count as changes.
var clockTime = regexp.MustCompile(`(?i)\b\d{1,2}:\d{2}(:\d{2})?\s*(am|pm|a\.m\.|p\.m\.)?`)

// pageFingerprint hashes the status-related lines of a page's visible text. Weather,
// temperatures and other text that changes daily don't affect it, so an unchanged
// fingerprint means the page says the same thing about the resort's status as last time.
func pageFingerprint(visibleText string) string {
	seen := map[string]bool{}
	var lines []string
	for _, line := range strings.Split(visibleText, "\n") {
		if !statusLine.MatchString(line) {
			continue
		}
		line = clockTime.ReplaceAllString(strings.ToLower(line), "")
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		lines = append(lines, line)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
