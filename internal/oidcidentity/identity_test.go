package oidcidentity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.signature"

type recordingOutputRunner struct {
	name string
	args []string
	out  []byte
	err  error
}

func (r *recordingOutputRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	out := append([]byte(nil), r.out...)
	return out, r.err
}

func TestNormalizeStripsCRLF(t *testing.T) {
	token, err := Normalize([]byte(sampleJWT + "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != sampleJWT {
		t.Fatalf("token = %q", token)
	}
}

func TestNormalizeRejectsEmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "nil"},
		{name: "empty", raw: []byte{}},
		{name: "whitespace", raw: []byte(" \n")},
		{name: "internal space", raw: []byte("a b.c")},
		{name: "no dots", raw: []byte("nodots")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Normalize(tt.raw); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestFileSourceNormalizesAndDoesNotLeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.jwt")
	if err := os.WriteFile(path, []byte(sampleJWT+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := (FileSource{Path: path}).Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != sampleJWT {
		t.Fatalf("token = %q", token)
	}
}

func TestFileSourceRejectsUnsafePath(t *testing.T) {
	if _, err := (FileSource{Path: "-token.jwt"}).Token(context.Background()); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandSourceRedactsOutputOnFailure(t *testing.T) {
	secret := sampleJWT + "-leaked"
	runner := &recordingOutputRunner{out: []byte(secret), err: errors.New(secret)}
	source := CommandSource{Name: "gcloud", Args: []string{"auth", "print-identity-token"}, runner: runner}
	token, err := source.Token(context.Background())
	if token != nil || !errors.Is(err, ErrTokenUnavailable) || strings.Contains(err.Error(), secret) {
		t.Fatalf("token=%q error=%v", token, err)
	}
	if runner.name != "gcloud" {
		t.Fatalf("command = %q", runner.name)
	}
}

func TestCommandSourceNormalizesStdout(t *testing.T) {
	runner := &recordingOutputRunner{out: []byte(sampleJWT + "\n")}
	source := CommandSource{
		Name:   "gcloud",
		Args:   []string{"auth", "print-identity-token", "--audiences=sigstore"},
		runner: runner,
	}
	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != sampleJWT {
		t.Fatalf("token = %q", token)
	}
}

func TestSourceFromEnvSelectsExactlyOne(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		source, err := SourceFromEnv(func(key string) string {
			if key == EnvTokenFile {
				return "/tmp/token.jwt"
			}
			return ""
		})
		if err != nil {
			t.Fatal(err)
		}
		file, ok := source.(FileSource)
		if !ok || file.Path != "/tmp/token.jwt" {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("argv", func(t *testing.T) {
		source, err := SourceFromEnv(func(key string) string {
			if key == EnvTokenArgv {
				return `["gcloud","auth","print-identity-token","--audiences=sigstore"]`
			}
			return ""
		})
		if err != nil {
			t.Fatal(err)
		}
		command, ok := source.(CommandSource)
		if !ok || command.Name != "gcloud" || strings.Join(command.Args, " ") != "auth print-identity-token --audiences=sigstore" {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("none", func(t *testing.T) {
		source, err := SourceFromEnv(func(string) string { return "" })
		if err != nil || source != nil {
			t.Fatalf("source = %#v err = %v", source, err)
		}
	})
	t.Run("conflict", func(t *testing.T) {
		_, err := SourceFromEnv(func(key string) string {
			switch key {
			case EnvTokenFile:
				return "/tmp/token.jwt"
			case EnvTokenArgv:
				return `["gcloud"]`
			default:
				return ""
			}
		})
		if !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid argv", func(t *testing.T) {
		_, err := SourceFromEnv(func(key string) string {
			if key == EnvTokenArgv {
				return "gcloud auth"
			}
			return ""
		})
		if !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMaterializeWritesStrippedToken(t *testing.T) {
	path, cleanup, err := Materialize(context.Background(), EnvSource{Value: sampleJWT + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != sampleJWT {
		t.Fatalf("file = %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("perm = %s", info.Mode())
	}
}

func TestMaterializeNilSource(t *testing.T) {
	path, cleanup, err := Materialize(context.Background(), nil)
	if err != nil || path != "" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	cleanup()
}

func TestClaimsPrefersEmailAndRejectsUnsafeIssuer(t *testing.T) {
	t.Run("email", func(t *testing.T) {
		got, err := Claims([]byte(identityJWT(t, map[string]string{
			"email": "release@example.invalid",
			"sub":   "ignored",
			"iss":   "https://accounts.google.com",
		})))
		if err != nil {
			t.Fatal(err)
		}
		if got.Subject != "release@example.invalid" || got.Issuer != "https://accounts.google.com" {
			t.Fatalf("claims = %#v", got)
		}
	})
	t.Run("sub", func(t *testing.T) {
		got, err := Claims([]byte(identityJWT(t, map[string]string{
			"sub": "repo:example/shop:ref:refs/heads/main",
			"iss": "https://token.actions.githubusercontent.com",
		})))
		if err != nil {
			t.Fatal(err)
		}
		if got.Subject != "repo:example/shop:ref:refs/heads/main" || got.Issuer != "https://token.actions.githubusercontent.com" {
			t.Fatalf("claims = %#v", got)
		}
	})
	t.Run("http issuer", func(t *testing.T) {
		if _, err := Claims([]byte(identityJWT(t, map[string]string{
			"email": "release@example.invalid",
			"iss":   "http://accounts.google.com",
		}))); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCloudCLISourceUsesGcloudAndImpersonation(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "gcloud" {
			return "/usr/bin/gcloud", nil
		}
		return "", errors.New("missing")
	}
	t.Run("no impersonate", func(t *testing.T) {
		source := CloudCLISource(func(string) string { return "" }, lookPath, "")
		command, ok := source.(CommandSource)
		if !ok || command.Name != "gcloud" || strings.Join(command.Args, " ") != "auth print-identity-token --audiences=sigstore --include-email" {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("magelift env", func(t *testing.T) {
		source := CloudCLISource(func(key string) string {
			if key == EnvSigningServiceAccount {
				return "ci@example.iam.gserviceaccount.com"
			}
			return ""
		}, lookPath, "")
		command, ok := source.(CommandSource)
		if !ok || strings.Join(command.Args, " ") != "auth print-identity-token --audiences=sigstore --include-email --impersonate-service-account ci@example.iam.gserviceaccount.com" {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("env wins over yaml", func(t *testing.T) {
		source := CloudCLISource(func(key string) string {
			if key == EnvSigningServiceAccount {
				return "env@example.iam.gserviceaccount.com"
			}
			return ""
		}, lookPath, "yaml@example.iam.gserviceaccount.com")
		command, ok := source.(CommandSource)
		if !ok || !strings.HasSuffix(strings.Join(command.Args, " "), "--impersonate-service-account env@example.iam.gserviceaccount.com") {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("yaml when env empty", func(t *testing.T) {
		source := CloudCLISource(func(string) string { return "" }, lookPath, "yaml@example.iam.gserviceaccount.com")
		command, ok := source.(CommandSource)
		if !ok || !strings.HasSuffix(strings.Join(command.Args, " "), "--impersonate-service-account yaml@example.iam.gserviceaccount.com") {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("missing gcloud", func(t *testing.T) {
		source := CloudCLISource(func(string) string { return "" }, func(string) (string, error) {
			return "", errors.New("not on PATH")
		}, "ci@example.iam.gserviceaccount.com")
		if source != nil {
			t.Fatalf("source = %#v", source)
		}
	})
}

func TestSourceFromEnvOrCloudCLIPrefersEnv(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/bin/gcloud", nil }
	source, err := SourceFromEnvOrCloudCLI(func(key string) string {
		if key == EnvTokenFile {
			return "/tmp/token.jwt"
		}
		return ""
	}, lookPath, "ci@example.iam.gserviceaccount.com")
	if err != nil {
		t.Fatal(err)
	}
	file, ok := source.(FileSource)
	if !ok || file.Path != "/tmp/token.jwt" {
		t.Fatalf("source = %#v", source)
	}
}

func identityJWT(t *testing.T, claims map[string]string) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
