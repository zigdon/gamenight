package main

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"net/url"

	"cloud.google.com/go/datastore"
)

var (
	dsClient *datastore.Client
	tmpl     = template.Must(template.ParseGlob("templates/*.html"))
)

func main() {
	ctx := context.Background()
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Printf("ERROR: environment variable GOOGLE_CLOUD_PROJECT not set. Datastore client may not initialize correctly.")
		// We can try to proceed without it, but Datastore operations might fail.
	} else {
        log.Printf("Using Google Cloud Project ID: %s", projectID)
    }

	var err error
	dsClient, err = datastore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
    log.Printf("Datastore client initialized successfully.")

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/invite", handleInvite)
	http.HandleFunc("/profile", handleProfile)
	http.HandleFunc("/debug", handleDebug)
	//http.HandleFunc("/schedule", nil)
	//http.HandleFunc("/tasks/nag", nil)
	//http.HandleFunc("/tasks/reset", nil)
	//http.HandleFunc("/tasks/schedule", nil)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleDebug(w http.ResponseWriter, r *http.Request) {
	invQ := datastore.NewQuery("Invitation").
	    FilterField("d", ">", time.Now()).
		Order("d").
		Limit(4)

	var invs []Invitation
		_, err := dsClient.GetAll(r.Context(), invQ, &invs)
	if err != nil {
		log.Printf("query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, i := range invs {
		fmt.Fprintf(w, "%#v\n", i)
	}
}


func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/_ah/logout?continue=/", http.StatusFound)
}

type ProfileData struct {
	User    *User
	Profile *User
	Users   []*User
	Tab     string
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	email := r.Header.Get("X-Appengine-User-Email")
	if email == "" {
		http.Redirect(w, r, "/_ah/login?continue=/profile", http.StatusFound)
		return
	}

	// For now, we'll just create a dummy user object.
	// Later, we'll need to load the user from the datastore.
	user := &User{
		ID:   email,
		Name: email,
	}

	data := &ProfileData{
		User:    user,
		Profile: user,
		Tab:     "profile",
	}

	err := tmpl.ExecuteTemplate(w, "profile.html", data)
	if err != nil {
		log.Printf("Error executing profile.html: %v", err)
	}
}

func getUser(ctx context.Context, email string) (*User, error) {
	uQ := datastore.NewQuery("User").FilterField("n", "=", email).Limit(1)
	var users []*User
	_, err := dsClient.GetAll(ctx, uQ, &users)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if len(users) == 0 {
		return nil, nil // User not found
	}
	u := users[0]
	if (u.ID == u.Name) {
		u.Name = strings.Split(u.ID, "@")[0]
	}
	return u, nil
}

// New type for InviteData to include messages and form values for re-rendering
type InviteData struct {
	Tab         string
	User        *User
	Invitations []Invitation
	When        string
	Where       string
	Notes       string
	Priority    string
	Error       string
	Msg         string
	ParsedTime  time.Time
}


