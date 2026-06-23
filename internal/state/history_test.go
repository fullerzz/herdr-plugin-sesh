package state

import "testing"

func TestHistoryRecordsMostRecentWithoutDuplicates(t *testing.T) {
	d := t.TempDir()
	if err := Record(d, "a"); err != nil {
		t.Fatal(err)
	}
	if err := Record(d, "b"); err != nil {
		t.Fatal(err)
	}
	if err := Record(d, "b"); err != nil {
		t.Fatal(err)
	}
	last, ok, err := Last(d)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || last != "a" {
		t.Fatalf("last=%q ok=%v", last, ok)
	}
}
