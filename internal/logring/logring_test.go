package logring

import "testing"

func TestRingKeepsNewestAndPagesBySeq(t *testing.T) {
	r := New(3)
	for _, s := range []string{"a", "b", "c", "d"} {
		_, _ = r.Write([]byte(`{"msg":"` + s + `"}` + "\n"))
	}
	all, last := r.After(0)
	if len(all) != 3 || last != 4 || string(all[0].JSON) != `{"msg":"b"}` {
		t.Fatalf("got %d lines, last %d, first %s", len(all), last, all[0].JSON)
	}
	if more, _ := r.After(3); len(more) != 1 || more[0].Seq != 4 {
		t.Fatalf("after 3: %+v", more)
	}
	if none, _ := r.After(4); len(none) != 0 {
		t.Fatal("expected nothing new")
	}
}
