package diagnostics

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestListOnlyMetadataSortedBounded(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, name := range []string{"one.pdf", "two.pdf", "three.pdf"} {
		_, err := s.Capture(Event{Username: "staff", Filename: name, Stage: "OCR_FAILED", Message: "error cookie:secret"}, strings.NewReader("%PDF-1.4\nbody"), 13, "cookie:private")
		if err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.List(2)
	if err != nil || len(events) != 2 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	if events[0].Time < events[1].Time {
		t.Fatal("not newest first")
	}
	if strings.Contains(events[0].Message, "secret") {
		t.Fatalf("leaked %+v", events[0])
	}
	if _, err := filepath.Abs(s.Root); err != nil {
		t.Fatal(err)
	}
	events, err = s.List(101)
	if err != nil || len(events) != 3 {
		t.Fatalf("invalid limit => len=%d err=%v", len(events), err)
	}
	_, _ = io.Discard.Write([]byte{})
}
