// Package realtime is the API behind a board's WebSocket, which carries news
// out only: teammates' saved changes reach the app over it, and nothing is
// written through it. The HTTP handlers hand what they saved to Broadcast.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	gwtypes "github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/coder/websocket"

	"github.com/Schiffer116/skipli/api/internal/access"
	"github.com/Schiffer116/skipli/api/internal/auth"
	"github.com/Schiffer116/skipli/api/internal/broadcast"
	"github.com/Schiffer116/skipli/api/internal/profile"
)

// API Gateway closes a WebSocket after two hours whatever happens, so a
// connection's record never needs to outlive that.
const connectionLifetime = 2 * time.Hour

// A broadcast that takes longer than this is abandoned: it's best-effort,
// and the save it follows is already done.
const broadcastTimeout = 5 * time.Second

// How many connections a broadcast posts to at once.
const fanOut = 16

// Gateway sends to API Gateway connections; tests stand in for it.
type Gateway interface {
	PostToConnection(context.Context, *apigatewaymanagementapi.PostToConnectionInput, ...func(*apigatewaymanagementapi.Options)) (*apigatewaymanagementapi.PostToConnectionOutput, error)
}

type Handler struct {
	db     *dynamodb.Client
	table  string
	tokens *auth.TokenVerifier
	// profiles name the people who connect.
	profiles *profile.Store
	// gateway reaches the connections of the API Gateway stage, or is nil
	// when the connections are the local relay's.
	gateway Gateway

	// The local relay's connections, by board, and who each belongs to.
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]broadcast.Person
}

// NewHandler serves the connections of the API Gateway stage at endpoint (its
// https management URL), or, with no endpoint, the local relay's.
func NewHandler(cfg aws.Config, db *dynamodb.Client, table, endpoint string, tokens *auth.TokenVerifier) *Handler {
	var gateway Gateway
	if endpoint != "" {
		gateway = apigatewaymanagementapi.NewFromConfig(cfg, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	return newHandler(db, table, tokens, gateway)
}

func newHandler(db *dynamodb.Client, table string, tokens *auth.TokenVerifier, gateway Gateway) *Handler {
	return &Handler{
		db:       db,
		table:    table,
		tokens:   tokens,
		profiles: profile.NewStore(db, table),
		gateway:  gateway,
		rooms:    map[string]map[*websocket.Conn]broadcast.Person{},
	}
}

// A board's connections are listed under one key, and each also has a
// record of its own that says which board it is on, since a disconnect
// names only the connection.
func boardKey(boardID string) string     { return "CONNS#" + boardID }
func connKey(connectionID string) string { return "CONN#" + connectionID }

// Handle serves the board WebSocket API Gateway: members connect, and the
// app pings to keep an idle connection open. Nothing else is accepted.
func (h *Handler) Handle(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch req.RequestContext.RouteKey {
	case "$connect":
		return status(h.connect(ctx, req)), nil
	case "$disconnect":
		h.disconnect(ctx, req.RequestContext.ConnectionID)
	}
	return status(http.StatusOK), nil
}

func status(code int) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: code}
}

func cookieHeader(headers map[string]string) string {
	for name, value := range headers {
		if strings.EqualFold(name, "Cookie") {
			return value
		}
	}
	return ""
}

// connect lets a board's members open its socket, and records the
// connection under the board so what's saved there reaches it.
func (h *Handler) connect(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) int {
	user, err := h.tokens.VerifyCookies(cookieHeader(req.Headers))
	if err != nil {
		return http.StatusUnauthorized
	}

	boardID := req.QueryStringParameters["boardId"]
	allowed, err := access.Check(ctx, h.db, h.table, boardID, user.ID)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		return http.StatusInternalServerError
	}
	if allowed != http.StatusOK {
		return allowed
	}

	person := h.person(ctx, user)
	id := req.RequestContext.ConnectionID
	expires := &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Add(connectionLifetime).Unix(), 10)}
	record := access.Key(boardKey(boardID), id)
	record["ExpiresAt"] = expires
	record["UserID"] = &types.AttributeValueMemberS{Value: person.ID}
	record["UserName"] = &types.AttributeValueMemberS{Value: person.Name}
	record["Email"] = &types.AttributeValueMemberS{Value: person.Email}
	pointer := access.Key(connKey(id), "BOARD")
	pointer["Board"] = &types.AttributeValueMemberS{Value: boardID}
	pointer["ExpiresAt"] = expires
	for _, item := range []map[string]types.AttributeValue{record, pointer} {
		if _, err := h.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(h.table), Item: item}); err != nil {
			log.Printf("failed to store connection: %v", err)
			return http.StatusInternalServerError
		}
	}
	// A connection can't be posted to until $connect has answered, so the
	// newcomer reads who is here with Present instead.
	h.send(ctx, boardID, presence(h.Present(ctx, boardID)), id)
	return http.StatusOK
}

