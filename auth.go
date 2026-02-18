package main

import(
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"cloud.google.com/go/datastore"
    smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/idtoken"
)

const projectID = 474756814972

func getSecret(ctx context.Context, key string) string {
	req := &smpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%d/secrets/%s/versions/latest", projectID, key),
	}

	res, err := sm.AccessSecretVersion(ctx, req)
	if err != nil {
		log.Fatalf("Can't get %q: %v", key, err)
	}

	return string(res.Payload.Data)
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Printf("Can't parse form: %v", err)
		http.Error(w, "Invalid form", http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()

    token := r.Header.Get("X-ID-TOKEN")
    if token == "" {
        http.Error(w, "Missing token", http.StatusBadRequest)
        return
    }

    aud := getSecret(ctx, "client_id")
    
    payload, err := idtoken.Validate(ctx, token, aud)
    if err != nil {
		log.Printf("Invalid token: %v", err)
        http.Error(w, "Invalid Token", http.StatusUnauthorized)
        return
    }

    email := payload.Claims["email"].(string)

    log.Printf("Authenticated user %s", email)

	session, _ := sessionStore.Get(r, "session")
	session.Values["authed"] = true
	session.Values["id"] = email
	if err := session.Save(r, w); err != nil {
		log.Printf("Can't write session: %v", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		log.Printf("Can't parse form: %v", err)
		http.Error(w, "Invalid form", http.StatusUnprocessableEntity)
		return
	}
	dest := r.FormValue("dest")
	if dest == "" { dest = "/" }

	u, err := getUserSession(ctx, r)
	if err != nil {
		log.Printf("Error getting user session: %v, proceeding", err)
	}
	if u != nil {
		// User is already logged in, just redirect
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	data := map[string]string{
		"ClientID": getSecret(ctx, "client_id"),
		"Redirect": dest,
	}
	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, data)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := sessionStore.Get(r, "session")

	log.Printf("Logging out %v", session.Values["id"])

	// Delete the cookie, and also invalidate it.
    session.Options.MaxAge = -1
    session.Values["authenticated"] = false

    err := session.Save(r, w)
    if err != nil {
        http.Error(w, "Failed to logout", http.StatusInternalServerError)
        return
    }

    // Redirect back to home or login page
    http.Redirect(w, r, "/", http.StatusFound)
}

func getUserSession(ctx context.Context, r *http.Request) (*User, error) {
	user := &User{}
	session, err := sessionStore.Get(r, "session")
	if err != nil {
		log.Printf("Error getting session: %v", err)
		return nil, fmt.Errorf("No session")
	}
	if a, ok := session.Values["authed"].(bool); !ok || !a {
		log.Printf("Session not authed")
		return nil, fmt.Errorf("Invalid session")
	}
	email, ok := session.Values["id"].(string)
	if !ok {
		log.Printf("Invalid ID in session")
		return nil, fmt.Errorf("Invalid ID")
	}
	key := datastore.NameKey("User", email, nil)
	if err := dsClient.Get(ctx, key, user); err != nil {
		log.Printf("Creating a new user for %q", email)
		user = &User{
			ID:   key,
			Name: strings.Split(email, "@")[0],
		}
		_, err := dsClient.Put(ctx, user.ID, user)
		if err != nil {
			log.Printf("Error saving new user to Datastore: %v", err)
			return nil, fmt.Errorf("Couldn't created user")
		}
	}
	if strings.Contains(user.Name, "@") {
		user.Name = strings.Split(user.Name, "@")[0]
	}
	return user, nil
}

func loginFunc(path string, next http.HandlerFunc) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
        session, err := sessionStore.Get(r, "session")
		if err != nil {
			log.Printf("Error reading cookie: %v", err)
		}

        if auth, ok := session.Values["authed"].(bool); !ok || !auth {
            http.Redirect(w, r, "/auth/login?dest="+path, http.StatusFound)
            return
        }
        next.ServeHTTP(w, r)
	})
}

func adminFunc(path string, next http.HandlerFunc) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
        session, _ := sessionStore.Get(r, "session")

		if r.Header.Get("X-Appengine-Cron") == "true" {
			log.Printf("Authorized request from cron")
			next.ServeHTTP(w, r)
			return
		}

        if auth, ok := session.Values["authed"].(bool); !ok || !auth {
            http.Redirect(w, r, "/auth/login?dest="+path, http.StatusFound)
            return
        }

		if u, _ := getUserSession(r.Context(), r); u == nil || !u.Superuser {
			log.Printf("Admin required for %q", path)
            http.Redirect(w, r, "/", http.StatusFound)
            return
		}
        next.ServeHTTP(w, r)
	})
}
