// Package awsfake serves the AWS JSON protocol from an httptest server so
// handlers can be tested against real SDK clients without AWS access.
package awsfake

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Call is one request the fake received, keyed by operation name such as
// "PutItem" or "AdminCreateUser".
type Call struct {
	Op   string
	Body map[string]any
}

// Respond returns the status and JSON body for an operation. A non-2xx status
// with a body like {"__type":"ConditionalCheckFailedException"} becomes a
// typed SDK error.
type Respond func(op string, body map[string]any) (int, any)

type Server struct {
	srv     *httptest.Server
	respond Respond

	mu    sync.Mutex
	calls []Call
}

func New(t *testing.T, respond Respond) *Server {
	s := &Server{respond: respond}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *Server) serve(w http.ResponseWriter, req *http.Request) {
	target := req.Header.Get("X-Amz-Target")
	_, op, _ := strings.Cut(target, ".")

	var body map[string]any
	raw, _ := io.ReadAll(req.Body)
	json.Unmarshal(raw, &body)

	s.mu.Lock()
	s.calls = append(s.calls, Call{Op: op, Body: body})
	s.mu.Unlock()

	status, out := s.respond(op, body)
	if strings.HasPrefix(target, "DynamoDB") {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	} else {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(out)
}

// Calls returns every request received so far whose operation is in ops, or
// every request when ops is empty.
func (s *Server) Calls(ops ...string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var calls []Call
	for _, c := range s.calls {
		if len(ops) == 0 || slices.Contains(ops, c.Op) {
			calls = append(calls, c)
		}
	}
	return calls
}

func (s *Server) DynamoDB() *dynamodb.Client {
	return dynamodb.New(dynamodb.Options{
		Region:           "us-east-1",
		BaseEndpoint:     aws.String(s.srv.URL),
		Credentials:      aws.AnonymousCredentials{},
		RetryMaxAttempts: 1,
	})
}

func (s *Server) Cognito() *cip.Client {
	return cip.New(cip.Options{
		Region:           "us-east-1",
		BaseEndpoint:     aws.String(s.srv.URL),
		Credentials:      aws.AnonymousCredentials{},
		RetryMaxAttempts: 1,
	})
}
