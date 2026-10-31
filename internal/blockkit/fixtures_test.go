package blockkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/slack-go/slack"
)

// fixturePayload mirrors the shape of the JSON files in testdata/.
// Not all fields are populated for every fixture; that's fine —
// json.Unmarshal leaves missing fields zero-valued.
type fixturePayload struct {
	Blocks      slack.Blocks       `json:"blocks"`
	Attachments []slack.Attachment `json:"attachments"`
}

func loadFixture(t *testing.T, name string) fixturePayload {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var p fixturePayload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return p
}

// assertBlockTypes parses a fixture's blocks and checks the type of
// each, in order.
func assertBlockTypes(t *testing.T, name string, want ...string) []Block {
	t.Helper()
	blocks := Parse(loadFixture(t, name).Blocks)
	var got []string
	for _, b := range blocks {
		got = append(got, blockType(b))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s: block types = %v, want %v", name, got, want)
	}
	return blocks
}

func TestFixture_GitHubPR(t *testing.T) {
	b := assertBlockTypes(t, "github_pr.json", "header", "section", "context")
	if h := b[0].(HeaderBlock); h.Text != "Pull Request opened" {
		t.Errorf("header = %q", h.Text)
	}
	if !RendersBody(b) {
		t.Error("RendersBody = false, want true")
	}
}

func TestFixture_GitHubPendingReview(t *testing.T) {
	b := assertBlockTypes(t, "github_pending_review.json", "rich_text")
	if RendersBody(b) {
		t.Error("a rich_text body renders through the body row; RendersBody = true")
	}
}

func TestFixture_PagerDutyAlert(t *testing.T) {
	p := loadFixture(t, "pagerduty_alert.json")
	if len(Parse(p.Blocks)) != 0 {
		t.Error("pagerduty fixture has no blocks")
	}
	atts := ParseAttachments(p.Attachments)
	if len(atts) != 1 {
		t.Fatalf("attachments = %d, want 1", len(atts))
	}
	if len(atts[0].Fields) != 4 {
		t.Errorf("fields = %d, want 4", len(atts[0].Fields))
	}
}

func TestFixture_DeployApproval(t *testing.T) {
	b := assertBlockTypes(t, "deploy_approval.json", "section", "actions")
	if a := b[1].(ActionsBlock); len(a.Elements) != 2 {
		t.Errorf("actions elements = %d, want 2 (Approve, Deny)", len(a.Elements))
	}
}

func TestFixture_OncallHandoff(t *testing.T) {
	assertBlockTypes(t, "oncall_handoff.json", "header", "section", "divider", "context")
}

func TestFixture_SectionWithFields(t *testing.T) {
	b := assertBlockTypes(t, "section_with_fields.json", "section")
	if s := b[0].(SectionBlock); len(s.Fields) == 0 {
		t.Error("section has no fields")
	}
}

func TestFixture_HeaderDividerSection(t *testing.T) {
	assertBlockTypes(t, "header_divider_section.json", "header", "divider", "section")
}
