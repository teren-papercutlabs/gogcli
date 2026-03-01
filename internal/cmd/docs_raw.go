package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DocsRawCmd passes a raw batchUpdate request directly to the Google Docs API.
// JSON is accepted via stdin (heredoc pattern), a file (@path), or inline string.
//
// Example (stdin):
//
//	gog docs raw DOC_ID - <<'EOF'
//	{"requests": [{"insertText": {"location": {"index": 1}, "text": "Hello"}}]}
//	EOF
type DocsRawCmd struct {
	DocID string `arg:"" name:"docId" help:"Document ID"`
	JSON  string `arg:"" optional:"" name:"json" help:"batchUpdate JSON body (use - or @path to read from stdin/file)"`
}

func (c *DocsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	docID := normalizeGoogleID(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}

	body, err := resolveRawJSONInput(c.JSON)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	rawRequests, ok := payload["requests"]
	if !ok {
		return usage(`JSON must contain a "requests" array (see https://developers.google.com/docs/api/reference/rest/v1/documents/batchUpdate)`)
	}

	var requests []*docs.Request
	if err := json.Unmarshal(rawRequests, &requests); err != nil {
		return fmt.Errorf("parse requests: %w", err)
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"replies":    resp.Replies,
		})
	}

	u.Out().Printf("batchUpdate applied to document %s (%d replies)", resp.DocumentId, len(resp.Replies))
	return nil
}
