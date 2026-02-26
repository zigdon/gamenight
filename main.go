package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/datastore"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/gorilla/csrf"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"google.golang.org/api/iterator"
)

var (
	dsClient      *datastore.Client
	svc           *calSvc
	sm            *secretmanager.Client
	sessionStore  *sessions.CookieStore
	stageInstance = false
)

func initThings(ctx context.Context) error {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	var err error
	dsClient, err = datastore.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("Failed to create client: %v", err)
	}

	svc, err = newCalSvc(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get calendar client: %v", err)
	}

	sm, err = secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("Can't get secrets: %v", err)
	}

	sessionStore = sessions.NewCookieStore([]byte(getSecret(ctx, "cookie_key")))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,      // Prevents JavaScript access (XSS protection)
		Secure:   true,      // Only sent over HTTPS
		SameSite: http.SameSiteLaxMode,
	}

	return nil
}

func main() {
	ctx := context.Background()
	if err := initThings(ctx); err != nil {
		log.Fatalf("Init failed: %v", err)
	}

	var opts []csrf.Option

	if devServer(ctx) {
		log.Printf("Appying csrf options for dev environment")
		opts = append(opts,
			csrf.TrustedOrigins([]string{"localhost:8080"}),
			csrf.Secure(false),
			csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// This will print the specific reason the CSRF check failed
				reason := csrf.FailureReason(r)
				fmt.Printf("CSRF Failure: %v\n", reason)

				http.Error(w, fmt.Sprintf("Forbidden: %v", reason), http.StatusForbidden)
			})),
		)
	}
	validateCSRF := csrf.Protect([]byte(getSecret(ctx, "csrf_key")), opts...)

	r := mux.NewRouter()

	r.HandleFunc("/", handleIndex)
	r.HandleFunc("/auth/logout", handleLogout)
	r.HandleFunc("/schedule", loginFunc(handleSchedule))
	r.HandleFunc("/tasks/nag", adminFunc(handleNag))
	r.HandleFunc("/tasks/schedule", adminFunc(handleTaskSchedule))
	r.HandleFunc("/tos", renderPage("tos", ""))
	r.HandleFunc("/about", renderPage("about", "about"))

	r.Handle("/auth/login", validateCSRF(http.HandlerFunc(handleLogin)))
	r.Handle("/auth/token", validateCSRF(http.HandlerFunc(handleToken)))
	r.Handle("/invite", validateCSRF(http.HandlerFunc(loginFunc(handleInvite))))
	r.Handle("/profile", validateCSRF(http.HandlerFunc(loginFunc(handleProfile))))
	r.Handle("/config", validateCSRF(http.HandlerFunc(adminFunc(handleConfig))))

	if devServer(ctx) {
		r.HandleFunc("/debug", handleDebug)
	}

	if os.Getenv("STAGING") != "" {
		stageInstance = true
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, handlers.ProxyHeaders(r)); err != nil {
		log.Fatal(err)
	}
}

func mkDefault() Gamenight {
	// Before tuesday, say probably.
	// Before friday, say maybe.
	// Then say no.
	switch time.Now().In(tz()).Weekday() {
	case time.Sunday, time.Monday, time.Tuesday:
		return Gamenight{Status: "Probably"}
	case time.Wednesday, time.Thursday, time.Friday:
		return Gamenight{Status: "Maybe"}
	default:
		return Gamenight{Status: "No"}
	}
}

type Future struct {
	Type     string
	When     time.Time
	Location string
	Owner    string
}

type IndexData struct {
	Current    Gamenight
	Future     []Future
	Updated    time.Time
	CalendarID string
}

func renderPage(file, tab string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, _ := getUserSession(ctx, r)
		data := struct {
			Base BaseTemplate
		}{
			Base: BaseTemplate{
				Tab:  tab,
				User: user,
			},
		}
		tmpl := template.Must(
			template.ParseFiles(
				"templates/base.html",
				fmt.Sprintf("templates/%s.html", file)))
		err := tmpl.ExecuteTemplate(w, fmt.Sprintf("%s.html", file), data)
		if err != nil {
			log.Printf("Error executing tos: %v", err)
		}
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var sched Gamenight
	now := time.Now().In(tz())
	// Sunday 00:00
	sun := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz()).AddDate(0, 0, int(7-now.Weekday()))
	found := make(map[string]bool)
	it := dsClient.Run(ctx, datastore.NewQuery("Gamenight").
		FilterField("s", "=", "Yes").
		FilterField("d", ">", now.AddDate(0, 0, -1)).
		FilterField("d", "<", sun).
		Order("d"))

	// The first result is the next scheduled gamenight.
	k, err := it.Next(&sched)
	if err == iterator.Done {
		sched = mkDefault()
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		sched.Date = sched.Date.In(tz())
		sched.Time = sched.Time.In(tz())
		if err := sched.Load(ctx); err != nil {
			log.Printf("Error loading %s: %v", k, err)
		}
		found[fmt.Sprintf("%s@%s", sched.GetOwner().Name, sched.When())] = true
	}

	// Any additional results should be listed below.
	var days []Future
	for {
		var gn Gamenight
		_, err := it.Next(&gn)
		if err == iterator.Done {
			break
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err = gn.Load(ctx); err != nil {
			log.Printf("Error filling gn: %v", err)
		}
		days = append(days, Future{
			Type:     "gamenight",
			When:     gn.When(),
			Location: gn.Location,
			Owner:    gn.GetOwner().Name,
		})
		found[fmt.Sprintf("%s@%s", gn.GetOwner().Name, gn.When())] = true
	}

	invs, err := getAllInvitations(ctx, now, true)
	if err != nil {
		log.Printf("Error getting invitations: %v", err)
	}
	for _, inv := range invs {
		if err = inv.Load(ctx); err != nil {
			log.Printf("Error filling inv: %v", err)
		}
		if _, ok := found[fmt.Sprintf("%s@%s", inv.GetOwner().Name, inv.When())]; ok {
			continue
		}
		days = append(days, Future{
			Type:     "invite",
			When:     inv.When(),
			Location: inv.Location,
			Owner:    inv.GetOwner().Name,
		})
	}

	var data = IndexData{
		Current:    sched,
		Future:     days,
		Updated:    now,
		CalendarID: config(ctx, "calendar_id"),
	}
	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/index.html"))
	err = tmpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		log.Printf("Error executing index: %v", err)
	}
}
