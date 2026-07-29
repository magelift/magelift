package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestRequestEncodingIsCanonical(t *testing.T) {
	first := validPrepareRequest()
	second := validPrepareRequest()
	second.Prepare.InputFiles[0], second.Prepare.InputFiles[1] = second.Prepare.InputFiles[1], second.Prepare.InputFiles[0]
	second.Prepare.StaticContent[0], second.Prepare.StaticContent[1] = second.Prepare.StaticContent[1], second.Prepare.StaticContent[0]

	firstJSON, err := EncodeRequest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := EncodeRequest(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical requests differ:\n%s\n%s", firstJSON, secondJSON)
	}
	if strings.Contains(strings.ToLower(string(firstJSON)), "secret") || strings.Contains(strings.ToLower(string(firstJSON)), "credential") {
		t.Fatalf("request exposes secret-bearing fields: %s", firstJSON)
	}
}

func TestFinalizeValidatesOCIDigest(t *testing.T) {
	request := Request{ProtocolVersion: 1, Stage: StageFinalize, Finalize: &FinalizeRequest{
		PreparedArtifact: "dist/rootfs.tar", SourceRevision: strings.Repeat("a", 40), ImageDigest: "sha256:bad",
	}}
	if _, err := EncodeRequest(request); err == nil || !strings.Contains(err.Error(), "OCI digest") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeResponseIsStrictAndBounded(t *testing.T) {
	valid := `{"protocolVersion":1,"stage":"finalize","finalize":{"imageDigest":"sha256:` + strings.Repeat("b", 64) + `","manifestPath":"dist/manifest.json","manifestSha256":"` + strings.Repeat("c", 64) + `"}}`
	if _, err := DecodeResponse(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]string{
		"unknown key":   strings.Replace(valid, `"stage":`, `"unknown":true,"stage":`, 1),
		"trailing data": valid + `{}`,
		"oversized":     strings.Repeat(" ", int(MaxResponseSize)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeResponse(strings.NewReader(input)); err == nil {
				t.Fatal("expected decode error")
			}
		})
	}
}

func TestEncodeRequestPreservesEmptyStaticContentList(t *testing.T) {
	request := validPrepareRequest()
	request.Prepare.StaticContent = []StaticContent{}

	encoded, err := EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"staticContent":[]`)) {
		t.Fatalf("empty static content encoded as %s", encoded)
	}
}

func TestEncodeRequestIncludesStaticContentStrategyAndThreads(t *testing.T) {
	request := validPrepareRequest()
	request.Prepare.StaticContent = []StaticContent{
		{Locale: "en_US", Theme: "Magento/luma", Strategy: "compact", Threads: 4},
	}

	encoded, err := EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"strategy":"compact"`)) || !bytes.Contains(encoded, []byte(`"threads":4`)) {
		t.Fatalf("encoded request missing strategy/threads: %s", encoded)
	}
}

func TestPrepareResponseCanonicalizesSetAndChecksumOrder(t *testing.T) {
	response := Response{ProtocolVersion: 1, Stage: StagePrepare, Prepare: &PrepareResponse{
		PreparedArtifact: "dist/rootfs.tar",
		PHPVersion:       "8.5.1",
		PHPExtensions:    []string{"pdo_mysql", "intl"},
		EnabledModules:   []string{"Vendor_Second", "Magento_Catalog"},
		Checksums: []FileChecksum{
			{Path: "vendor/autoload.php", SHA256: strings.Repeat("d", 64)},
			{Path: "app/etc/config.php", SHA256: strings.Repeat("e", 64)},
		},
		RequiredRuntimeCapabilities: []string{"search.opensearch", "database.mysql"},
	}}
	encoded, err := EncodeResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(encoded), "app/etc/config.php") > strings.Index(string(encoded), "vendor/autoload.php") ||
		strings.Index(string(encoded), "database.mysql") > strings.Index(string(encoded), "search.opensearch") {
		t.Fatalf("response is not canonical: %s", encoded)
	}
}

func TestRejectsInvalidStagesAndUnsafePaths(t *testing.T) {
	request := validPrepareRequest()
	request.Prepare.InputFiles[0].Path = "../auth.json"
	if _, err := EncodeRequest(request); err == nil || !strings.Contains(err.Error(), "cannot traverse") {
		t.Fatalf("unexpected path error: %v", err)
	}
	request = validPrepareRequest()
	request.Stage = "compile"
	if _, err := EncodeRequest(request); err == nil || !strings.Contains(err.Error(), "invalid build runner stage") {
		t.Fatalf("unexpected stage error: %v", err)
	}
}

func validPrepareRequest() Request {
	return Request{ProtocolVersion: 1, Stage: StagePrepare, Prepare: &PrepareRequest{
		RepositoryRoot:      "/workspace/shop",
		SourceRevision:      strings.Repeat("a", 40),
		Application:         Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		PHPVersion:          "8.5.1",
		CompatibilityStatus: "supported",
		InputFiles: []InputFile{
			{Path: "composer.lock", SHA256: strings.Repeat("b", 64)},
			{Path: "app/etc/config.php", SHA256: strings.Repeat("c", 64)},
		},
		StaticContent: []StaticContent{{Locale: "fr_FR", Theme: "Magento/luma"}, {Locale: "en_US", Theme: "Magento/luma"}},
	}}
}
