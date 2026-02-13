package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"cloud.google.com/go/datastore"
)

// Check if the request is coming from cron, or from an admin. Returns an
// error if neither, true if admin, false is cron.
func cronOrAdmin(ctx context.Context, r *http.Request) (bool, error) {
	if r.Header.Get("X-Appengine-Cron") == "true" {
		log.Printf("Authorized request from cron")
		return false, nil
	}

	email := r.Header.Get("X-Appengine-User-Email")
	if email == "" {
		log.Printf("Not logged in, unauthorized")
        return false, fmt.Errorf("Not authorized")
    }

    user, err := getUser(ctx, email)
    if err != nil {
        return false, fmt.Errorf("error fetching user %q: %v", email, err)
    }

	if !user.Superuser {
		return false, fmt.Errorf("%s is not an admin", email)
	}

	return true, nil
}

func handleTaskSchedule(w http.ResponseWriter, r *http.Request) {
	// Whatever happens, redirect back to the schedule page.
	defer func() {
		http.Redirect(w, r, "/schedule", http.StatusFound)
	}()

	// Check auth.
	ctx := r.Context()
	out := func(t string, args ...any) {
		log.Printf(t, args...)
	}
	admin, err := cronOrAdmin(ctx, r)
	if err != nil {
        http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	if admin {
		w.Header().Set("Content-Type", "text/plain")
		out = func(t string, args ...any) {
			log.Printf(t, args...)
			fmt.Fprintf(w, t, args...)
		}
	}

	// The plan:
	// 1. Examine invites up to the next scheduled GN, or the next Saturday, whichever is first.
	// 2. Discard any invites that are after a scheduled gamenight.
	// 3. Discard any for a date later than the nearest invite.
	// 4. Sort invites by priority.
	// 5. If the top invite is not for Saturday, schedule it.
	// 6. If it is for Saturday, and it's high priority, or today is at least Tuesday, schedule it.
	// 7. Otherwise, do nothing.

	// Execution:
	// 1. Examine all pending invites.
	now := time.Now()

	invs, err := getAllInvitations(ctx, now)
	if err != nil {
		out("Failed to get invitations: %v", err)
		return
	}
	if len(invs) == 0 {
		out("No pending invitations found")
		return
	}

	// Up to midnight of next saturday, or next GN date, whichever is first.
	next := time.Date(now.Year(), now.Month(), now.Day(),
	    0, 0, 0, 0, now.Location()).AddDate(0, 0, int(7-now.Weekday()))
	if nextGN, err := getNextGamenight(ctx); err != nil {
		out("Couldn't find next gamenight: %v", err)
	} else {
		if nextGN.Date.Before(next) {
			out("Next gamenight is before saturday: %s", nextGN.Date.Format("Monday, 2006-01-02"))
			next = nextGN.Date
		}
	}

	// 2. Discard any invites that are after a scheduled gamenight.
	byDate := make(map[string][]Invitation)
	earliest := ""
	for _, i := range invs {
		if i.When().After(next) {
			out("Discarding %s, as it is after %s", i.String(), next)
			continue
		}
		date := i.When().Format("2006-01-02")
		byDate[date] = append(byDate[date], i)
		if earliest == "" || date < earliest {
			earliest = date
		}
	}

	// No invites to consider, give up.
	if earliest == "" {
		out("No invitations found before %s, aborting scheduling", next)
		return
	}

	// 3. Discard any for a date later than the nearest invite.
	out("Found %d (of %d) invitations for %s:\n", len(byDate[earliest]), len(invs), earliest)
	invs = byDate[earliest]

	// 4. Sort invites by priority.
	slices.SortStableFunc(invs, func(a, b Invitation) int {
		return int(b.Priority - a.Priority)
	})
	for n, i := range invs {
		i.Load(ctx)
		out("%d.  %s\n", n, i.String())
	}
	i := invs[0]

	// TODO: If there's a tie in date/priority, prefer the person who hasn't
	// hosted recently. For now, let's assume that's not a problem.

	// 5. If the top invite is not for Saturday, schedule it.
	// 6. If it is for Saturday, and it's high priority, or today is at least Tuesday, schedule it.
	if i.Date.Weekday() == time.Saturday {
		if i.Priority == PriorityCan && now.Weekday() < time.Tuesday {
			out("Today is %s, not scheduling 'Can' for Saturday yet", now.Weekday())
			return
		}
	}

	out("Scheduling %s", i.String())

	gn := &Gamenight{
		ID: datastore.IncompleteKey("Gamenight", nil),
		Status: "Yes",
		LastUpdate: now,
		Date: i.When(),
		Time: i.When(),  // Redundant and obsolete, but keep filling it for now.
		Location: i.Location,
		Notes: i.Notes,
		owner: i.OwnerKey,
		invite: i.Key,
	}

	nk, err := dsClient.Put(ctx, gn.ID, gn)
	out("Created entry: %v", nk)

	eid, err := gn.CreateEvent(ctx)
	if err != nil {
		out("Failed to create new event: %v", err)
		return
	}

	out("Created event: %v", eid)

	// TODO: send out notifications to those who asked.

}
