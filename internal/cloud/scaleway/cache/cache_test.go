package cache

import "testing"

func TestRedisEndpointHost(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"redis://cache.internal:6379":  "cache.internal",
		"rediss://cache.internal:6380": "cache.internal",
		"cache.internal:6379":          "cache.internal",
		"cache.internal":               "cache.internal",
		"":                             "",
		"   ":                          "",
	}
	for input, want := range cases {
		if got := redisEndpointHost(input); got != want {
			t.Fatalf("redisEndpointHost(%q)=%q want %q", input, got, want)
		}
	}
}
