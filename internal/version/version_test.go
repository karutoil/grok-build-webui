package version

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in     string
		want   []int
		wantOK bool
	}{
		{"v0.1", []int{0, 1}, true},
		{"0.12", []int{0, 12}, true},
		{"v1.2.3", []int{1, 2, 3}, true},
		{"v0.4+g9abc123", []int{0, 4}, true},
		{"v0.4-g9abc123", []int{0, 4}, true},
		{"v0.8-gdeadbeef-dirty", []int{0, 8}, true},
		{"dev", nil, false},
		{"", nil, false},
		{"abc", nil, false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if ok != c.wantOK {
			t.Errorf("Parse(%q) ok=%v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if len(got.Parts) != len(c.want) {
			t.Errorf("Parse(%q) = %v, want %v", c.in, got.Parts, c.want)
			continue
		}
		for i := range got.Parts {
			if got.Parts[i] != c.want[i] {
				t.Errorf("Parse(%q) = %v, want %v", c.in, got.Parts, c.want)
				break
			}
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1", "v0.2", -1},
		{"v0.2", "v0.1", 1},
		{"v0.10", "v0.9", 1}, // numeric, not lexicographic
		{"v0.1", "v0.1", 0},
		{"v0.1", "v0.1.0", 0}, // missing segment == zero
		{"v0.1+gabc", "v0.1-def", 0},
		{"v1.0", "v0.99.9", 1},
	}
	for _, c := range cases {
		pa, oka := Parse(c.a)
		pb, okb := Parse(c.b)
		if !oka || !okb {
			t.Fatalf("parse failed for %q/%q", c.a, c.b)
		}
		if got := Compare(pa, pb); got != c.want {
			t.Errorf("Compare(%s,%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestStatus(t *testing.T) {
	cases := []struct {
		cur, latest, want string
	}{
		{"v0.7", "v0.7", StatusUpToDate},
		{"v0.7", "v0.8", StatusOutOfDate},
		{"v0.10", "v0.9", StatusUpToDate}, // numeric compare wins
		{"v0.7", "", StatusUnknown},
		{"dev", "v9.9", StatusLocal},
		{"v0.7+g9abc123def", "v9.9", StatusLocal},
		{"v0.7-g9abc123", "v9.9", StatusLocal},
		{"weird", "v1.0", StatusUnknown},
	}
	for _, c := range cases {
		if got := Status(c.cur, c.latest); got != c.want {
			t.Errorf("Status(%q,%q) = %q, want %q", c.cur, c.latest, got, c.want)
		}
	}
}

func TestChannel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"v0.7", ChannelRelease},
		{"v0.7+g9abc", ChannelLocal},
		{"dev", ChannelDev},
		{"", ChannelDev},
	}
	for _, c := range cases {
		if got := Channel(c.in); got != c.want {
			t.Errorf("Channel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
