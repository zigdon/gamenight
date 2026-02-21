package main

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
)

// New type for InviteData to include messages and form values for re-rendering
type InviteData struct {
	Base        BaseTemplate
	Invitations []*Invitation
	When        string
	Where       string
	Notes       string
	Priority    string
	Checks      map[string]string
	ParsedTime  time.Time
	Scheduled   bool
}

func createInvite(r *http.Request, data *InviteData, user *User) error {
	whenStr := r.FormValue("when")
	whereStr := r.FormValue("where")
	notesStr := r.FormValue("notes")
	priorityStr := r.FormValue("priority")

	data.When = whenStr
	data.Where = whereStr
	data.Notes = notesStr
	data.Priority = priorityStr

	// Basic validation
	if whenStr == "" {
		data.Base.Error = "When do you want to host?"
	}
	if whereStr == "" {
		data.Base.Error = "Where do you want to host?"
	}
	if priorityStr == "" {
		data.Base.Error = "Gotta have a priority."
	}

	if data.Base.Error != "" {
		return fmt.Errorf("Form error: %v", data.Base.Error)
	}

	parsedTime, err := parseTimespec(whenStr)
	if err != nil {
		data.Base.Error = fmt.Sprintf("Not sure what you mean by \"%s\"", whenStr)
		return fmt.Errorf("failed to parse timespec: %v", err)
	}

	// Create an Invitation entity
	invite := &Invitation{
		Key: datastore.IncompleteKey("Invitation", nil),
		Date:     parsedTime,
		Time:     parsedTime,
		Location: whereStr,
		Notes:    notesStr,
		Priority: PriorityUndefined,

		OwnerKey:    user.ID,
	}

	// Parse Priority string to Priority enum
	switch priorityStr {
	case "Can":
		invite.Priority = PriorityCan
	case "Want":
		invite.Priority = PriorityWant
	case "Insist":
		invite.Priority = PriorityInsist
	default:
		data.Base.Error = "Invalid priority value."
		return fmt.Errorf("failed to parse priority: %v", priorityStr)
	}

	if parsedTime.Before(time.Now()) {
		data.Base.Error = "Can't create an invitation in the past!"
		return fmt.Errorf("refused to create invitation in the past: %s", parsedTime)
	}

	// Save to Datastore
	ctx := r.Context()
	nk, err := dsClient.Put(ctx, invite.Key, invite)
	if err != nil {
		data.Base.Error = "Error saving invitation!"
		return fmt.Errorf("failed to save invite: %v", err)
	}
	invite.Key = nk

	log.Printf("Created invitation (%v): from %s for %s @ %s (Priority: %v)",
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

	// Attempt to warn about odd invitations.
	// Expected: start 5pm-10pm, Saturday, in the future, within 30 days.
	checks := make(map[string]string)
	if parsedTime.Weekday() != 6 {  // Not saturday
		checks["wd"] = fmt.Sprintf("for %s, not Saturday", parsedTime.Weekday().String())
	}
	if parsedTime.Hour() < 17 || parsedTime.Hour() >= 22 {
		checks["hr"] = fmt.Sprintf("starting at %s", parsedTime.Format("3pm"))
	}
	if time.Until(parsedTime) > 30*24*time.Hour {
		checks["f"] = fmt.Sprintf("%.0f days in the future", time.Until(parsedTime).Round(24*time.Hour).Hours()/24)
	}
	data.Checks = checks
	data.Base.Msg = fmt.Sprintf("Invitation created for %s%s!",
		parsedTime.Format("Monday, January 2"), suf)
	data.Invitations = append(data.Invitations, invite)
	
	return nil
}

func withdrawInvite(r *http.Request, data *InviteData, user *User) error {
	id, err := strconv.Atoi(r.FormValue("withdraw"))
	if err != nil {
		data.Base.Error = "Invalid request"
		return fmt.Errorf("invalid withdraw ID: %v", r.FormValue("withdraw"))
	}
	ctx := r.Context()
	inv := &Invitation{}
	key := datastore.IDKey("Invitation", int64(id), nil)
	err = dsClient.Get(ctx, key, inv)
	if err != nil {
		data.Base.Error = "Invalid request"
		return fmt.Errorf("failed to find invite %v: %v", key, err)
	}
	if !inv.OwnerKey.Equal(user.ID) && !user.Superuser {
		data.Base.Error = "Invalid request"
		return fmt.Errorf("%v not owner of %#v", user, inv)
	}
	gn, err := inv.GetGamenight(ctx)
	if err != nil {
		log.Printf("Error finding a gamenight from invite %v: %v", key, err)
		data.Base.Error = "Failed to cancel gamenight"
		return fmt.Errorf("Can't withdraw gn")
	}
	if gn != nil {
		gn.Load(ctx)
		desc := gn.String()
		if err := gn.Delete(ctx); err != nil {
			data.Base.Error = "Couldn't delete gamenight event"
			return fmt.Errorf("Error deleting gamenight %s: %v", desc, err)
		} else {
			log.Printf("Deleted gn: %s", desc)
		}
	}
	if err := dsClient.Delete(ctx, key); err != nil {
		data.Base.Error = "Failed to withdraw invite"
		return fmt.Errorf("error deleting invite %#v: %v", inv, err)
	}
	data.Base.Msg = "Invitation withdrawn"
	log.Printf("Invitation %s withdrawn by %s", inv.String(), user.ID)
	return nil
}

func handleInvite(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
	user, _ := getUserSession(ctx, r)

	invs, err := getAllInvitations(ctx, time.Now(), false)
	if err != nil {
		log.Printf("query err: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := &InviteData{
		Base: BaseTemplate{
			Tab: "invite",
			User: user,
			DevServer: devServer(ctx),
		},
		Invitations: invs,
		Checks: make(map[string]string),
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		data.Base.Error = fmt.Sprintf("Error parsing form: %v", err)
		return
	}
	log.Printf("%v", r.Form)

	// Parse any warnings from the query
	// wd - weekday
	// hr - hour
	// f - future
	for _, k := range []string{"wd", "hr", "f"} {
		v := r.FormValue(k)
		if v == "" {
			continue
		}
		data.Checks[k] = v
	}
	if r.FormValue("sched") != "" {
		data.Scheduled = true
	}

	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/invite.html"))
	defer func() {
		err := tmpl.ExecuteTemplate(w, "invite.html", data)
		if err != nil {
			log.Printf("Error executing invite.html: %v", err)
		}
	}()

	if r.Method == http.MethodPost {
		if (r.FormValue("withdraw") != "") {
			if err := withdrawInvite(r, data, user); err != nil {
				log.Printf("Error withdrawing invitation: %v", err)
				return
			}
		} else {
			if err := createInvite(r, data, user); err != nil {
				log.Printf("Error creating invitation: %v", err)
				return
			}

			// Try and immediately schedule, if appropriate.
			if added, err := maybeSchedule(ctx, w, false); err != nil {
				log.Printf("Couldn't run instant-schedule: %v", err)
			} else if added {
				data.Checks["sched"] = "yes"
			}
		}
		if err != nil {
			log.Printf("Error processing POST: %v", err)
			return
		}

        // Redirect to clear form
		params := []string{"/invite?msg="+url.QueryEscape(data.Base.Msg)}
		for k, v := range data.Checks {
			params = append(params, k+"="+url.QueryEscape(v))
		}
        http.Redirect(w, r, strings.Join(params, "&"), http.StatusFound)
        return
	}

    // Check for message in URL query parameters (after redirect)
    if msg := r.URL.Query().Get("msg"); msg != "" {
        data.Base.Msg = html.EscapeString(msg)
    }
}

