// Package realtime is the API behind a board's WebSocket. The app sends its
// Board changes there; each one is checked, applied, answered with an ack,
// and broadcast to the board's other connections once accepted.
package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/Schiffer116/skipli/api/internal/boardchange"
)

// API Gateway closes a WebSocket after two hours whatever happens, so a
// connection's record never needs to outlive that.
const connectionLifetime = 2 * time.Hour

// Gateway sends to API Gateway connections; tests stand in for it.
type Gateway interface {
	PostToConnection(context.Context, *apigatewaymanagementapi.PostToConnectionInput, ...func(*apigatewaymanagementapi.Options)) (*apigatewaymanagementapi.PostToConnectionOutput, error)
}

type Handler struct {
	db      *dynamodb.Client
	table   string
	tokens  *auth.TokenVerifier
	changes *boardchange.Store
	// gateway reaches the connections of the API Gateway stage at endpoint.
	gateway func(endpoint string) Gateway

	// The local relay's connections, by board.
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]struct{}
}

func NewHandler(cfg aws.Config, db *dynamodb.Client, table string, tokens *auth.TokenVerifier) *Handler {
	return newHandler(db, table, tokens, func(endpoint string) Gateway {
		return apigatewaymanagementapi.NewFromConfig(cfg, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	})
}

func newHandler(db *dynamodb.Client, table string, tokens *auth.TokenVerifier, gateway func(string) Gateway) *Handler {
	return &Handler{
		db:      db,
		table:   table,
		tokens:  tokens,
		changes: boardchange.NewStore(db, table),
		gateway: gateway,
		rooms:   map[string]map[*websocket.Conn]struct{}{},
	}
}

// message is what the app sends on a board's socket: a Board change to
// apply, under an id the ack repeats, or a ping that only keeps an idle
// connection open.
type message struct {
	Action string             `json:"action"`
	ID     string             `json:"id"`
	Change boardchange.Change `json:"change"`
}

// ack answers a change: status 200 when it was applied, otherwise the HTTP
// status the same change would have been refused with.
type ack struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

// broadcast carries an accepted change to the board's other connections.
type broadcast struct {
	Type   string             `json:"type"`
	Change boardchange.Change `json:"change"`
}

// receive handles one message. allowed answers whether the sender may change
// the board a change names, as an HTTP status. It returns the reply for the
// sender (nil for none) and, when a change was applied, the broadcast that
// carries it to the board's other connections.
func (h *Handler) receive(ctx context.Context, body []byte, allowed func(boardID string) (int, error)) (reply []byte, accepted *broadcast) {
	var msg message
	err := json.Unmarshal(body, &msg)
	if err == nil && msg.Action == "ping" {
		return nil, nil
	}
	answer := func(status int, text string) []byte {
		out, _ := json.Marshal(ack{Type: "ack", ID: msg.ID, Status: status, Error: text})
		return out
	}
	if err != nil || msg.Action != "change" {
		return answer(http.StatusBadRequest, "not a change"), nil
	}

	status, err := allowed(msg.Change.BoardID)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		return answer(http.StatusInternalServerError, "failed to check membership"), nil
	}
	if status != http.StatusOK {
		return answer(status, http.StatusText(status)), nil
	}

	if err := h.changes.Apply(ctx, msg.Change); err != nil {
		status := boardchange.Status(err)
		if status == http.StatusInternalServerError {
			log.Printf("failed to apply %s: %v", msg.Change.Type, err)
			return answer(status, "failed to apply change"), nil
		}
		return answer(status, err.Error()), nil
	}

	return answer(http.StatusOK, ""), &broadcast{Type: "change", Change: msg.Change}
}

// Handle serves the board WebSocket API Gateway. Its $default route sends the
// returned body back to the sender, which is how a change gets its ack.
func (h *Handler) Handle(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.RequestContext.RouteKey == "$connect" {
		return status(h.connect(ctx, req)), nil
	}

	sender := req.RequestContext.ConnectionID
	reply, accepted := h.receive(ctx, []byte(req.Body), func(boardID string) (int, error) {
		return h.connectionAllowed(ctx, boardID, sender)
	})
	if accepted != nil {
		endpoint := fmt.Sprintf("https://%s/%s", req.RequestContext.DomainName, req.RequestContext.Stage)
		h.broadcast(ctx, h.gateway(endpoint), sender, accepted)
	}
	return events.APIGatewayProxyResponse{StatusCode: http.StatusOK, Body: string(reply)}, nil
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
// connection under the board so changes reach it.
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

	item := access.Key(boardID, "CONN#"+req.RequestContext.ConnectionID)
	item["User"] = &types.AttributeValueMemberS{Value: user.ID}
	item["ExpiresAt"] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(time.Now().Add(connectionLifetime).Unix(), 10),
	}
	if _, err := h.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(h.table), Item: item}); err != nil {
		log.Printf("failed to store connection: %v", err)
		return http.StatusInternalServerError
	}
	return http.StatusOK
}

// connectionAllowed answers whether the person behind a connection may
// change a board: the connection must have been opened on that board, and
// they must still be a member of it.
func (h *Handler) connectionAllowed(ctx context.Context, boardID, connectionID string) (int, error) {
	if boardID == "" {
		return http.StatusBadRequest, nil
	}
	out, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key:       access.Key(boardID, "CONN#"+connectionID),
	})
	if err != nil {
		return 0, err
	}
	user, _ := out.Item["User"].(*types.AttributeValueMemberS)
	if user == nil {
		return access.Missing(ctx, h.db, h.table, boardID)
	}
	return access.Check(ctx, h.db, h.table, boardID, user.Value)
}

// broadcast sends an accepted change to every connection on its board but
// the sender's, forgetting connections API Gateway says are gone.
func (h *Handler) broadcast(ctx context.Context, gateway Gateway, sender string, accepted *broadcast) {
	boardID := accepted.Change.BoardID
	data, err := json.Marshal(accepted)
	if err != nil {
		log.Printf("failed to encode change: %v", err)
		return
	}
	expr, err := expression.NewBuilder().WithKeyCondition(
		expression.Key("PK").Equal(expression.Value(boardID)).
			And(expression.Key("SK").BeginsWith("CONN#")),
	).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		return
	}

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
			return
		}
		for _, item := range page.Items {
			sk, _ := item["SK"].(*types.AttributeValueMemberS)
			if sk == nil {
				continue
			}
			id := strings.TrimPrefix(sk.Value, "CONN#")
			if id == sender {
				continue
			}

			_, err := gateway.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
				ConnectionId: aws.String(id),
				Data:         data,
			})
			var gone *gwtypes.GoneException
			if errors.As(err, &gone) {
				h.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(h.table), Key: access.Key(boardID, sk.Value)})
			} else if err != nil {
				log.Printf("failed to send to %s: %v", id, err)
			}
		}
	}
}
