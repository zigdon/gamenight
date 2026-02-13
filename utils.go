package main

import (
	"context"
	"fmt"
	"net/http"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
)

func tz() *time.Location {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(fmt.Sprintf("How do we not have a tz? %v", err))
	}
	return tz
}

func config(ctx context.Context, id string) string {
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

func loggedIn(w http.ResponseWriter, r *http.Request) (*User, error) {
    ctx := r.Context()
	email := r.Header.Get("X-Appengine-User-Email")
	if email == "" {
		http.Redirect(w, r, "/_ah/login?continue=/schedule", http.StatusFound)
		return nil, fmt.Errorf("Not logged in: %w", http.StatusTemporaryRedirect)
	}

    user, err := getUser(ctx, email)
    if err != nil {
        log.Printf("Error fetching user %s: %v", email, err)
        http.Error(w, "Error fetching user", http.StatusInternalServerError)
		return nil, fmt.Errorf("Error fetching user: %w", http.StatusInternalServerError)
    }

	return user, nil
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

func getInvite(ctx context.Context, k *datastore.Key) (Invitation, error) {
	var i invLoader
	if err := dsClient.Get(ctx, k, &i); err != nil {
		return Invitation{}, fmt.Errorf("error getting invite %v: %v", k, err)
	}
	inv := i.Convert()
	return inv, nil
}

func getAllInvitations(ctx context.Context, cutoff time.Time) ([]Invitation, error) {
	q := datastore.NewQuery("Invitation").FilterField("d", ">=", cutoff)
	var ils []invLoader
	ks, err := dsClient.GetAll(ctx, q, &ils)
	if err != nil {
		return nil, fmt.Errorf("error loading invitations: %v", err)
	}

	var invs []Invitation
	for n := range ks {
		il := ils[n]
		i := il.Convert()
		if err := i.Load(ctx); err != nil {
			return nil, fmt.Errorf("Error filling invite: %v", err)
		}
		invs = append(invs, i)
	}

	return invs, nil
}

func getNextGamenight(ctx context.Context) (*Gamenight, error) {
	now := time.Now().In(tz())
	q := datastore.NewQuery("Gamenight").
		FilterField("d", ">=",
		time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())).
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
		"2006-01-02 15:04",
		"2006-01-02, 15:04",
		"2006-01-02",
		"Jan 2 3pm",
		"Jan 2, 3pm",
		"Jan 2 3:04pm",
		"Jan 2, 3:04pm",
		"Jan 2 15:04",
		"Jan 2, 15:04",
		"Jan 2",
		"1/2 3pm",
		"1/2, 3pm",
		"1/2 3:04pm",
		"1/2, 3:04pm",
		"1/2 15:04",
		"1/2, 15:04",
		"1/2",
		// from here on we're really reaching (n=21)
		"Monday 3pm",
		"Monday, 3pm",
		"Monday 3:04pm",
		"Monday, 3:04pm",
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
		log.Printf("Failed to parse date %q as %q: %v", timespec, layout, err)
	}

	if found == -1 {
		return parsedTime, err
	}

	// If all we got was a weekday (and maybe a time), the date will parse
	// as 0000-01-01. In that case, we need to get the weekday requested, and
	// find the next one of those for the date.
	if (found >= 21) {
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
		parsedTime = parsedTime.Add(20*time.Hour)
	}

	// Default year is this year unless it's in the past
	now := time.Now().In(tz())
	if parsedTime.Year() == 0 {
		parsedTime = parsedTime.AddDate(now.Year(), 0, 0)
		if (parsedTime.Before(now)) {
			parsedTime = parsedTime.AddDate(1, 0, 0)
		}
	}
	return parsedTime, err
}

