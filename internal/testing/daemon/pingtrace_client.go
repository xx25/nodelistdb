package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FidomailClient talks to a fidomail node's loopback control API
// (docs/CONTROL-API.md in the fidomail repo): it sends a netmail, reads a
// sent message's delivery state, and lists inbound netmail for one local
// recipient name. It is the whole of the tracer's contact with FTN.
type FidomailClient struct {
	base  string
	token string
	http  *http.Client
}

// NewFidomailClient builds a client for the API at base (e.g.
// http://127.0.0.1:8083) presenting the given bearer token.
func NewFidomailClient(base, token string, timeout time.Duration) *FidomailClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &FidomailClient{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: timeout},
	}
}

// SendNetmailRequest is the POST /api/v1/netmail body.
type SendNetmailRequest struct {
	ToAddr   string `json:"to_addr"`
	ToName   string `json:"to_name"`
	FromAddr string `json:"from_addr,omitempty"`
	FromName string `json:"from_name,omitempty"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Direct   bool   `json:"direct,omitempty"`
}

// SendNetmailResponse is what fidomail reports for an accepted send.
type SendNetmailResponse struct {
	MessageID   uint64 `json:"message_id"`
	MSGID       string `json:"msgid"`
	Disposition string `json:"disposition"` // queued | local
	RouteVia    string `json:"route_via"`
	RouteSource string `json:"route_source"`
	SessionAKA  string `json:"session_aka"`
}

// NetmailStatus is GET /api/v1/netmail/{id}.
type NetmailStatus struct {
	ID        uint64    `json:"id"`
	MSGID     string    `json:"msgid"`
	Status    string    `json:"status"`
	ToAddr    string    `json:"to_addr"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InboxItem is one inbound netmail from GET /api/v1/netmail/inbox.
type InboxItem struct {
	ID            uint64    `json:"id"`
	MSGID         string    `json:"msgid"`
	ReplyID       string    `json:"reply_id"`
	Network       string    `json:"network"`
	FromName      string    `json:"from_name"`
	FromAddr      string    `json:"from_addr"`
	ToName        string    `json:"to_name"`
	ToAddr        string    `json:"to_addr"`
	Subject       string    `json:"subject"`
	Date          time.Time `json:"date"`
	ReceivedAt    time.Time `json:"received_at"`
	Tearline      string    `json:"tearline"`
	PID           string    `json:"pid"`
	Vias          []string  `json:"vias"`
	Body          string    `json:"body"`
	BodyTruncated bool      `json:"body_truncated"`
}

// InboxPage is one page of inbox items, oldest first, with the id
// watermark to continue from.
type InboxPage struct {
	Items []InboxItem `json:"items"`
	MaxID uint64      `json:"max_id"`
}

// APIError is fidomail's error envelope with the HTTP status attached.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("fidomail API %d %s: %s", e.Status, e.Code, e.Message)
}

// IsNotFound reports a 404 from the API.
func IsNotFound(err error) bool {
	e, ok := err.(*APIError)
	return ok && e.Status == http.StatusNotFound
}

// SendNetmail queues one netmail.
func (c *FidomailClient) SendNetmail(ctx context.Context, req SendNetmailRequest) (SendNetmailResponse, error) {
	var out SendNetmailResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/netmail", nil, req, &out)
	return out, err
}

// NetmailStatus reads a sent message's delivery state.
func (c *FidomailClient) NetmailStatus(ctx context.Context, id uint64) (NetmailStatus, error) {
	var out NetmailStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/netmail/"+strconv.FormatUint(id, 10), nil, nil, &out)
	return out, err
}

// Inbox lists inbound netmail addressed to toName at one of the node's
// AKAs, ids greater than or equal to minID (0 = no bound), received
// since the given time (zero = no bound), at most limit items.
func (c *FidomailClient) Inbox(ctx context.Context, network, toName string, minID uint64, since time.Time, limit int) (InboxPage, error) {
	q := url.Values{}
	q.Set("to_name", toName)
	if network != "" {
		q.Set("network", network)
	}
	if minID > 0 {
		q.Set("min_id", strconv.FormatUint(minID, 10))
	}
	if !since.IsZero() {
		q.Set("since", since.UTC().Format(time.RFC3339))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out InboxPage
	err := c.do(ctx, http.MethodGet, "/api/v1/netmail/inbox", q, nil, &out)
	return out, err
}

func (c *FidomailClient) do(ctx context.Context, method, path string, q url.Values, body, out interface{}) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fidomail API %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("fidomail API %s %s: read: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &env)
		if env.Error.Code == "" {
			env.Error.Code = "http_" + strconv.Itoa(resp.StatusCode)
			env.Error.Message = strings.TrimSpace(string(data))
		}
		return &APIError{Status: resp.StatusCode, Code: env.Error.Code, Message: env.Error.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("fidomail API %s %s: decode: %w", method, path, err)
	}
	return nil
}
