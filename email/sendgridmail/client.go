package sendgridmail

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go"
)

// Sendgrid is a sendgrid api client implementation
type Sendgrid struct {
	domain string
	key    string
	host   string
}

// NewSengrid creates a new client instance
func NewSendgrid(domain string, key string) *Sendgrid {
	return &Sendgrid{
		domain: domain,
		key:    key,
		host:   "https://api.sendgrid.com",
	}
}

// ParseSetting contains an inbound parse setting related info
type ParseSetting struct {
	Hostname  string
	URL       string
	SpamCheck bool `json:"spam_check"`
	SendRaw   bool `json:"send_raw"`
}

// CreateParseSetting creates a parse setting
func (s *Sendgrid) CreateParseSetting(url string) (*ParseSetting, error) {
	request := s.getRequest("/user/webhooks/parse/settings", rest.Post)
	requestBody, err := json.Marshal(&ParseSetting{
		Hostname:  s.domain,
		SpamCheck: false,
		SendRaw:   false,
	})
	if err != nil {
		return nil, err
	}
	request.Body = requestBody
	response, err := s.api(request)
	if err != nil {
		return nil, errors.Wrap(err, "Sendgrid.CreateParseSetting: failed to create a parse setting")
	}
	var responseBody *ParseSetting
	if err := json.Unmarshal([]byte(response.Body), &responseBody); err != nil {
		return nil, errors.Wrap(err, "Sendgrid.CreateParseSetting: failed to unmarshal json response")
	}
	return responseBody, nil
}

// GetParseSettings gets all parse settings
func (s *Sendgrid) GetParseSettings() ([]*ParseSetting, error) {
	request := s.getRequest("/user/webhooks/parse/settings", rest.Get)
	response, err := s.api(request)
	if err != nil {
		return nil, errors.Wrap(err, "Sendgrid.CreateParseSetting: failed to get parse settings")
	}
	var responseBody []*ParseSetting
	if err := json.Unmarshal([]byte(response.Body), &responseBody); err != nil {
		return nil, errors.Wrap(err, "Sendgrid.CreateParseSetting: failed to unmarshal json response")
	}
	return responseBody, nil
}

func (s *Sendgrid) getRequest(endpoint string, method rest.Method) rest.Request {
	request := sendgrid.GetRequest(s.key, endpoint, s.host)
	request.Method = method
	return request
}

func (s *Sendgrid) api(request rest.Request) (*rest.Response, error) {
	return sendgrid.API(request)
}
