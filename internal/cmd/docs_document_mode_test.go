package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsCreateDocumentMode(t *testing.T) {
	origDrive := newDriveService
	origDocs := newDocsService
	t.Cleanup(func() {
		newDriveService = origDrive
		newDocsService = origDocs
	})

	var updates []map[string]any
	var copies int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, ":batchUpdate") && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode batchUpdate: %v", err)
			}
			updates = append(updates, body)
			_ = json.NewEncoder(w).Encode(map[string]any{"replies": []any{map[string]any{}}})
		case r.Method == http.MethodGet && strings.HasPrefix(drivePath, "/files/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "doc1", "name": "Doc", "mimeType": "application/vnd.google-apps.document",
			})
		case drivePath == "/files" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "doc1", "name": "Doc", "mimeType": "application/vnd.google-apps.document",
			})
		case strings.Contains(drivePath, "/copy") && r.Method == http.MethodPost:
			copies++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "doc2", "name": "Copy", "mimeType": "application/vnd.google-apps.document",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	if err := runKong(t, &DocsCreateCmd{}, []string{"Doc"}, ctx, flags); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := runKong(t, &DocsCreateCmd{}, []string{"Doc", "--paged"}, ctx, flags); err != nil {
		t.Fatalf("paged: %v", err)
	}
	beforeCopy := len(updates)
	if err := runKong(t, &DocsCopyCmd{}, []string{"doc1", "Copy"}, ctx, flags); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copies != 1 {
		t.Fatalf("copies = %d", copies)
	}
	if len(updates) != beforeCopy {
		t.Fatalf("copy issued a style update")
	}
	if len(updates) != 2 {
		t.Fatalf("updates = %d, want 2", len(updates))
	}
	assertDocumentMode(t, updates[0], documentModePageless)
	assertDocumentMode(t, updates[1], documentModePages)
}

func TestDriveUploadConvertDocumentMode(t *testing.T) {
	origDrive := newDriveService
	origDocs := newDocsService
	t.Cleanup(func() {
		newDriveService = origDrive
		newDocsService = origDocs
	})

	var updates []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, ":batchUpdate") && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode batchUpdate: %v", err)
			}
			updates = append(updates, body)
			_ = json.NewEncoder(w).Encode(map[string]any{"replies": []any{map[string]any{}}})
		case strings.Contains(path, "/files") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "up1", "name": "notes", "mimeType": driveMimeGoogleDoc,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notes, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	if err := runKong(t, &DriveUploadCmd{}, []string{notes, "--convert"}, ctx, flags); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(updates))
	}
	assertDocumentMode(t, updates[0], documentModePageless)

	plain := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	err = runKong(t, &DriveUploadCmd{}, []string{plain, "--paged"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--paged applies only when creating a Google Doc") {
		t.Fatalf("paged on plain upload: %v", err)
	}
}

func assertDocumentMode(t *testing.T, body map[string]any, want string) {
	t.Helper()
	requests, _ := body["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", body["requests"])
	}
	req, _ := requests[0].(map[string]any)
	styleUpdate, _ := req["updateDocumentStyle"].(map[string]any)
	if styleUpdate["fields"] != documentModeFieldMask {
		t.Fatalf("fields = %#v", styleUpdate["fields"])
	}
	style, _ := styleUpdate["documentStyle"].(map[string]any)
	format, _ := style["documentFormat"].(map[string]any)
	if format["documentMode"] != want {
		t.Fatalf("documentMode = %#v, want %s", format["documentMode"], want)
	}
}
