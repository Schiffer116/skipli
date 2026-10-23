package cli

import (
	"strings"
	"testing"
)

func TestPick(t *testing.T) {
	ids := []string{"abc1", "abc2", "xyz"}
	for _, c := range []struct{ ref, want, err string }{
		{"xy", "xyz", ""},
		{"abc1", "abc1", ""},
		{"abc", "", "ambiguous"},
		{"q", "", "no card"},
		{"", "", "empty"},
	} {
		got, err := pick("card", c.ref, ids)
		if got != c.want || (err == nil) != (c.err == "") || err != nil && !strings.Contains(err.Error(), c.err) {
			t.Errorf("pick(%q) = %q, %v; want %q, %q", c.ref, got, err, c.want, c.err)
		}
	}
}

func TestPlace(t *testing.T) {
	siblings := []string{"a", "b", "c"}
	str := func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	}
	for _, c := range []struct{ before, after, wantBefore, wantAfter string }{
		{"", "", "c", "-"},
		{"a", "", "-", "a"},
		{"b", "", "a", "b"},
		{"", "c", "c", "-"},
		{"", "a", "a", "b"},
	} {
		before, after, err := place("card", c.before, c.after, siblings)
		if err != nil || str(before) != c.wantBefore || str(after) != c.wantAfter {
			t.Errorf("place(%q, %q) = %s, %s, %v; want %s, %s", c.before, c.after, str(before), str(after), err, c.wantBefore, c.wantAfter)
		}
	}
	if b, a, err := place("card", "", "", nil); err != nil || b != nil || a != nil {
		t.Errorf("place into nothing = %v, %v, %v", b, a, err)
	}
}
