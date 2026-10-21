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
	// gateway reaches the connections of the API Gateway stage, or is nil
	// when the connections are the local relay's.
	gateway Gateway

	// The local relay's connections, by board.
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]struct{}
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
		db:      db,
		table:   table,
		tokens:  tokens,
		gateway: gateway,
		rooms:   map[string]map[*websocket.Conn]struct{}{},
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

	id := req.RequestContext.ConnectionID
	expires := &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Add(connectionLifetime).Unix(), 10)}
	record := access.Key(boardKey(boardID), id)
	record["ExpiresAt"] = expires
	pointer := access.Key(connKey(id), "BOARD")
	pointer["Board"] = &types.AttributeValueMemberS{Value: boardID}
	pointer["ExpiresAt"] = expires
	for _, item := range []map[string]types.AttributeValue{record, pointer} {
		if _, err := h.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(h.table), Item: item}); err != nil {
			log.Printf("failed to store connection: %v", err)
			return http.StatusInternalServerError
		}
	}
	return http.StatusOK
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
	ctx, cancel := context.WithTimeout(ctx, broadcastTimeout)
	defer cancel()

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("failed to encode message: %v", err)
		return
	}

	h.broadcastLocal(ctx, boardID, data)
	if h.gateway != nil {
		h.broadcastGateway(ctx, boardID, data)
	}
}

func (h *Handler) broadcastGateway(ctx context.Context, boardID string, data []byte) {
	expr, err := expression.NewBuilder().WithKeyCondition(
		expression.Key("PK").Equal(expression.Value(boardKey(boardID))),
	).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		return
	}

	var ids []string
	pages := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      aws.String("SK"),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			log.Printf("failed to list connections: %v", err)
			break
		}
		for _, item := range page.Items {
			if sk, _ := item["SK"].(*types.AttributeValueMemberS); sk != nil {
				ids = append(ids, sk.Value)
			}
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
