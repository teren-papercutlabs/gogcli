package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// CalendarRawCmd passes a raw request body directly to a Calendar API endpoint.
// Useful for creating or updating events with the full API object shape.
//
// Example — create an event with full Calendar API JSON:
//
//	gog calendar raw events.insert CALENDAR_ID - <<'EOF'
//	{
//	  "summary": "Team Meeting",
//	  "start": {"dateTime": "2026-03-01T10:00:00+08:00"},
//	  "end":   {"dateTime": "2026-03-01T11:00:00+08:00"},
//	  "attendees": [{"email": "colleague@example.com"}]
//	}
//	EOF
//
// Example — update an event:
//
//	gog calendar raw events.update CALENDAR_ID EVENT_ID - <<'EOF'
//	{"summary": "Updated Title"}
//	EOF
type CalendarRawCmd struct {
	Endpoint   string `arg:"" name:"endpoint" help:"API endpoint: events.insert | events.update | events.patch | events.delete | calendars.insert | calendars.update"`
	CalendarID string `arg:"" optional:"" name:"calendarId" help:"Calendar ID (use 'primary' for primary calendar)"`
	EventID    string `arg:"" optional:"" name:"eventId" help:"Event ID (for per-event endpoints)"`
	JSON       string `arg:"" optional:"" name:"json" help:"Request body JSON (use - or @path to read from stdin/file)"`
}

func (c *CalendarRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	endpoint := strings.ToLower(strings.TrimSpace(c.Endpoint))

	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		calendarID = "primary"
	}
	eventID := strings.TrimSpace(c.EventID)

	// Detect if eventId or calendarId is actually the JSON arg (e.g., user omits eventId).
	jsonArg := c.JSON
	if jsonArg == "" && (eventID == "-" || strings.HasPrefix(eventID, "{") || strings.HasPrefix(eventID, "@")) {
		jsonArg = eventID
		eventID = ""
	}
	if jsonArg == "" && (calendarID == "-" || strings.HasPrefix(calendarID, "{") || strings.HasPrefix(calendarID, "@")) {
		jsonArg = calendarID
		calendarID = "primary"
	}

	body, err := resolveRawJSONInput(jsonArg)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	switch endpoint {
	case "events.insert":
		return calendarRawEventsInsert(ctx, flags, u, svc, calendarID, body)
	case "events.update":
		return calendarRawEventsUpdate(ctx, flags, u, svc, calendarID, eventID, body)
	case "events.patch":
		return calendarRawEventsPatch(ctx, flags, u, svc, calendarID, eventID, body)
	case "events.delete":
		return calendarRawEventsDelete(ctx, flags, u, svc, calendarID, eventID)
	case "calendars.insert":
		return calendarRawCalendarsInsert(ctx, u, svc, body)
	case "calendars.update":
		return calendarRawCalendarsUpdate(ctx, u, svc, calendarID, body)
	default:
		return usage(fmt.Sprintf("unknown endpoint %q — supported: events.insert, events.update, events.patch, events.delete, calendars.insert, calendars.update", c.Endpoint))
	}
}

func calendarRawEventsInsert(ctx context.Context, flags *RootFlags, u *ui.UI, svc *calendar.Service, calendarID string, body []byte) error {
	var event calendar.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("parse event: %w", err)
	}

	if err := dryRunExit(ctx, flags, "calendar.events.insert", map[string]any{
		"calendarId": calendarID,
		"summary":    event.Summary,
	}); err != nil {
		return err
	}

	created, err := svc.Events.Insert(calendarID, &event).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":          created.Id,
			"summary":     created.Summary,
			"htmlLink":    created.HtmlLink,
			"status":      created.Status,
		})
	}
	u.Out().Printf("created event %q (id: %s)", created.Summary, created.Id)
	if created.HtmlLink != "" {
		u.Out().Printf("link: %s", created.HtmlLink)
	}
	return nil
}

func calendarRawEventsUpdate(ctx context.Context, flags *RootFlags, u *ui.UI, svc *calendar.Service, calendarID, eventID string, body []byte) error {
	if eventID == "" {
		return usage("events.update requires eventId argument")
	}

	var event calendar.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("parse event: %w", err)
	}

	if err := dryRunExit(ctx, flags, "calendar.events.update", map[string]any{
		"calendarId": calendarID,
		"eventId":    eventID,
	}); err != nil {
		return err
	}

	updated, err := svc.Events.Update(calendarID, eventID, &event).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":       updated.Id,
			"summary":  updated.Summary,
			"htmlLink": updated.HtmlLink,
		})
	}
	u.Out().Printf("updated event %q (id: %s)", updated.Summary, updated.Id)
	return nil
}

func calendarRawEventsPatch(ctx context.Context, flags *RootFlags, u *ui.UI, svc *calendar.Service, calendarID, eventID string, body []byte) error {
	if eventID == "" {
		return usage("events.patch requires eventId argument")
	}

	var event calendar.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("parse event: %w", err)
	}

	if err := dryRunExit(ctx, flags, "calendar.events.patch", map[string]any{
		"calendarId": calendarID,
		"eventId":    eventID,
	}); err != nil {
		return err
	}

	patched, err := svc.Events.Patch(calendarID, eventID, &event).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":      patched.Id,
			"summary": patched.Summary,
		})
	}
	u.Out().Printf("patched event %q (id: %s)", patched.Summary, patched.Id)
	return nil
}

func calendarRawEventsDelete(ctx context.Context, flags *RootFlags, u *ui.UI, svc *calendar.Service, calendarID, eventID string) error {
	if eventID == "" {
		return usage("events.delete requires eventId argument")
	}

	if err := dryRunExit(ctx, flags, "calendar.events.delete", map[string]any{
		"calendarId": calendarID,
		"eventId":    eventID,
	}); err != nil {
		return err
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete calendar event %s", eventID)); confirmErr != nil {
		return confirmErr
	}

	if err := svc.Events.Delete(calendarID, eventID).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted": eventID,
		})
	}
	u.Out().Printf("deleted event %s", eventID)
	return nil
}

func calendarRawCalendarsInsert(ctx context.Context, u *ui.UI, svc *calendar.Service, body []byte) error {
	var cal calendar.Calendar
	if err := json.Unmarshal(body, &cal); err != nil {
		return fmt.Errorf("parse calendar: %w", err)
	}

	created, err := svc.Calendars.Insert(&cal).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":      created.Id,
			"summary": created.Summary,
		})
	}
	u.Out().Printf("created calendar %q (id: %s)", created.Summary, created.Id)
	return nil
}

func calendarRawCalendarsUpdate(ctx context.Context, u *ui.UI, svc *calendar.Service, calendarID string, body []byte) error {
	var cal calendar.Calendar
	if err := json.Unmarshal(body, &cal); err != nil {
		return fmt.Errorf("parse calendar: %w", err)
	}

	updated, err := svc.Calendars.Update(calendarID, &cal).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"id":      updated.Id,
			"summary": updated.Summary,
		})
	}
	u.Out().Printf("updated calendar %q (id: %s)", updated.Summary, updated.Id)
	return nil
}
