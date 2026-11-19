package pipeline

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPHPRuntimeHeadlessCORS(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP is not installed")
	}
	prepend, err := filepath.Abs(filepath.Join("..", "..", "..", "images", "php-nginx", "deployment-config.php"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	router := filepath.Join(dir, "index.php")
	if err := os.WriteFile(router, []byte(`<?php echo $_SERVER['HTTP_AUTHORIZATION'] ?? 'native';`), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, php, "-d", "auto_prepend_file="+prepend, "-S", address, router)
	cmd.Env = append(os.Environ(), "CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ALLOWED_ORIGINS=https://shop.example,https://other.example")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	client := &http.Client{Timeout: time.Second}
	base := "http://" + address
	for i := 0; ; i++ {
		res, err := client.Get(base + "/")
		if err == nil {
			_ = res.Body.Close()
			break
		}
		if i == 100 {
			t.Fatalf("PHP server did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	request := func(method, path, origin string, headers map[string]string) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequest(method, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return res, string(body)
	}
	res, _ := request("OPTIONS", "/graphql", "https://shop.example", map[string]string{"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "authorization,content-type,store"})
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "https://shop.example" || res.Header.Get("Access-Control-Allow-Credentials") != "true" || !strings.Contains(res.Header.Get("Access-Control-Allow-Headers"), "authorization") {
		t.Fatalf("preflight: %d %v", res.StatusCode, res.Header)
	}
	res, body := request("GET", "/rest/V1/customers/me", "https://shop.example", map[string]string{"Authorization": "Bearer test"})
	if res.StatusCode != 200 || body != "Bearer test" || res.Header.Get("Access-Control-Allow-Origin") != "https://shop.example" {
		t.Fatalf("authenticated API: %d %v %q", res.StatusCode, res.Header, body)
	}
	res, _ = request("OPTIONS", "/graphql", "https://evil.example", map[string]string{"Access-Control-Request-Method": "POST"})
	if res.StatusCode != 403 || res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted preflight: %d %v", res.StatusCode, res.Header)
	}
	res, body = request("GET", "/", "", nil)
	if res.StatusCode != 200 || body != "native" || res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("native request: %d %v %q", res.StatusCode, res.Header, body)
	}
	res, _ = request("GET", "/graphql", "https://shop.example.evil", nil)
	if res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("origin prefix matched")
	}
	res, _ = request("POST", "/graphql", "https://other.example", nil)
	if res.Header.Get("Access-Control-Allow-Origin") != "https://other.example" {
		t.Fatalf("second configured origin: %v", res.Header)
	}
	res, _ = request("OPTIONS", "/graphql", "https://shop.example", map[string]string{"Access-Control-Request-Method": "TRACE"})
	if res.StatusCode != 403 {
		t.Fatalf("unsupported method: %d", res.StatusCode)
	}
}
