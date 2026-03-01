package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// SheetsRawCmd passes a raw batchUpdate request directly to the Google Sheets API.
// JSON is accepted via stdin (heredoc pattern), a file (@path), or inline string.
//
// Example (stdin):
//
//	gog sheets raw SPREADSHEET_ID - <<'EOF'
//	{"requests": [{"addSheet": {"properties": {"title": "NewSheet"}}}]}
//	EOF
type SheetsRawCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	JSON          string `arg:"" optional:"" name:"json" help:"batchUpdate JSON body (use - or @path to read from stdin/file)"`
}

func (c *SheetsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	body, err := resolveRawJSONInput(c.JSON)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	// Parse into a generic map so we can extract "requests" and pass through verbatim.
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// "requests" is required for batchUpdate.
	rawRequests, ok := payload["requests"]
	if !ok {
		return usage(`JSON must contain a "requests" array (see https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets/batchUpdate)`)
	}

	// Unmarshal into typed requests slice.
	var requests []*sheets.Request
	if err := json.Unmarshal(rawRequests, &requests); err != nil {
		return fmt.Errorf("parse requests: %w", err)
	}

	bur := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	// Optional: include spreadsheet in response.
	if raw, ok := payload["includeSpreadsheetInResponse"]; ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err == nil {
			bur.IncludeSpreadsheetInResponse = v
		}
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, bur).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"replies":       resp.Replies,
		})
	}

	u.Out().Printf("batchUpdate applied to %s (%d replies)", resp.SpreadsheetId, len(resp.Replies))
	return nil
}

// resolveRawJSONInput reads JSON from a value, stdin sentinel, @file, or raw stdin pipe.
func resolveRawJSONInput(spec string) ([]byte, error) {
	if spec != "" {
		return resolveInlineOrFileBytes(spec)
	}
	// No arg provided — check if stdin has data (piped).
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return b, nil
	}
	return nil, usage("provide JSON via argument, @file, - (stdin), or pipe")
}
