package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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
