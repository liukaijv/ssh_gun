package autostart

import "testing"

func TestQuoteExecutable(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`C:\Apps\Feisuo.exe`, `C:\Apps\Feisuo.exe`},
		{`C:\Program Files\Feisuo\Feisuo.exe`, `"C:\Program Files\Feisuo\Feisuo.exe"`},
		{`  D:\Feisuo.exe  `, `D:\Feisuo.exe`},
		{"", ""},
	}
	for _, tt := range cases {
		if got := quoteExecutable(tt.in); got != tt.want {
			t.Errorf("quoteExecutable(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
