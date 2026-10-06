package factory

import "testing"

func TestEnvEnabled(t *testing.T) {
	cases := map[string]bool{
		"": false, "0": false, "false": false, "FALSE": false, "no": false, "off": false, " 0 ": false,
		"1": true, "true": true, "yes": true, "api": true,
	}
	for value, want := range cases {
		t.Setenv("KHBB_TEST_FLAG", value)
		if got := envEnabled("KHBB_TEST_FLAG"); got != want {
			t.Errorf("envEnabled with %q = %v, want %v", value, got, want)
		}
	}
}
