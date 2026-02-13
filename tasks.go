package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"
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
	admin, err := cronOrAdmin(ctx, r)
	if err != nil {
        http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	if _, err := maybeSchedule(ctx, w, admin); err != nil {
		log.Printf("Error scheduling gamenight: %v", err)
	}
}

func maybeSchedule(ctx context.Context, w http.ResponseWriter, debug bool) (bool, error) {
	out := func(t string, args ...any) {
		log.Printf(t, args...)
	}
	if debug {
		w.Header().Set("Content-Type", "text/plain")
		out = func(t string, args ...any) {
			log.Printf(t, args...)
			fmt.Fprintf(w, t+"\n", args...)
		}
	}

	// The plan:
	// 1. Examine invites up to the next scheduled GN, or the next Saturday, whichever is first.
	// 2. Discard any invites that are after a scheduled gamenight.
	// 3. Discard any for a date later than the nearest invite.
	// 4. Sort invites by priority.
	// 5. Attempt to schedule the top invite.

	// Execution:
	// 1. Examine all pending invites.
	now := time.Now().In(tz())

	invs, err := getAllInvitations(ctx, now)
	if err != nil {
		out("Failed to get invitations: %v", err)
		return false, err
	}
	if len(invs) == 0 {
		out("No pending invitations found")
		return false, nil
	}

	// Up to midnight of next saturday, or next GN date, whichever is first.
	next := time.Date(now.Year(), now.Month(), now.Day(),
	    0, 0, 0, 0, tz()).AddDate(0, 0, int(7-now.Weekday()))
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
		return false, nil
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

	// 5. Attempt to schedule the top invite.
	if err := invs[0].Schedule(ctx); err != nil {
		return false, fmt.Errorf("couldn't schedule: %v", err)
	}
	return invs[0].Scheduled != nil, nil
}