// person is how a signed-in user shows to the others on a board.
func (h *Handler) person(ctx context.Context, user auth.User) broadcast.Person {
	name := profile.DefaultName(user.Email)
	if names, err := h.profiles.Names(ctx, []auth.User{user}); err != nil {
		log.Printf("failed to read profile: %v", err)
	} else {
		name = names[user.ID]
	}
	return broadcast.Person{ID: user.ID, Name: name, Email: user.Email}
}

func presence(people []broadcast.Person) broadcast.Message {
	if people == nil {
		people = []broadcast.Person{}
	}
	return broadcast.Message{Type: broadcast.PresenceChanged, People: people}
}

// disconnect forgets a closed connection. A record left behind (this call
// is best-effort too) is dropped when a broadcast finds it gone, or expires.
func (h *Handler) disconnect(ctx context.Context, connectionID string) {
	out, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key:       access.Key(connKey(connectionID), "BOARD"),
	})
	if err != nil {
		log.Printf("failed to find connection: %v", err)
		return
	}
	board, _ := out.Item["Board"].(*types.AttributeValueMemberS)
	if board == nil {
		return
	}
	h.forget(ctx, board.Value, connectionID)
	h.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(h.table), Key: access.Key(connKey(connectionID), "BOARD")})
	h.send(ctx, board.Value, presence(h.Present(ctx, board.Value)), connectionID)
}

func (h *Handler) forget(ctx context.Context, boardID, connectionID string) {
	if _, err := h.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(h.table), Key: access.Key(boardKey(boardID), connectionID)}); err != nil {
		log.Printf("failed to forget connection: %v", err)
	}
}

// Broadcast pushes a message to every connection open on the board,
// forgetting the ones API Gateway says are gone. Connections that can't be
// reached are skipped: the app refetches the board whenever it reconnects.
func (h *Handler) Broadcast(ctx context.Context, boardID string, message broadcast.Message) {
	h.send(ctx, boardID, message, "")
}

// send is Broadcast that skips one connection, which may not be open yet or
// is closing.
func (h *Handler) send(ctx context.Context, boardID string, message broadcast.Message, except string) {
	ctx, cancel := context.WithTimeout(ctx, broadcastTimeout)
	defer cancel()

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("failed to encode message: %v", err)
		return
	}

	h.broadcastLocal(ctx, boardID, data)
	if h.gateway != nil {
		h.broadcastGateway(ctx, boardID, data, except)
	}
}

// Present lists the people with the board open, each once.
func (h *Handler) Present(ctx context.Context, boardID string) []broadcast.Person {
	var people []broadcast.Person
	h.mu.Lock()
	for _, person := range h.rooms[boardID] {
		people = append(people, person)
	}
	h.mu.Unlock()
	if h.gateway != nil {
		conns, _ := h.connections(ctx, boardID)
		for _, conn := range conns {
			if conn.person.ID != "" {
				people = append(people, conn.person)
			}
		}
	}
	slices.SortStableFunc(people, func(a, b broadcast.Person) int { return strings.Compare(a.ID, b.ID) })
	people = slices.CompactFunc(people, func(a, b broadcast.Person) bool { return a.ID == b.ID })
	slices.SortStableFunc(people, func(a, b broadcast.Person) int {
		return strings.Compare(strings.ToLower(a.Name)+"\x00"+a.ID, strings.ToLower(b.Name)+"\x00"+b.ID)
	})
	return people
}

type connection struct {
	id     string
	person broadcast.Person
}

// connections lists the board's connections in the table.
func (h *Handler) connections(ctx context.Context, boardID string) ([]connection, error) {
	expr, err := expression.NewBuilder().WithKeyCondition(
		expression.Key("PK").Equal(expression.Value(boardKey(boardID))),
	).WithProjection(expression.NamesList(
		expression.Name("SK"), expression.Name("UserID"), expression.Name("UserName"), expression.Name("Email"),
	)).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		return nil, err
	}

	var conns []connection
	pages := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      expr.Projection(),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			log.Printf("failed to list connections: %v", err)
			return conns, err
		}
		for _, item := range page.Items {
			text := func(name string) string {
				s, _ := item[name].(*types.AttributeValueMemberS)
				if s == nil {
					return ""
				}
				return s.Value
			}
			if id := text("SK"); id != "" {
				conns = append(conns, connection{id, broadcast.Person{ID: text("UserID"), Name: text("UserName"), Email: text("Email")}})
			}
		}
	}
	return conns, nil
}

func (h *Handler) broadcastGateway(ctx context.Context, boardID string, data []byte, except string) {
	conns, _ := h.connections(ctx, boardID)
	var ids []string
	for _, conn := range conns {
		if conn.id != except {
			ids = append(ids, conn.id)
		}
	}

	var wg sync.WaitGroup
	slots := make(chan struct{}, fanOut)
	for _, id := range ids {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			_, err := h.gateway.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
				ConnectionId: aws.String(id),
				Data:         data,
			})
			var gone *gwtypes.GoneException
			if errors.As(err, &gone) {
				h.forget(ctx, boardID, id)
			} else if err != nil {
				log.Printf("failed to send to %s: %v", id, err)
			}
		}()
	}
	wg.Wait()
}
