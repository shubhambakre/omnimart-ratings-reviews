package moderation

import (
	"regexp"
	"strings"
)

type Decision struct {
	Clean  bool
	Reason string
}

type Moderator interface {
	Inspect(title, body string) Decision
}

// SimpleModerator is a stand-in for a real moderation pipeline (profanity model,
// PII scrubber, spam classifier). In production this would be an async call to
// a moderation service; the interface boundary here is what gets re-pointed.
type SimpleModerator struct {
	banned []string
	piiRe  *regexp.Regexp
}

func NewSimpleModerator() *SimpleModerator {
	return &SimpleModerator{
		banned: []string{"badword", "scam", "fraud", "garbage"},
		// crude email + US phone regex; PII triggers manual review
		piiRe: regexp.MustCompile(`(?i)([a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}|\b\d{3}[-.\s]?\d{3}[-.\s]?\d{4}\b)`),
	}
}

func (m *SimpleModerator) Inspect(title, body string) Decision {
	combined := strings.ToLower(title + " " + body)
	for _, w := range m.banned {
		if strings.Contains(combined, w) {
			return Decision{Clean: false, Reason: "contains banned term: " + w}
		}
	}
	if m.piiRe.MatchString(title + " " + body) {
		return Decision{Clean: false, Reason: "contains PII"}
	}
	return Decision{Clean: true}
}
