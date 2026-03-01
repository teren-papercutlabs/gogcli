package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailRawCmd passes a raw request body directly to a Gmail API endpoint.
// This covers the Gmail REST operations that accept structured JSON input.
//
// Example — modify labels on a message:
//
//	gog gmail raw messages.modify MSG_ID - <<'EOF'
//	{"addLabelIds": ["STARRED"], "removeLabelIds": ["UNREAD"]}
//	EOF
//
// Example — batch-modify multiple messages:
//
//	gog gmail raw messages.batchModify - <<'EOF'
//	{"ids": ["MSG1", "MSG2"], "addLabelIds": ["STARRED"]}
//	EOF
type GmailRawCmd struct {
	Endpoint   string `arg:"" name:"endpoint" help:"API endpoint: messages.modify | messages.batchModify | messages.batchDelete | threads.modify | labels.create | labels.update"`
	ResourceID string `arg:"" optional:"" name:"resourceId" help:"Message, thread, or label ID (for per-resource endpoints)"`
	JSON       string `arg:"" optional:"" name:"json" help:"Request body JSON (use - or @path to read from stdin/file)"`
}

func (c *GmailRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	endpoint := strings.ToLower(strings.TrimSpace(c.Endpoint))

	// Determine JSON source: if resourceId looks like JSON or a stdin sentinel, treat it as the JSON arg.
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

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	switch endpoint {
	case "messages.modify":
		return gmailRawMessagesModify(ctx, flags, u, svc, resourceID, body)
	case "messages.batchmodify", "messages.batch-modify":
		return gmailRawMessagesBatchModify(ctx, u, svc, body)
	case "messages.batchdelete", "messages.batch-delete":
		return gmailRawMessagesBatchDelete(ctx, flags, u, svc, body)
	case "threads.modify":
		return gmailRawThreadsModify(ctx, u, svc, resourceID, body)
	case "labels.create":
		return gmailRawLabelsCreate(ctx, u, svc, body)
	case "labels.update":
		return gmailRawLabelsUpdate(ctx, u, svc, resourceID, body)
	default:
		return usage(fmt.Sprintf("unknown endpoint %q — supported: messages.modify, messages.batchModify, messages.batchDelete, threads.modify, labels.create, labels.update", c.Endpoint))
	}
}

func gmailRawMessagesModify(ctx context.Context, flags *RootFlags, u *ui.UI, svc *gmail.Service, messageID string, body []byte) error {
	if strings.TrimSpace(messageID) == "" {
		return usage("messages.modify requires a resourceId (message ID) argument")
	}
	messageID = normalizeGmailMessageID(messageID)

	var req gmail.ModifyMessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}

	if err := dryRunExit(ctx, flags, "gmail.messages.modify", map[string]any{
		"messageId":      messageID,
		"addLabelIds":    req.AddLabelIds,
		"removeLabelIds": req.RemoveLabelIds,
	}); err != nil {
		return err
	}

	msg, err := svc.Users.Messages.Modify("me", messageID, &req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":       msg.Id,
			"labelIds": msg.LabelIds,
		})
	}
	u.Out().Printf("modified message %s (labels: %s)", msg.Id, strings.Join(msg.LabelIds, ", "))
	return nil
}

func gmailRawMessagesBatchModify(ctx context.Context, u *ui.UI, svc *gmail.Service, body []byte) error {
	var req gmail.BatchModifyMessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}

	if err := svc.Users.Messages.BatchModify("me", &req).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"modified": req.Ids,
			"count":    len(req.Ids),
		})
	}
	u.Out().Printf("batch-modified %d messages", len(req.Ids))
	return nil
}

func gmailRawMessagesBatchDelete(ctx context.Context, flags *RootFlags, u *ui.UI, svc *gmail.Service, body []byte) error {
	var req gmail.BatchDeleteMessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}

	if confirmErr := confirmDestructive(ctx, flags, "permanently delete gmail messages"); confirmErr != nil {
		return confirmErr
	}

	if err := svc.Users.Messages.BatchDelete("me", &req).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted": req.Ids,
			"count":   len(req.Ids),
		})
	}
	u.Out().Printf("batch-deleted %d messages", len(req.Ids))
	return nil
}

func gmailRawThreadsModify(ctx context.Context, u *ui.UI, svc *gmail.Service, threadID string, body []byte) error {
	if strings.TrimSpace(threadID) == "" {
		return usage("threads.modify requires a resourceId (thread ID) argument")
	}

	var req gmail.ModifyThreadRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}

	thread, err := svc.Users.Threads.Modify("me", threadID, &req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id": thread.Id,
		})
	}
	u.Out().Printf("modified thread %s", thread.Id)
	return nil
}

func gmailRawLabelsCreate(ctx context.Context, u *ui.UI, svc *gmail.Service, body []byte) error {
	var label gmail.Label
	if err := json.Unmarshal(body, &label); err != nil {
		return fmt.Errorf("parse label: %w", err)
	}

	created, err := svc.Users.Labels.Create("me", &label).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":   created.Id,
			"name": created.Name,
		})
	}
	u.Out().Printf("created label %s (id: %s)", created.Name, created.Id)
	return nil
}

func gmailRawLabelsUpdate(ctx context.Context, u *ui.UI, svc *gmail.Service, labelID string, body []byte) error {
	if strings.TrimSpace(labelID) == "" {
		return usage("labels.update requires a resourceId (label ID) argument")
	}

	var label gmail.Label
	if err := json.Unmarshal(body, &label); err != nil {
		return fmt.Errorf("parse label: %w", err)
	}

	updated, err := svc.Users.Labels.Update("me", labelID, &label).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":   updated.Id,
			"name": updated.Name,
		})
	}
	u.Out().Printf("updated label %s (id: %s)", updated.Name, updated.Id)
	return nil
}
