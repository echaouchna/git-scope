package gitstatus

import "testing"

func TestParseAheadBehind(t *testing.T) {
	t.Parallel()

	ahead, behind, ok := parseAheadBehind("# branch.ab +3 -7")
	if !ok {
		t.Fatal("expected parseAheadBehind to succeed")
	}
	if ahead != 3 || behind != 7 {
		t.Fatalf("unexpected ahead/behind: got %d/%d", ahead, behind)
	}
}

func TestParseXY(t *testing.T) {
	t.Parallel()

	staged, unstaged := parseXY("1 MM N... 100644 100644 100644 foo foo")
	if !staged || !unstaged {
		t.Fatalf("expected staged and unstaged to be true, got %v %v", staged, unstaged)
	}
}
