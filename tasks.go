package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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


func handleNag(w http.ResponseWriter, r *http.Request) {
	// Whatever happens, redirect back to root.
	defer func() {
		http.Redirect(w, r, "/", http.StatusFound)
	}()

	// Check auth.
	ctx := r.Context()
	_, err := cronOrAdmin(ctx, r)
	if err != nil {
        http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	if nextGN, err := getNextGamenight(ctx); err != nil {
		log.Printf("Couldn't find next gamenight: %v", err)
	} else if nextGN != nil {
		now := time.Now().In(tz())
		sun := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz()).AddDate(0,0,int(7-now.Weekday()))
		if nextGN.When().Before(sun) {
			log.Printf("No need to nag, gn is scheduled for %s", nextGN.When())
			return
		}
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		return
	}

	seq := r.FormValue("email")
	switch seq {
	case "first":
		err = email(ctx, firstNag, nil)
	case "second":
		err = email(ctx, secondNag, nil)
	default:
		err = fmt.Errorf("Bad nag call: %q", seq)
	}
	if err != nil {
		log.Printf("Error in nag task: %v", err)
	}
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
