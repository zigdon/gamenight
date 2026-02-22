package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

func tz() *time.Location {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(fmt.Sprintf("How do we not have a tz? %v", err))
	}
	return tz
}

func config(ctx context.Context, id string) string {
	if stageInstance && !strings.Contains(id, "_stage") {
		if cfg := config(ctx, id+"_stage"); cfg != "" {
			return cfg
		}
	}
	cQ := datastore.NewQuery("Config").FilterField("n", "=", id)
	var cfgs []Config
	_, err := dsClient.GetAll(ctx, cQ, &cfgs)
	if err != nil {
		log.Printf("config err: %v", err)
		return ""
	}
	if len(cfgs) > 0 {
		return cfgs[0].Value
	}
	log.Printf("No config found for %v", id)
	return ""
}

func getAllUsers(ctx context.Context) ([]User, error) {
	var users []User
	q := datastore.NewQuery("User")
	_, err := dsClient.GetAll(ctx, q, &users)
	if err != nil {
		return nil, fmt.Errorf("Error querying users: %v", err)
	}
	return users, nil
}

func getUser(ctx context.Context, email string) (*User, error) {
	user := &User{}
	key := datastore.NameKey("User", email, nil)
	err := dsClient.Get(ctx, key, user)
	if err != nil {
		user = &User{
			ID:   key,
			Name: strings.Split(email, "@")[0],
		}
		nk, err := dsClient.Put(ctx, user.ID, user)
		if err != nil {
			log.Printf("Error saving new user to Datastore: %v", err)
			return nil, fmt.Errorf("Couldn't created user")
		}

		log.Printf("New user created: %v", nk)
	}
	if strings.Contains(user.Name, "@") {
		user.Name = strings.Split(user.Name, "@")[0]
	}
	return user, nil
}

func getInvite(ctx context.Context, k *datastore.Key) (*Invitation, error) {
	var i invLoader
	if err := dsClient.Get(ctx, k, &i); err != nil {
		return nil, fmt.Errorf("error getting invite %v: %v", k, err)
	}
	inv := i.Convert()
	return &inv, nil
}

func getAllInvitations(ctx context.Context, starting time.Time, week bool) ([]*Invitation, error) {
	q := datastore.NewQuery("Invitation").FilterField("d", ">=", starting)
	if week {
		q = q.FilterField("d", "<=", time.Now().AddDate(0, 0, 7))
	}
	it := dsClient.Run(ctx, q)

	var invs []*Invitation
	for {
		var il invLoader
		k, err := it.Next(&il)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Error reading %v: %v", k, err)
		}
		i := il.Convert()
		if err := i.Load(ctx); err != nil {
			return nil, fmt.Errorf("Error filling invite: %v", err)
		}
		invs = append(invs, &i)
	}

	return invs, nil
}

func getNextGamenight(ctx context.Context) (*Gamenight, error) {
	now := time.Now().In(tz())
	q := datastore.NewQuery("Gamenight").
		FilterField("d", ">=", now.AddDate(0, 0, -1)).
		FilterField("s", "=", "Yes").
		Order("d").
		Limit(1)
	var gns []Gamenight
	if _, err := dsClient.GetAll(ctx, q, &gns); err != nil {
		return nil, fmt.Errorf("error getting next gamenight: %v", err)
	}
	if len(gns) == 0 {
		return nil, fmt.Errorf("no gamenight found")
	}
	log.Printf("Next gamenight: %s", gns[0].String())
	return &gns[0], nil
}

