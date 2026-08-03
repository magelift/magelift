package cache

import "testing"

func TestRedisEndpointHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "redis url", input: "redis://cache.internal:6379", want: "cache.internal"},
		{name: "rediss url", input: "rediss://cache.internal:6380", want: "cache.internal"},
		{name: "host port", input: "cache.internal:6379", want: "cache.internal"},
		{name: "bare host", input: "cache.internal", want: "cache.internal"},
		{name: "empty", input: "", want: ""},
		{name: "whitespace", input: "   ", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := redisEndpointHost(tc.input); got != tc.want {
				t.Fatalf("redisEndpointHost(%q)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}
