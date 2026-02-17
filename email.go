package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"text/template"

	"github.com/smtp2go-oss/smtp2go-go"
)

type emailTemplate string
const (
	firstNag      emailTemplate = "first"
	secondNag     emailTemplate = "second"
	gnScheduled   emailTemplate = "scheduled"
)

func email(ctx context.Context, emailType emailTemplate, gn *Gamenight) error {
	subj := map[emailTemplate]string{
		firstNag: "Want to host gamenight?",
		secondNag: "Still looking to find a host for gamenight this week!",
		gnScheduled: "A new gamenight has been scheduled!",
	}

	file := fmt.Sprintf("templates/%s.email", emailType)
	tmpl := template.Must(template.ParseFiles("templates/footer.email", file))
	data := map[string]string{
		"InviteURL": fmt.Sprintf("https://%s/invite", config(ctx, "url")),
		"Unsubscribe": fmt.Sprintf("https://%s/profile", config(ctx, "url")),
	}

	var pref userPreference
	switch emailType {
		case firstNag, secondNag: {
			pref = emailsPreference
			data["Unsubscribe"] += "?hi=reminder"
			data["Checkbox"] = "Get an email if gamenight needs scheduling"
			data["Purpose"] = "if no one is hosting gamenight"
		}
		case gnScheduled: {
			pref = notifyPreference
			data["Unsubscribe"] += "?hi=sched"
			data["Checkbox"] = "Get an email when a gamenight is scheduled"
			data["Purpose"] = "when a new gamenight is scheduled"
			if gn == nil {
				return fmt.Errorf("Missing gamenight data")
			}
			data["When"] = gn.When().Format("Monday, 1/02 at 3:04pm")
			data["Where"] = gn.Location
			data["Who"] = gn.GetOwner().Name
			data["Notes"] = gn.Notes
		}
		default: {
			return fmt.Errorf("Impossible email type")
		}
	}
	users, err := getSelectedUsers(ctx, pref)
	if err != nil {
		return fmt.Errorf("Error getting users for email %q: %v", emailType, err)
	}

	if len(users) == 0 {
		log.Printf("No users selected for %q", emailType)
		return nil
	}

	var buf bytes.Buffer
	tmplName := fmt.Sprintf("%s.email", emailType)
	if err = tmpl.ExecuteTemplate(&buf, tmplName, data); err != nil {
		return fmt.Errorf("Error executing template: %v", err)
	}

	return send(ctx, users, subj[emailType], buf.String())
}

func send(ctx context.Context, users []*User, subject, body string) error {
	if len(users) == 0 {
		log.Printf("No users to email, nothing to do")
		return nil
	}

	// Your API key as a standard Go string
    apiKey := config(ctx, "smtp2go_key")
	if apiKey == "" {
		return fmt.Errorf("No API key found")
	}
	sender := config(ctx, "sender")
	if sender == "" {
		return fmt.Errorf("No sender defined")
	}

	os.Setenv("SMTP2GO_API_KEY", apiKey)

	var addrs []string
	for _, u := range users {
		addrs = append(addrs, u.Email())
	}

	email := smtp2go.Email{
		From: fmt.Sprintf("Gamenight <%s>", sender),
		To: []string{"dan@peeron.com"},
		Bcc: addrs,
		Subject:  subject,
		TextBody: body,
	}
	res, err := smtp2go.Send(&email)
	if err != nil {
		return fmt.Errorf("Error sending email %q: %v", subject, err)
	}
	log.Printf("Sent %q to %d users: %s", subject, len(users), res.RequestId)
	log.Printf("From: %s", sender)
	log.Printf("BCC: %v", addrs)
	log.Printf("Subject: %s", subject)
	log.Printf("Body:\n%s", body)
	return nil
}
