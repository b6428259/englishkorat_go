package controllers

import "testing"

func TestParseHourMinute(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expHour    int
		expMinutes int
	}{
		{
			name:       "simple time",
			input:      "08:30",
			expHour:    8,
			expMinutes: 30,
		},
		{
			name:       "iso datetime",
			input:      "2007-11-30T00:00:00+07:00",
			expHour:    0,
			expMinutes: 0,
		},
		{
			name:       "mysql datetime",
			input:      "2007-11-30 13:45:00",
			expHour:    13,
			expMinutes: 45,
		},
		{
			name:       "time with trailing zone",
			input:      "09:15:00Z",
			expHour:    9,
			expMinutes: 15,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h, m, err := parseHourMinute(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if h != tc.expHour || m != tc.expMinutes {
				t.Fatalf("expected %02d:%02d, got %02d:%02d", tc.expHour, tc.expMinutes, h, m)
			}
		})
	}
}

func TestParseHourMinuteInvalid(t *testing.T) {
	if _, _, err := parseHourMinute("invalid"); err == nil {
		t.Fatalf("expected error for invalid input")
	}
}
