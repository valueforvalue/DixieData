package templates

import (
	"bytes"
	"context"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
	"strings"
	"testing"
)

func TestEventDetailHasSingleEditEventLink(t *testing.T) {
	var buf bytes.Buffer
	if err := EventDetail(viewmodel.EventRecord{PersonRecord: viewmodel.PersonRecord{ID: 519, DisplayID: "EVT-00519"}, Kind: "Battle"}, nil).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Count(got, `href="/events/519/edit"`) != 1 {
		t.Fatalf("Edit Event count = %d, want 1; got %q", strings.Count(got, `href="/events/519/edit"`), got)
	}
	if !strings.Contains(got, `href="/events/519/edit"`) {
		t.Fatalf("missing page-level edit link: %q", got)
	}
	if strings.Contains(got, `data-action="/events/519/edit"`) {
		t.Fatalf("nonfunctional panel edit action remains: %q", got)
	}
}
