package adapter

import (
	"testing"

	"github.com/yundera/maison/internal/apps"
)

// Progress lines are decoded one at a time as stdout streams in; everything else is
// left for decode at the end, so nothing is emitted twice.
func TestEmitProgressLine(t *testing.T) {
	var got []apps.Event
	emit := func(ev apps.Event) { got = append(got, ev) }

	emitProgressLine(`{"type":"progress","message":"uploading","pct":42.5,"done":10,"total":20}`, emit)
	emitProgressLine(`{"type":"progress","message":"hashing","done":5,"total":50}`, emit)
	emitProgressLine(`{"type":"result","result":{}}`, emit)
	emitProgressLine(`{"type":"log","message":"hello"}`, emit)
	emitProgressLine(`not json`, emit)
	emitProgressLine(`{"type":"progress"}`, nil) // a nil emit is fine

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Pct != 42.5 || got[0].Done != 10 || got[0].Total != 20 || got[0].Message != "uploading" {
		t.Errorf("first event = %+v", got[0])
	}
	if got[1].Pct != apps.PctUnknown || got[1].Done != 5 || got[1].Total != 50 {
		t.Errorf("second event (no pct) = %+v, want PctUnknown with the byte counts", got[1])
	}
}