func parseTimespec(timespec string) (time.Time, error) {
	var parsedTime time.Time
	var err error
	// Try to parse as specific layout first, then more general ones.
	// Example: "Saturday, 8pm", "Oct 12, 7:30pm"
	layouts := []string{
		"2006-01-02 3pm",
		"2006-01-02, 3pm",
		"2006-01-02 3:04pm",
		"2006-01-02, 3:04pm",
		"2006-01-02 1504",
		"2006-01-02 15:04",
		"2006-01-02, 1504",
		"2006-01-02, 15:04",
		"2006-01-02",
		"Jan 2 3pm",
		"Jan 2, 3pm",
		"Jan 2 3:04pm",
		"Jan 2, 3:04pm",
		"Jan 2 1504",
		"Jan 2 15:04",
		"Jan 2, 15:04",
		"Jan 2",
		"1/2 3pm",
		"1/2, 3pm",
		"1/2 3:04pm",
		"1/2, 3:04pm",
		"1/2 1504",
		"1/2 15:04",
		"1/2, 15:04",
		"1/2",
		// from here on we're really reaching (n=25)
		"Monday 3pm",
		"Monday, 3pm",
		"Monday 3:04pm",
		"Monday, 3:04pm",
		"Monday 1504",
		"Monday 15:04",
		"Monday, 15:04",
		"Monday",
	}
	found := -1
	for n, layout := range layouts {
		parsedTime, err = time.ParseInLocation(layout, timespec, tz())
		if err == nil {
			log.Printf("Parsed date %q as %q: %s", timespec, layout, parsedTime)
			found = n
			break
		}
		// log.Printf("Failed to parse date %q as %q: %v", timespec, layout, err)
	}

	if found == -1 {
		return parsedTime, err
	}

	// If all we got was a weekday (and maybe a time), the date will parse
	// as 0000-01-01. In that case, we need to get the weekday requested, and
	// find the next one of those for the date.
	if found >= 25 {
		req := strings.ToLower(strings.Split(strings.Split(timespec, ",")[0], " ")[0])
		cur := time.Now().In(tz())
		cnt := 0
		log.Printf("Looking for the next %q", req)
		for strings.ToLower(cur.Weekday().String()) != req {
			if cnt > 7 {
				return parsedTime, fmt.Errorf("Can't figure out what you mean by %q", req)
			}
			cnt++
			cur = cur.AddDate(0, 0, 1)
		}
		log.Printf("Found %s", cur.Format("2006-01-02"))
		parsedTime = parsedTime.AddDate(cur.Year(), int(cur.Month())-1, cur.Day()-1)
		log.Printf("Parsed: %s", parsedTime.String())
	}

	if parsedTime.Hour() == 0 {
		parsedTime = parsedTime.Add(20 * time.Hour)
	}

	// Default year is this year unless it's in the past
	now := time.Now().In(tz())
	if parsedTime.Year() == 0 {
		parsedTime = parsedTime.AddDate(now.Year(), 0, 0)
		if parsedTime.Before(now) {
			parsedTime = parsedTime.AddDate(1, 0, 0)
		}
	}
	return parsedTime, err
}

func devServer(ctx context.Context) bool {
	return os.Getenv("GAE_ENV") != "standard"
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

	invs, err := getAllInvitations(ctx, now, true)
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
		if nextGN.Date.In(tz()).Before(next) {
			out("Next gamenight is before the cuttoff (%s): %s", next.Format("2006-01-02 15:04"), nextGN.String())
			next = nextGN.Date
		}
	}

	// 2. Discard any invites that are after a scheduled gamenight.
	byDate := make(map[string][]*Invitation)
	earliest := ""
	for _, i := range invs {
		// If it is already scheduled, no need to schedule it again.
		if i.Scheduled != nil {
			out("Skipping %s, as it is already scheduled", i.String())
			continue
		}
		if !i.When().Before(next) {
			out("Discarding %s, as it is after %s", i.String(), next)
			continue
		}
		out("Found %s", i.String())
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
	slices.SortStableFunc(invs, func(a, b *Invitation) int {
		return int(b.Priority - a.Priority)
	})
	for n, i := range invs {
		i.Load(ctx)
		out("%d.  %s\n", n, i.String())
	}

	// 5. Attempt to schedule the top invite.
	if err := invs[0].Schedule(ctx); err != nil {
		out("Failed to schedule: %v", err)
		return false, fmt.Errorf("couldn't schedule: %v", err)
	}
	return invs[0].Scheduled != nil, nil
}

func getSelectedUsers(ctx context.Context, pref userPreference) ([]*User, error) {
	it := dsClient.Run(ctx,
		datastore.NewQuery("User").FilterField(string(pref), "=", true))
	var users []*User
	for {
		var u User
		if _, err := it.Next(&u); err == iterator.Done {
			break
		} else if err != nil {
			return users, err
		}
		users = append(users, &u)
	}

	return users, nil
}
