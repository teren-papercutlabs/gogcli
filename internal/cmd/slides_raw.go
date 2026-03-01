package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// SlidesRawCmd passes a raw batchUpdate request directly to the Google Slides API.
// JSON is accepted via stdin (heredoc pattern), a file (@path), or inline string.
//
// Example (stdin):
//
//	gog slides raw PRESENTATION_ID - <<'EOF'
//	{"requests": [{"deleteObject": {"objectId": "SLIDE_ID"}}]}
//	EOF
type SlidesRawCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	JSON           string `arg:"" optional:"" name:"json" help:"batchUpdate JSON body (use - or @path to read from stdin/file)"`
}

func (c *SlidesRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	presentationID := normalizeGoogleID(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
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
		return usage(`JSON must contain a "requests" array (see https://developers.google.com/slides/api/reference/rest/v1/presentations/batchUpdate)`)
	}

	var requests []*slides.Request
	if err := json.Unmarshal(rawRequests, &requests); err != nil {
		return fmt.Errorf("parse requests: %w", err)
	}

	svc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"presentationId": resp.PresentationId,
			"replies":        resp.Replies,
		})
	}

	u.Out().Printf("batchUpdate applied to presentation %s (%d replies)", resp.PresentationId, len(resp.Replies))
	return nil
}
