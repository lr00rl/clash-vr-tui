package proxies

import "testing"

func TestParseDelayQuery(t *testing.T) {
	tests := []struct {
		in        string
		wantOp    string
		wantThres int
		wantOK    bool
	}{
		{"delay<200", "<", 200, true},
		{"delay>500", ">", 500, true},
		{"delay = 300", "", 0, false}, // "= 300" not supported; only =timeout/=0
		{"delay=timeout", "timeout", 0, true},
		{"delay=0", "timeout", 0, true},
		{"hk", "", 0, false},
		{"delay", "", 0, false},
		{"DELAY<100", "<", 100, true}, // case-insensitive
	}
	for _, tt := range tests {
		op, thr, ok := parseDelayQuery(tt.in)
		if op != tt.wantOp || thr != tt.wantThres || ok != tt.wantOK {
			t.Errorf("parseDelayQuery(%q) = (%q,%d,%v), want (%q,%d,%v)",
				tt.in, op, thr, ok, tt.wantOp, tt.wantThres, tt.wantOK)
		}
	}
}

func TestMatchNode(t *testing.T) {
	tests := []struct {
		name   string
		delay  int
		filter string
		want   bool
	}{
		{"JP-Tokyo", 88, "delay<200", true},
		{"JP-Tokyo", 350, "delay<200", false},
		{"JP-Tokyo", 0, "delay<200", false}, // timeout excluded from delay<
		{"JP-Tokyo", 600, "delay>500", true},
		{"JP-Tokyo", 0, "delay=timeout", true},
		{"JP-Tokyo", 88, "delay=timeout", false},
		{"JP-Tokyo", 88, "tokyo", true}, // substring, case-insensitive
		{"JP-Tokyo", 88, "osaka", false},
	}
	for _, tt := range tests {
		if got := matchNode(tt.name, tt.delay, tt.filter); got != tt.want {
			t.Errorf("matchNode(%q, %d, %q) = %v, want %v", tt.name, tt.delay, tt.filter, got, tt.want)
		}
	}
}

func TestSortModeString(t *testing.T) {
	if SortDefault.String() != "Default" || SortByName.String() != "Name" || SortByDelay.String() != "Delay" {
		t.Errorf("SortMode.String mismatch")
	}
}
