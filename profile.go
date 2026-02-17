package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
)
type ProfileData struct {
	Base    BaseTemplate
	Profile *User
	Users   []User
	Highlight string
}

func updateProfile(r *http.Request, data *ProfileData, user *User) error {
	ctx := r.Context()
	form := r.Form
	var id string
	if user.Superuser && len(form["pid"]) > 0 {
		id = form["pid"][0]
	} else {
		id = user.ID.Name
	}

	if user.ID.Name != id && !user.Superuser {
		return fmt.Errorf("User %v can't edit %v", user, id)
	}

	profile, err := getUser(ctx, id)
	if err != nil {
		return fmt.Errorf("Can't find user %v: %v", id, err)
	}

	// Make sure the form is filled out
	for _, f := range []string{"name", "location"} {
		if len(form[f]) == 0 {
			return fmt.Errorf("Missing form field %s", f)
		}
	}

	var changes []string
	if profile.Name != form["name"][0] {

		changes = append(changes,
			fmt.Sprintf("%s: %q -> %q", "Name", profile.Name, form["name"][0]))
		profile.Name = form["name"][0]
	}
	if profile.DefaultLocation != form["location"][0] {

		changes = append(changes,
			fmt.Sprintf("%s: %q -> %q", "Location", profile.DefaultLocation, form["location"][0]))
		profile.DefaultLocation = form["location"][0]
	}
	if profile.Emails != (len(form["nag"]) > 0) {
		changes = append(changes,
			fmt.Sprintf("%s: %v -> %v", "Nag", profile.Emails, !profile.Emails))
		profile.Emails = len(form["nag"]) > 0
	}
	if profile.Invite != (len(form["invite"]) > 0) {
		changes = append(changes,
			fmt.Sprintf("%s: %v -> %v", "Invite", profile.Invite, !profile.Invite))
		profile.Invite = len(form["invite"]) > 0
	}
	if profile.Notify != (len(form["notify"]) > 0) {
		changes = append(changes,
			fmt.Sprintf("%s: %v -> %v", "Notify", profile.Notify, !profile.Notify))
		profile.Notify = len(form["notify"]) > 0
	}
	if profile.Superuser != (len(form["admin"]) > 0) {
		changes = append(changes,
			fmt.Sprintf("%s: %v -> %v", "Admin", profile.Superuser, !profile.Superuser))
		profile.Superuser = len(form["admin"]) > 0
	}

	if len(changes) > 0 {
		log.Print("Changes:\n")
		for _, c := range changes {
			log.Print(c)
		}

		_, err := dsClient.Put(ctx, profile.ID, profile)
		if err != nil {
			return fmt.Errorf("Error updating user: %v", err)
		}

		data.Base.Msg = "Profile updated"
	}
	
	return nil
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	user, err := loggedIn(w, r)
	if err != nil {
		log.Print(err.Error())
		return
	}

	data := &ProfileData{
		Profile: user,
		Base: BaseTemplate{Tab: "profile", User: user},
	}
	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/profile.html"))

	// Parse form data
	if err := r.ParseForm(); err != nil {
		data.Base.Error = fmt.Sprintf("Error parsing form: %v", err)
		tmpl.ExecuteTemplate(w, "invite.html", data)
		return
	}

	if hi := r.FormValue("hi"); hi != "" {
		data.Highlight = hi
	}

	if r.Method == http.MethodPost {
		err = updateProfile(r, data, user)
		if err != nil {
			if data.Base.Error == "" {
				data.Base.Error = "Internal error"
			}
			log.Printf("Error processing POST: %v", err)
			tmpl.ExecuteTemplate(w, "profile.html", data)
			return
		}

        // Redirect to clear form
		if user.Superuser {
			var pid = user.ID.Name
			if r.FormValue("edit") != "" {
				pid = r.FormValue("edit")
			}
			http.Redirect(w, r,
			  fmt.Sprintf("/profile?edit=%s&msg=%s", url.QueryEscape(pid), url.QueryEscape(data.Base.Msg)), http.StatusFound)
		} else {
			http.Redirect(w, r, "/profile?msg="+url.QueryEscape(data.Base.Msg), http.StatusFound)
		}
        return
	}
	data.Base.Msg = r.FormValue("msg")

	if user.Superuser {
		pid := r.FormValue("edit")
		if pid != "" {
			profile, err := getUser(r.Context(), pid)
			if err != nil {
				log.Printf("Error loading user %q: %v", pid, err)
				data.Base.Error = "Can't load user"
			}
			data.Profile = profile
		}
		users, err := getAllUsers(r.Context())
		if err != nil {
			log.Print(err)
			data.Base.Error = "Error querying users"
		}
		data.Users = users
	}

	err = tmpl.ExecuteTemplate(w, "profile.html", data)
	if err != nil {
		log.Printf("Error executing profile.html: %v", err)
	}
}
