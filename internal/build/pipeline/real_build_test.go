package pipeline

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildrunner "github.com/magelift/magelift/internal/build/runner"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/source"
	"github.com/magelift/magelift/internal/toolchain"
)

// Opt-in integration against an owned synthetic shop and prebuilt local images.
// It pushes only when MAGELIFT_REAL_IMAGE explicitly selects an owned registry.
// It never provisions cloud resources.
func TestRealBuildKeepsSecretsOutOfImageAndPreservesDisabledModule(t *testing.T) {
	root := os.Getenv("MAGELIFT_REAL_SHOP_ROOT")
	if root == "" {
		t.Skip("requires MAGELIFT_REAL_SHOP_ROOT and explicit local images")
	}
	builderRef, runtimeRef := os.Getenv("MAGELIFT_REAL_BUILDER_IMAGE"), os.Getenv("MAGELIFT_REAL_RUNTIME_IMAGE")
	if builderRef == "" || runtimeRef == "" {
		t.Fatal("explicit builder/runtime images are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	repository, err := (source.GitInspector{}).Inspect(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	configBytes, err := os.ReadFile(filepath.Join(root, "magelift.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(configBytes)
	if err != nil {
		t.Fatal(err)
	}
	images, err := toolchain.NewResolver(toolchain.SystemCommandRunner(), builderRef, runtimeRef).Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeConfig, err := os.ReadFile(filepath.Join(root, "app", "etc", "config.php"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "etc", "env.php")); !os.IsNotExist(err) {
		t.Fatal("synthetic source must have no runtime env.php")
	}
	const canary = "ml03-fake-credential-8e7c4f905b1d"
	auth := []byte(`{"http-basic":{"example.invalid":{"username":"acceptance","password":"` + canary + `"}}}`)
	defer clear(auth)
	var diagnostics bytes.Buffer
	if proofDir := os.Getenv("MAGELIFT_REAL_PROOF_DIR"); proofDir != "" {
		if err := os.MkdirAll(proofDir, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.WriteFile(filepath.Join(proofDir, "build.log"), diagnostics.Bytes(), 0o600); err != nil {
				t.Error(err)
			}
		})
	}
	work := t.TempDir()
	engine := buildkit.NewRunner(&diagnostics)
	engine.TempRoot = work
	useEmbedded := os.Getenv("MAGELIFT_REAL_EMBEDDED_RECIPE") == "1"
	request := Request{
		Repository: repository, Config: file, BuilderImage: images.BuilderID, RuntimeImage: runtimeRef, RuntimeImageID: images.RuntimeID,
		ImageReference: "magelift/ml03-proof:" + repository.Revision[:12], Platform: "linux/amd64", MageLiftVersion: "0.0.0-dev", ArtifactDirectory: work, ComposerAuth: auth,
		UseEmbeddedRecipe: useEmbedded,
	}
	if target := os.Getenv("MAGELIFT_REAL_IMAGE"); target != "" {
		if !useEmbedded {
			t.Fatal("registry acceptance requires embedded source recipe")
		}
		request.ImageReference = target
		request.Output = buildkit.OutputPush
		request.Platform = ""
		request.Platforms = []string{"linux/amd64"}
		request.ProvenanceSource = os.Getenv("MAGELIFT_REAL_SOURCE_URL")
	}
	if cache := os.Getenv("MAGELIFT_REAL_CACHE_REF"); cache != "" {
		request.BuildCache = &config.BuildCache{From: []string{cache}, To: cache, Mode: "max"}
	}
	result, err := New(ContainerAdapter{TempRoot: work, Stderr: &diagnostics, Network: "bridge", Platform: "linux/amd64", MageLiftVersion: "0.0.0-dev"}, engine).Run(ctx, request)
	if bytes.Contains(diagnostics.Bytes(), []byte(canary)) {
		t.Fatal("synthetic credential appeared in diagnostics")
	}
	if err != nil {
		t.Fatalf("real build failed: %v", err)
	}
	manifest, err := os.ReadFile(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(manifest, []byte(canary)) {
		t.Fatal("synthetic credential appeared in manifest")
	}
	var metadata struct {
		MageLiftVersion string                   `json:"mageLiftVersion"`
		EnabledModules  []string                 `json:"enabledModules"`
		BuildRecipe     *buildrunner.BuildRecipe `json:"buildRecipe"`
	}
	if err := json.Unmarshal(manifest, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.MageLiftVersion != "0.0.0-dev" {
		t.Fatalf("manifest CLI version=%s", metadata.MageLiftVersion)
	}
	disabled := os.Getenv("MAGELIFT_REAL_DISABLED_MODULE")
	for _, module := range metadata.EnabledModules {
		if disabled != "" && module == disabled {
			t.Fatal("disabled module became enabled")
		}
	}
	afterConfig, err := os.ReadFile(filepath.Join(root, "app", "etc", "config.php"))
	if err != nil || !bytes.Equal(beforeConfig, afterConfig) {
		t.Fatal("build modified source configuration")
	}
	if _, err := (source.GitInspector{}).Inspect(ctx, root); err != nil {
		t.Fatal(err)
	}
	afterImages, err := toolchain.NewResolver(toolchain.SystemCommandRunner(), builderRef, runtimeRef).Resolve(ctx)
	if err != nil || afterImages.RuntimeID != images.RuntimeID {
		t.Fatal("runtime changed during build")
	}
	inspectionReference := result.Image.ImageReference
	if result.Image.Pushed {
		inspectionReference += "@" + result.Image.Digest
		if output, err := exec.CommandContext(ctx, "docker", "pull", "--platform", "linux/amd64", inspectionReference).CombinedOutput(); err != nil {
			t.Fatalf("pull published image: %v %s", err, output)
		}
	}
	labelBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format={{json .Config.Labels}}", inspectionReference).Output()
	if err != nil {
		t.Fatal(err)
	}
	var labels map[string]string
	if err := json.Unmarshal(labelBytes, &labels); err != nil {
		t.Fatal(err)
	}
	if labels["io.magelift.builder.image"] != images.BuilderID || labels["org.opencontainers.image.version"] != "0.0.0-dev" {
		t.Fatal("OCI trace labels do not match build inputs")
	}
	if useEmbedded {
		if metadata.BuildRecipe == nil || metadata.BuildRecipe.Platform != "linux/amd64" || metadata.BuildRecipe.SHA256 != labels["io.magelift.recipe.sha256"] || len(metadata.BuildRecipe.BaseImages) != 3 {
			t.Fatal("external manifest lacks exact recipe/platform/upstream trace")
		}
		if !strings.HasPrefix(labels["io.magelift.runtime.image"], "recipe:sha256:") || labels["io.magelift.recipe.sha256"] == "" {
			t.Fatalf("embedded recipe labels = %#v", labels)
		}
	} else if labels["io.magelift.runtime.image"] != images.RuntimeID {
		t.Fatal("OCI runtime trace label does not match build inputs")
	}
	if disabled != "" {
		code := `$c=require "/app/app/etc/config.php"; exit(($c["modules"][getenv("ML_DISABLED_MODULE")]??null)===0?0:1);`
		cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none", "--platform", "linux/amd64", "--entrypoint", "php", "--env", "ML_DISABLED_MODULE="+disabled, inspectionReference, "-r", code)
		if err := cmd.Run(); err != nil {
			t.Fatal("final image did not preserve disabled module state")
		}
	}
	archive := filepath.Join(work, "image.tar")
	if err := exec.CommandContext(ctx, "docker", "image", "save", "--output", archive, inspectionReference).Run(); err != nil {
		t.Fatal(err)
	}
	entries, scanned, err := scanImageArchive(archive, []byte(canary))
	if err != nil {
		t.Fatal(err)
	}
	if proofDir := os.Getenv("MAGELIFT_REAL_PROOF_DIR"); proofDir != "" {
		if err := os.MkdirAll(proofDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proofDir, "build.log"), diagnostics.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		resultJSON, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := os.WriteFile(filepath.Join(proofDir, "build-result.json"), resultJSON, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proofDir, "manifest.json"), manifest, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("image=%s manifestSHA256=%s archiveEntries=%d uncompressedBytes=%d; canary absent, source unchanged, disabled module preserved", result.Image.Digest, result.ManifestSHA256, entries, scanned)
}

func scanImageArchive(path string, canary []byte) (int, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	archive := tar.NewReader(file)
	entries := 0
	var scanned int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, scanned, err
		}
		if !header.FileInfo().Mode().IsRegular() {
			continue
		}
		entries++
		buffered := bufio.NewReader(archive)
		magic, _ := buffered.Peek(4)
		var stream io.Reader = buffered
		var zipped *gzip.Reader
		if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
			zipped, err = gzip.NewReader(buffered)
			if err != nil {
				return entries, scanned, err
			}
			if strings.Contains(zipped.Name, string(canary)) || strings.Contains(zipped.Comment, string(canary)) || bytes.Contains(zipped.Extra, canary) {
				return entries, scanned, fmt.Errorf("synthetic credential in layer header")
			}
			stream = zipped
		} else if len(magic) == 4 && bytes.Equal(magic, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
			return entries, scanned, fmt.Errorf("Zstandard image layer scan is not supported by this test")
		}
		count, err := scanWithoutCanary(stream, canary)
		if zipped != nil {
			zipped.Close()
		}
		scanned += count
		if err != nil {
			return entries, scanned, err
		}
	}
	return entries, scanned, nil
}

func scanWithoutCanary(reader io.Reader, canary []byte) (int64, error) {
	buffer := make([]byte, 1<<20)
	var tail []byte
	var count int64
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			count += int64(n)
			combined := append(tail, buffer[:n]...)
			if bytes.Contains(combined, canary) {
				return count, fmt.Errorf("synthetic credential found in saved image")
			}
			keep := len(canary) - 1
			if keep > len(combined) {
				keep = len(combined)
			}
			tail = append(tail[:0], combined[len(combined)-keep:]...)
		}
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return count, err
		}
	}
}
