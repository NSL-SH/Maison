package engine

import (
	"reflect"
	"testing"
)

// The stdout tee behind RunStream: lines arrive whole whatever the chunking, blank
// ones are dropped, and a last line with no newline still arrives on flush.
func TestLineWriterSplitsAcrossChunks(t *testing.T) {
	var got []string
	w := &lineWriter{onLine: func(l string) { got = append(got, l) }}
	for _, chunk := range []string{`{"type":"pro`, "gress\"}\n\n  ", "two\nthr", "ee"} {
		if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if want := []string{`{"type":"progress"}`, "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("before flush: got %q, want %q", got, want)
	}
	w.flush()
	if want := []string{`{"type":"progress"}`, "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after flush: got %q, want %q", got, want)
	}
}

func TestLineWriterNilCallbackIsANoop(t *testing.T) {
	w := &lineWriter{}
	if n, err := w.Write([]byte("a\nb")); err != nil || n != 3 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	w.flush()
}
