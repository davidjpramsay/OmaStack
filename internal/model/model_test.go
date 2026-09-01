package model

import "testing"

func TestRestartPolicy(t *testing.T) {
	cases := []struct {
		policy        RestartPolicy
		code, attempt int
		want          bool
	}{
		{RestartPolicy{Mode: "never"}, 1, 0, false},
		{RestartPolicy{Mode: "on-failure"}, 1, 0, true},
		{RestartPolicy{Mode: "on-failure"}, 0, 0, false},
		{RestartPolicy{Mode: "always"}, 0, 0, true},
		{RestartPolicy{Mode: "always", MaxAttempts: 2}, 1, 2, false},
	}
	for _, test := range cases {
		if got := test.policy.ShouldRestart(test.code, test.attempt); got != test.want {
			t.Fatalf("%#v => %v, want %v", test, got, test.want)
		}
	}
}
