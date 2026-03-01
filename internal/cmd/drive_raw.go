package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveRawCmd passes a raw request body directly to a Drive API endpoint.
// Useful for metadata updates that the higher-level commands don't expose.
//
// Example — update file metadata:
//
//	gog drive raw files.update FILE_ID - <<'EOF'
//	{"name": "New Name", "description": "Updated description"}
//	EOF
//
// Example — create file metadata:
//
//	gog drive raw files.create - <<'EOF'
//	{"name": "New Folder", "mimeType": "application/vnd.google-apps.folder", "parents": ["PARENT_ID"]}
//	EOF
type DriveRawCmd struct {
	Endpoint   string `arg:"" name:"endpoint" help:"API endpoint: files.update | files.create | files.copy | permissions.create"`
	ResourceID string `arg:"" optional:"" name:"resourceId" help:"File ID (for per-resource endpoints)"`
	JSON       string `arg:"" optional:"" name:"json" help:"Request body JSON (use - or @path to read from stdin/file)"`
}

func (c *DriveRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	endpoint := strings.ToLower(strings.TrimSpace(c.Endpoint))

	// Detect if resourceId is actually the JSON arg.
	jsonArg := c.JSON
	resourceID := c.ResourceID
	if jsonArg == "" && (resourceID == "-" || strings.HasPrefix(resourceID, "{") || strings.HasPrefix(resourceID, "@")) {
		jsonArg = resourceID
		resourceID = ""
	}

	body, err := resolveRawJSONInput(jsonArg)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	switch endpoint {
	case "files.update":
		return driveRawFilesUpdate(ctx, flags, u, svc, resourceID, body)
	case "files.create":
		return driveRawFilesCreate(ctx, u, svc, body)
	case "files.copy":
		return driveRawFilesCopy(ctx, u, svc, resourceID, body)
	case "permissions.create":
		return driveRawPermissionsCreate(ctx, u, svc, resourceID, body)
	default:
		return usage(fmt.Sprintf("unknown endpoint %q — supported: files.update, files.create, files.copy, permissions.create", c.Endpoint))
	}
}

func driveRawFilesUpdate(ctx context.Context, flags *RootFlags, u *ui.UI, svc *drive.Service, fileID string, body []byte) error {
	fileID = normalizeGoogleID(strings.TrimSpace(fileID))
	if fileID == "" {
		return usage("files.update requires a resourceId (file ID) argument")
	}

	var meta drive.File
	if err := json.Unmarshal(body, &meta); err != nil {
		return fmt.Errorf("parse file metadata: %w", err)
	}

	if err := dryRunExit(ctx, flags, "drive.files.update", map[string]any{
		"fileId": fileID,
		"name":   meta.Name,
	}); err != nil {
		return err
	}

	updated, err := svc.Files.Update(fileID, &meta).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":          updated.Id,
			"name":        updated.Name,
			"mimeType":    updated.MimeType,
			"webViewLink": updated.WebViewLink,
		})
	}
	u.Out().Printf("updated file %s (id: %s)", updated.Name, updated.Id)
	return nil
}

func driveRawFilesCreate(ctx context.Context, u *ui.UI, svc *drive.Service, body []byte) error {
	var meta drive.File
	if err := json.Unmarshal(body, &meta); err != nil {
		return fmt.Errorf("parse file metadata: %w", err)
	}

	created, err := svc.Files.Create(&meta).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":          created.Id,
			"name":        created.Name,
			"mimeType":    created.MimeType,
			"webViewLink": created.WebViewLink,
		})
	}
	u.Out().Printf("created %s (id: %s)", created.Name, created.Id)
	return nil
}

func driveRawFilesCopy(ctx context.Context, u *ui.UI, svc *drive.Service, fileID string, body []byte) error {
	fileID = normalizeGoogleID(strings.TrimSpace(fileID))
	if fileID == "" {
		return usage("files.copy requires a resourceId (file ID) argument")
	}

	var meta drive.File
	if err := json.Unmarshal(body, &meta); err != nil {
		return fmt.Errorf("parse file metadata: %w", err)
	}

	copied, err := svc.Files.Copy(fileID, &meta).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":          copied.Id,
			"name":        copied.Name,
			"mimeType":    copied.MimeType,
			"webViewLink": copied.WebViewLink,
		})
	}
	u.Out().Printf("copied to %s (id: %s)", copied.Name, copied.Id)
	return nil
}

func driveRawPermissionsCreate(ctx context.Context, u *ui.UI, svc *drive.Service, fileID string, body []byte) error {
	fileID = normalizeGoogleID(strings.TrimSpace(fileID))
	if fileID == "" {
		return usage("permissions.create requires a resourceId (file ID) argument")
	}

	var perm drive.Permission
	if err := json.Unmarshal(body, &perm); err != nil {
		return fmt.Errorf("parse permission: %w", err)
	}

	created, err := svc.Permissions.Create(fileID, &perm).
		SupportsAllDrives(true).
		Fields("id, role, type, emailAddress").
		Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":           created.Id,
			"role":         created.Role,
			"type":         created.Type,
			"emailAddress": created.EmailAddress,
		})
	}
	u.Out().Printf("created permission id=%s role=%s type=%s", created.Id, created.Role, created.Type)
	return nil
}
