package sendgridmail

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
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
)

var _ email.Provider = &SendgridMail{}

// SendgridMail is a sendgrid implementation of the Mail inter
type SendgridMail struct {
	websiteAddr   string
	sg            *Sendgrid
	db            data.Database
	isBlacklisted func(string) bool
}

// NewSendgridProvider creates a new Sendgrid email.Provider
func NewSendgridProvider(domain string, key string) *SendgridMail {
	return &SendgridMail{
		sg: NewSendgrid(domain, key),
	}
}

// Start implements email.Provider Start()
func (s *SendgridMail) Start(websiteAddr string, db data.Database, r *mux.Router, isBlackisted func(string) bool) error {
	s.websiteAddr = websiteAddr
	s.db = db
	r.HandleFunc("/sg/incoming/{inboxID}", s.sendgridIncoming).Methods(http.MethodPost)
	return nil
}

// Stop implements email.Provider Stop()
func (s *SendgridMail) Stop() error {
	return nil
}

// RegisterRoute implements email.Provider RegisterRoute()
func (s *SendgridMail) RegisterRoute(i data.Inbox) (string, error) {
	routeAddr := s.websiteAddr + "/sg/incoming" + i.ID + "/"
	route, err := s.sg.CreateParseSetting(routeAddr)
	fmt.Println(route)
	return "", errors.Wrap(err, "createRoute: failed to create sendgrid parse setting")
}

// DeleteExpiredRoutes implements email.Provider DeleteExpiredRoutes()
func (s *SendgridMail) DeleteExpiredRoutes() error {
	// TODO(lopezator) Implement delete expired routes
	// Sendgrid doesn't have a description field for a parse setting, so we must set inside the storage somehow
	// And retrieve it here instead of parsing it from an object description field in API

	return nil
}

func (s *SendgridMail) sendgridIncoming(w http.ResponseWriter, r *http.Request) {
	var envelope = struct {
		To   []string
		From string
	}{}
	if err := json.Unmarshal([]byte(r.FormValue("envelope")), &envelope); err != nil {
		log.Printf("SendgridIncoming: failed to unmarshal envelope data: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if s.isBlacklisted(envelope.From) {
		w.WriteHeader(http.StatusNotAcceptable)
		metrics.IncomingEmails.With(prometheus.Labels{
			"action": "rejected",
		}).Inc()
		return
	}

	vars := mux.Vars(r)
	id := vars["inboxID"]

	i, err := s.db.GetInboxByID(id)

	if err != nil {
		log.Printf("SendgridIncoming: failed to get inbox: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var msg data.Message

	msg.InboxID = i.ID
	msg.TTL = i.TTL

	mID, err := uuid.NewRandom()
	if err != nil {
		log.Printf("SendgridIncoming: failed to generate uuid for inbox: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.ID = mID.String()
	msg.ReceivedAt = time.Now().Unix()
	msg.Sender = envelope.From
	from, err := mail.ParseAddress(r.FormValue("from"))
	if err != nil {
		log.Printf("SendgridIncoming: failed to parse from email address: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	msg.From = from.Address
	msg.Subject = r.FormValue("subject")
	msg.BodyPlain = r.FormValue("text")

	html := r.FormValue("html")
	// Check to see if there is anything in html before we modify it. Otherwise we end up setting a blank html doc
	// on all plaintext emails preventing them from being displayed.
	if strings.Compare(html, "") != 0 {
		sr := strings.NewReader(html)

		var doc *goquery.Document
		doc, err = goquery.NewDocumentFromReader(sr)

		if err != nil {
			log.Printf("SendgridIncoming: failed to create goquery doc: %v", err)
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
			log.Printf("SendgridIncoming: failed to get html doc: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		msg.BodyHTML = modifiedHTML
	}

	err = s.db.SaveNewMessage(msg)

	if err != nil {
		log.Printf("SendgridIncoming: failed to save message to db: %v", err)
	}

	_, err = w.Write([]byte(id))

	if err != nil {
		log.Printf("SendgridIncoming: failed to write response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metrics.IncomingEmails.With(prometheus.Labels{
		"action": "accepted",
	}).Inc()
}
