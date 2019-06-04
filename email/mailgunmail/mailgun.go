package mailgunmail

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/haydenwoodhead/burner.kiwi/data"
	"github.com/haydenwoodhead/burner.kiwi/email"
	"github.com/haydenwoodhead/burner.kiwi/metrics"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/mailgun/mailgun-go.v1"
)

var _ email.Provider = &MailgunMail{}

// MailgunMail is a mailgun implementation of the Mail inter
type MailgunMail struct {
	websiteAddr   string
	mg            mailgun.Mailgun
	db            data.Database
	isBlacklisted func(string) bool
}

// NewMailgunProvider creates a new Mailgun email.Provider
func NewMailgunProvider(domain string, key string) *MailgunMail {
	return &MailgunMail{
		mg: mailgun.NewMailgun(domain, key, ""),
	}
}

// Start implements email.Provider Start()
func (m *MailgunMail) Start(websiteAddr string, db data.Database, r *mux.Router, isBlackisted func(string) bool) error {
	m.db = db
	m.isBlacklisted = isBlackisted
	m.websiteAddr = websiteAddr
	r.HandleFunc("/mg/incoming/{inboxID}/", m.mailgunIncoming).Methods(http.MethodPost)
	return nil
}

// Stop implements email.Provider Stop()
func (m *MailgunMail) Stop() error {
	return nil
}

// RegisterRoute implements email.Provider RegisterRoute()
func (m *MailgunMail) RegisterRoute(i data.Inbox) (string, error) {
	routeAddr := m.websiteAddr + "/mg/incoming/" + i.ID + "/"
	route, err := m.mg.CreateRoute(mailgun.Route{
		Priority:    1,
		Description: strconv.Itoa(int(i.TTL)),
		Expression:  "match_recipient(\"" + i.Address + "\")",
		Actions:     []string{"forward(\"" + routeAddr + "\")", "store()", "stop()"},
	})
	return route.ID, errors.Wrap(err, "createRoute: failed to create mailgun route")
}

// DeleteExpiredRoutes implements email.Provider DeleteExpiredRoutes()
func (m *MailgunMail) DeleteExpiredRoutes() error {
	_, rs, err := m.mg.GetRoutes(1000, 0)

	if err != nil {
		return errors.Wrap(err, "Mailgun.DeleteExpiredRoutes: failed to get routes")
	}

	for _, r := range rs {
		tInt, err := strconv.ParseInt(r.Description, 10, 64)

		if err != nil {
			log.Printf("Mailgun.DeleteExpiredRoutes: failed to parse route description as int: id=%v\n", r.ID)
			continue
		}

		t := time.Unix(tInt, 0)

		// if our route's ttl (expiration time) is before now... then delete it
		if t.Before(time.Now()) {
			err := m.mg.DeleteRoute(r.ID)

			if err != nil {
				log.Printf("Mailgun.DeleteExpiredRoutes: failed to delete route: id=%v\n", r.ID)
				continue
			}
		}
	}

	return nil
}

func (m *MailgunMail) mailgunIncoming(w http.ResponseWriter, r *http.Request) {
	ver, err := m.mg.VerifyWebhookRequest(r)
	if err != nil {
		log.Printf("MailgunIncoming: failed to verify request: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !ver {
		log.Printf("MailgunIncoming: invalid request")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if m.isBlacklisted(r.FormValue("sender")) {
		w.WriteHeader(http.StatusNotAcceptable)
		metrics.IncomingEmails.With(prometheus.Labels{
			"action": "rejected",
		}).Inc()
		return
	}

	vars := mux.Vars(r)
	id := vars["inboxID"]

	i, err := m.db.GetInboxByID(id)

	if err != nil {
		log.Printf("MailgunIncoming: failed to get inbox: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var msg data.Message

	msg.InboxID = i.ID
	msg.TTL = i.TTL

	mID, err := uuid.NewRandom()
	if err != nil {
		log.Printf("MailgunIncoming: failed to generate uuid for inbox: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.ID = mID.String()
	msg.ReceivedAt = time.Now().Unix()
	msg.MGID = r.FormValue("message-id")
	msg.Sender = r.FormValue("sender")
	msg.From = r.FormValue("from")
	msg.Subject = r.FormValue("subject")
	msg.BodyPlain = r.FormValue("body-plain")

	html := r.FormValue("body-html")

	// Check to see if there is anything in html before we modify it. Otherwise we end up setting a blank html doc
	// on all plaintext emails preventing them from being displayed.
	if strings.Compare(html, "") != 0 {
		sr := strings.NewReader(html)

		var doc *goquery.Document
		doc, err = goquery.NewDocumentFromReader(sr)

		if err != nil {
			log.Printf("MailgunIncoming: failed to create goquery doc: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Find all a tags and add a target="_blank" attr to them so they open links in a new tab rather than in the iframe
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			s.SetAttr("target", "_blank")
		})

		var modifiedHTML string
		modifiedHTML, err = doc.Html()

		if err != nil {
			log.Printf("MailgunIncoming: failed to get html doc: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		msg.BodyHTML = modifiedHTML
	}

	err = m.db.SaveNewMessage(msg)

	if err != nil {
		log.Printf("MailgunIncoming: failed to save message to db: %v", err)
	}

	_, err = w.Write([]byte(id))

	if err != nil {
		log.Printf("MailgunIncoming: failed to write response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metrics.IncomingEmails.With(prometheus.Labels{
		"action": "accepted",
	}).Inc()
}
