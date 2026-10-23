package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Schiffer116/skipli/api/internal/broadcast"
)

// apiError is a non-2xx answer; its text is the server's own message.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return e.Message }

var errSignedOut = errors.New("signed out: run `skipli-cli login <email>` again")

// client calls the HTTP API with the stored Session's cookies. A 401 gets
// one refresh and one retry; nothing else is retried.
type client struct {
	base string
	http *http.Client
	path string // the session file
	sess session
}

func (c *client) send(ctx context.Context, method, path string, body any, headers http.Header, cookies ...*http.Cookie) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header[k] = v
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		if after := resp.Header.Get("Retry-After"); resp.StatusCode == http.StatusTooManyRequests && after != "" {
			msg += " (retry in " + after + "s)"
		}
		return resp, data, &apiError{Status: resp.StatusCode, Message: msg}
	}
	return resp, data, nil
}

// call makes an authenticated request and returns the response body.
func (c *client) call(ctx context.Context, method, path string, body any, headers http.Header) ([]byte, error) {
	if c.sess.IDToken == "" && c.sess.RefreshToken == "" {
		return nil, errors.New("not signed in: run `skipli-cli login <email>`")
	}
	for attempt := 0; ; attempt++ {
		_, data, err := c.send(ctx, method, path, body, headers, &http.Cookie{Name: "id_token", Value: c.sess.IDToken})
		var api *apiError
		if attempt == 0 && errors.As(err, &api) && api.Status == http.StatusUnauthorized {
			if err := c.refresh(ctx); err != nil {
				return nil, err
			}
			continue
		}
		return data, err
	}
}

func (c *client) refresh(ctx context.Context) error {
	if c.sess.RefreshToken == "" {
		return errSignedOut
	}
	resp, _, err := c.send(ctx, "POST", "/api/auth/refresh", nil, nil, &http.Cookie{Name: "refresh_token", Value: c.sess.RefreshToken})
	var api *apiError
	if errors.As(err, &api) && api.Status == http.StatusUnauthorized {
		return errSignedOut
	}
	if err != nil {
		return err
	}
	token := cookie(resp, "id_token")
	if token == "" {
		return errors.New("refresh gave no id token")
	}
	c.sess.IDToken = token
	return saveSession(c.path, c.sess)
}

func cookie(resp *http.Response, name string) string {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// changeHeader names a write, as the app does, so the broadcast of it to the
// board's open pages carries an id.
func changeHeader(id string) http.Header {
	return http.Header{broadcast.ChangeIDHeader: {id}}
}