func handleInvite(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
	email := r.Header.Get("X-Appengine-User-Email")
	if email == "" {
		http.Redirect(w, r, "/_ah/login?continue=/invite", http.StatusFound)
		return
	}

    // Attempt to get the user from datastore
    user, err := getUser(ctx, email)
    if err != nil {
        log.Printf("Error fetching user %s: %v", email, err)
        http.Error(w, "Error fetching user", http.StatusInternalServerError)
        return
    }
    if user == nil {
        // User not found in Datastore, create a new one.
        user = &User{ID: email, Name: strings.Split(email, "@")[0]}
    }

	var invs []Invitation
	invQ := datastore.NewQuery("Invitation").
	    FilterField("d", ">", time.Now()).
		Order("d")

	_, err = dsClient.GetAll(ctx, invQ, &invs)
	if err != nil {
		log.Printf("query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := &InviteData{
		Tab:  "invite",
		User: user,
		Invitations: invs,
	}

	if r.Method == http.MethodPost {
		log.Printf("Handling POST")
		// Parse form data
		if err := r.ParseForm(); err != nil {
			data.Error = fmt.Sprintf("Error parsing form: %v", err)
			tmpl.ExecuteTemplate(w, "invite.html", data)
			return
		}

		log.Printf("Form parsed")
		whenStr := r.FormValue("when")
		whereStr := r.FormValue("where")
		notesStr := r.FormValue("notes")
		priorityStr := r.FormValue("priority")

		data.When = whenStr
		data.Where = whereStr
		data.Notes = notesStr
		data.Priority = priorityStr
		log.Printf("parsed data: %v", data)

		// Basic validation
		if whenStr == "" {
			data.Error = "When do you want to host?"
		}
		if whereStr == "" {
			data.Error = "Where do you want to host?"
		}
		if priorityStr == "" {
			data.Error = "Gotta have a priority."
		}

		if data.Error != "" {
			log.Printf("Form error: %v", data.Error)
			tmpl.ExecuteTemplate(w, "invite.html", data)
			return
		}
		log.Printf("Passed validation")

		// --- Date/Time Parsing (Placeholder, will be enhanced) ---
		var parsedTime time.Time
		var parseErr error
        // Try to parse as specific layout first, then more general ones.
        // Example: "Saturday, 8pm", "Oct 12, 7:30pm"
        // For now, let's assume a simple layout. Need more robust parsing.
        layouts := []string{
			"2006-01-02 3pm",              // 0: YYYY-MM-DD HHpm
			"2006-01-02 15:04",            // 1: YYYY-MM-DD HH:mm
			"2006-01-02",                  // 2: YYYY-MM-DD
			"Jan 2nd 15:04",               // 3: mmm DDnd HH:mm
			"Jan 2nd",                     // 4: mmm DDnd
			"1/02 15:04",                  // 5: MM/DD HH:mm
			"1/02 3pm",                    // 6: MM/DD HHpm
			"1/02",                        // 7: MM/DD
        }
        for _, layout := range layouts {
            parsedTime, parseErr = time.Parse(layout, whenStr)
            if parseErr == nil {
				log.Printf("Parsed date %q as %q: %s", whenStr, layout, parsedTime)
                break
            }
			log.Printf("Failed to parse date %q as %q: %v", whenStr, layout, parseErr)
        }

		if parseErr != nil {
			log.Printf("Giving up on parsing %q", whenStr)
			data.Error = fmt.Sprintf("Not sure what you mean by \"%s\"", whenStr)
			tmpl.ExecuteTemplate(w, "invite.html", data)
			return
		}

		log.Printf("Parsed date: %s", parsedTime.Format("2006-01-02 15:04"))

		if parsedTime.Hour() == 0 {
			parsedTime = parsedTime.Add(20*time.Hour)
		}

		// Default year is this year unless it's in the past
		now := time.Now()
		if parsedTime.Year() == 0 {
			parsedTime = parsedTime.AddDate(now.Year(), 0, 0)
			if (parsedTime.Before(now)) {
				parsedTime = parsedTime.AddDate(1, 0, 0)
			}
		}
		log.Printf("Filled date: %s", parsedTime.Format("2006-01-02 15:04"))

		// --- Datastore operations ---
        // Create an Invitation entity
        invite := Invitation{
			Key: datastore.IncompleteKey("Invitation", nil),
            Date:     parsedTime.UTC(),
            Time:     parsedTime.UTC(),
            Owner:    datastore.NameKey("User", user.Name, nil),
            Location: whereStr,
            Notes:    notesStr,
            Priority: PriorityUndefined,
        }

        // Parse Priority string to Priority enum
		log.Printf("Parsing priority")
        switch priorityStr {
        case "Can":
            invite.Priority = PriorityCan
        case "Want":
            invite.Priority = PriorityWant
        case "Insist":
            invite.Priority = PriorityInsist
        default:
            data.Error = "Invalid priority value."
            tmpl.ExecuteTemplate(w, "invite.html", data)
            return
        }

        // Save to Datastore
        log.Printf("Attempting to save invitation to Datastore for user %s", user.Name)
		nk, err := dsClient.Put(ctx, invite.Key, &invite)
        if err != nil {
            log.Printf("Error saving invitation to Datastore: %v", err) // Log the error
            data.Error = fmt.Sprintf("Error saving invitation: %v", err)
            tmpl.ExecuteTemplate(w, "invite.html", data)
            return
        }

        log.Printf("Successfully invitation (%v): from %s for %s @ %s (Priority: %v)",
            nk, user.Name, invite.When().Format("Mon, Jan 2 15:04"),
			invite.Location, invite.Priority)
        
        data.ParsedTime = parsedTime // Store parsed time in data struct
		suf := "th"
		if parsedTime.Day() == 1 {
			suf = "st"
		} else if parsedTime.Day() == 2 {
			suf = "nd"
		} else if parsedTime.Day() == 3 {
			suf = "rd"
		}
        data.Msg = fmt.Sprintf("Invitation created for %s%s!",
		    parsedTime.Format("Monday, January 2"), suf)
        // Redirect to clear form
        http.Redirect(w, r, "/invite?msg="+url.QueryEscape(data.Msg), http.StatusFound)
        return
	}

    // Check for message in URL query parameters (after redirect)
    if msg := r.URL.Query().Get("msg"); msg != "" {
        data.Msg = html.EscapeString(msg)
    }
	err = tmpl.ExecuteTemplate(w, "invite.html", data)
	if err != nil {
		log.Printf("Error executing invite.html: %v", err)
	}
}

func mkDefault() Gamenight {
	// Before tuesday, say probably. 
	// Before friday, say maybe.
	// Then say no.
	switch (time.Now().Weekday()) {
	case time.Sunday, time.Monday, time.Tuesday:
		return Gamenight{Status: "Probably"}
	case time.Wednesday, time.Thursday, time.Friday:
		return Gamenight{Status: "Maybe"}
	default:
		return Gamenight{Status: "No"}
	}
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

func handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var sched Gamenight
	var gns []Gamenight
	found := make(map[string]bool)
	schQ := datastore.NewQuery("Gamenight").
		FilterField("s", "=", "Yes").
		FilterField("d", ">", time.Now().AddDate(0,0,-2)).
		Order("d")

	_, err := dsClient.GetAll(ctx, schQ, &gns)
	if err != nil {
		log.Printf("sched query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if (len(gns) == 0) {
		sched = mkDefault()
	} else {
		sched = gns[0]
		found[fmt.Sprintf("%s@%s", sched.Owner.Name, sched.When())] = true
	}

	gnQ := datastore.NewQuery("Gamenight").
		FilterField("d", ">", time.Now().AddDate(0,0,-2)).
		Order("d")

	gns = []Gamenight{}
	_, err = dsClient.GetAll(ctx, gnQ, &gns)
	if err != nil {
		log.Printf("gns query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var invs []Invitation
	invQ := datastore.NewQuery("Invitation").
	    FilterField("d", ">", time.Now()).
		Order("d").
		Limit(4)

	_, err = dsClient.GetAll(ctx, invQ, &invs)
	if err != nil {
		log.Printf("query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Future struct {
		Type string
		When time.Time
		Location string
		Owner string
	}

	type IndexData struct {
		Current Gamenight
		Future []Future
		Updated time.Time
		CalendarID string
	}

	var days []Future
	for _, gn := range gns {
		if (gn.ID == sched.ID) {
			continue
		}
		o := "N/A"
		if gn.Owner != nil {
			o = gn.Owner.Name
		}
		days = append(days, Future{
			Type: "gamenight",
			When: gn.When(),
			Location: gn.Location,
			Owner: o,
		})
		if gn.Invite != nil {
			found[fmt.Sprintf("%s@%s", gn.Owner.Name, gn.When())] = true
		}
	}
	for _, inv := range invs {
		if _, ok := found[fmt.Sprintf("%s@%s", inv.Owner.Name, inv.When())]; ok {
			continue
		}
		days = append(days, Future{
			Type: "invite",
			When: inv.When(),
			Location: inv.Location,
			Owner: inv.Owner.Name,
		})
	}

	var data = IndexData{
		Current: sched,
		Future: days,
		Updated: time.Now(),
		CalendarID: config(ctx, "calendar_id"),
	}
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		log.Printf("Error executing index: %v", err)
	}
}
