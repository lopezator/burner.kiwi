package sendgridmail

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/haydenwoodhead/burner.kiwi/data"
	"github.com/haydenwoodhead/burner.kiwi/data/inmemory"
)

func TestSendgrid_SendgridIncoming_Verified(t *testing.T) {
	s := SendgridMail{
		db: inmemory.GetInMemoryDB(),
		isBlacklisted: func(email string) bool {
			return false
		},
	}

	if err := s.db.SaveNewInbox(data.Inbox{
		Address:        "rober@example.com",
		ID:             "4d45c7c3-ea31-4a42-b953-4fe0b6e39553",
		CreatedAt:      time.Now().Unix(),
		TTL:            time.Now().Add(1 * time.Hour).Unix(),
		FailedToCreate: false,
	}); err != nil {
		t.Fatal(err)
	}

	router := mux.NewRouter()
	router.HandleFunc("/sg/incoming/{inboxID}/", s.sendgridIncoming)

	httpServer := httptest.NewServer(router)

	resp, err := http.PostForm(httpServer.URL+"/sg/incoming/4d45c7c3-ea31-4a42-b953-4fe0b6e39553/", url.Values{
		"envelope": {`{"to":["david@example.com"],"from":"david@example.com"}`},
		"from":     {`David López <david@example.com>`},
		"subject":  {"Hello world"},
		"text":     {"Hola mundo!"},
		"html":     {`<html><body><a href="https://example.com">Hola Mundo!</a></body></html>`},
	})

	if err != nil {
		t.Fatalf("TestServer_SendgridIncoming_Verified: failed to post data: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("TestServer_SendgridIncoming_Verified: returned status not 200 got: %v", resp.StatusCode)
	}

	msgs, _ := s.db.GetMessagesByInboxID("4d45c7c3-ea31-4a42-b953-4fe0b6e39553")

	if len(msgs) == 0 {
		t.Fatalf("TestServer_SendgridIncoming_Verified: failed to save incoming message to database")
	}

	msg := msgs[0]

	if msg.Sender != "david@example.com" || msg.From != "david@example.com" {
		t.Fatalf("TestServer_SendgridIncoming_Verified: sender or from not correct. Should be david@example.com. Sender: %v, from %v", msg.Sender, msg.From)
	}

	if msg.Subject != "Hello there" {
		t.Fatalf("TestServer_SendgridIncoming_Verfified: subject not 'Hello world', actually %v", msg.Subject)
	}

	if msg.BodyPlain != "Hello there" {
		t.Fatalf("TestServer_SendgridIncoming_Verfified: BodyPlain not 'Hola mundo!', actually %v", msg.BodyPlain)
	}

	const expectedHTML = `<html><head></head><body><a href="https://example.com" target="_blank">Hola Mundo!</a></body></html>`

	if msg.BodyHTML != expectedHTML {
		t.Fatalf("TestServer_SendgridIncoming_Verfified: html body different than expected. \nExpected: %v\nGot: %v", expectedHTML, msg.BodyHTML)
	}
}

/*
func TestMailgun_MailgunIncoming_Blacklisted(t *testing.T) {
	m := MailgunMail{
		mg: FakeMG{Verify: true},
		db: inmemory.GetInMemoryDB(),
		isBlacklisted: func(email string) bool {
			return true
		},
	}

	m.db.SaveNewInbox(data.Inbox{
		Address:        "bobby@example.com",
		ID:             "17b79467-f409-4e7d-86a9-0dc79b77f7c3",
		CreatedAt:      time.Now().Unix(),
		TTL:            time.Now().Add(1 * time.Hour).Unix(),
		FailedToCreate: false,
		MGRouteID:      "1234",
	})

	router := mux.NewRouter()
	router.HandleFunc("/mg/incoming/{inboxID}/", m.mailgunIncoming)

	httpServer := httptest.NewServer(router)

	resp, err := http.PostForm(httpServer.URL+"/mg/incoming/17b79467-f409-4e7d-86a9-0dc79b77f7c3/", url.Values{
		"message-id": {"1234"},
		"sender":     {"hayden@example.com"},
		"from":       {"hayden@example.com"},
		"subject":    {"Hello there"},
		"body-plain": {"Hello there"},
		"body-html":  {`<html><body><a href="https://example.com">Hello there</a></body></html>`},
	})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotAcceptable, resp.StatusCode)
}

func TestMailgun_MailgunIncoming_UnVerified(t *testing.T) {
	m := MailgunMail{
		mg: FakeMG{Verify: false},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	m.mailgunIncoming(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("TestServer_MailgunIncoming_UnVerified: expected status code: %v, got %v", http.StatusUnauthorized, rr.Code)
	}
}

func TestMailgun_DeleteExpiredRoutes(t *testing.T) {
	m := MailgunMail{
		mg: FakeMG{},
	}

	// Should be deleted
	routes["1234"] = mailgun.Route{
		Priority:    1,
		Description: "1",
		Expression:  "",
		Actions:     []string{},
		CreatedAt:   "1",
		ID:          "1234",
	}

	// Should be deleted
	routes["91011"] = mailgun.Route{
		Priority:    1,
		Description: fmt.Sprintf("%v", time.Now().Add(-1*time.Second).Unix()),
		Expression:  "",
		Actions:     []string{},
		CreatedAt:   "1",
		ID:          "91011",
	}

	// should not be deleted
	routes["5678"] = mailgun.Route{
		Priority:    1,
		Description: "2124941352",
		Expression:  "",
		Actions:     []string{},
		CreatedAt:   "1",
		ID:          "5678",
	}

	m.DeleteExpiredRoutes()

	if _, ok := routes["1234"]; ok {
		t.Errorf("TestServer_DeleteOldRoutes: Expired route still exists.")
	}

	if _, ok := routes["91011"]; ok {
		t.Errorf("TestServer_DeleteOldRoutes: Expired route still exists.")
	}

	if _, ok := routes["5678"]; !ok {
		t.Errorf("TestServer_DeleteOldRoutes: Valid route deleted.")
	}
}
*/
