package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

	"github.com/Schiffer116/skipli/api/internal/auth"
)

const connectionLifetime = 2 * time.Hour

type Handler struct {
	cfg    aws.Config
	db     *dynamodb.Client
	table  string
	tokens *auth.TokenVerifier

	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]struct{}
}

func NewHandler(cfg aws.Config, db *dynamodb.Client, table string, tokens *auth.TokenVerifier) *Handler {
	return &Handler{cfg: cfg, db: db, table: table, tokens: tokens, rooms: map[string]map[*websocket.Conn]struct{}{}}
}

func (h *Handler) Handle(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.RequestContext.RouteKey == "$connect" {
		return h.connect(ctx, req), nil
	}
	return h.relay(ctx, req), nil
}

func status(code int) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: code}
}

func key(pk, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: sk},
	}
}

func (h *Handler) exists(ctx context.Context, pk, sk string) (bool, error) {
	out, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:            aws.String(h.table),
		Key:                  key(pk, sk),
		ProjectionExpression: aws.String("PK"),
	})
	if err != nil {
		return false, err
	}
	return out.Item != nil, nil
}

func (h *Handler) connect(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) events.APIGatewayProxyResponse {
	var cookies string
	for name, value := range req.Headers {
		if strings.EqualFold(name, "Cookie") {
			cookies = value
		}
	}
	user, err := h.tokens.VerifyCookies(cookies)
	if err != nil {
		return status(401)
	}

	boardID := req.QueryStringParameters["boardId"]
	member, err := h.exists(ctx, boardID, "MEMBER#"+user.ID)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		return status(500)
	}
	if !member {
		return status(403)
	}

	item := key(boardID, "CONN#"+req.RequestContext.ConnectionID)
	item["ExpiresAt"] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(time.Now().Add(connectionLifetime).Unix(), 10),
	}
	if _, err := h.db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(h.table), Item: item}); err != nil {
		log.Printf("failed to store connection: %v", err)
		return status(500)
	}
	return status(200)
}

type message struct {
	BoardID string          `json:"boardId"`
	Event   string          `json:"event"`
	Args    json.RawMessage `json:"args"`
}

func (h *Handler) relay(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) events.APIGatewayProxyResponse {
	var msg message
	if err := json.Unmarshal([]byte(req.Body), &msg); err != nil || msg.Event == "" {
		return status(400)
	}

	sender := req.RequestContext.ConnectionID
	connected, err := h.exists(ctx, msg.BoardID, "CONN#"+sender)
	if err != nil {
		log.Printf("failed to check connection: %v", err)
		return status(500)
	}
	if !connected {
		return status(403)
	}

	data, err := json.Marshal(struct {
		Event string          `json:"event"`
		Args  json.RawMessage `json:"args"`
	}{msg.Event, msg.Args})
	if err != nil {
		return status(400)
	}

	expr, err := expression.NewBuilder().WithKeyCondition(
		expression.Key("PK").Equal(expression.Value(msg.BoardID)).
			And(expression.Key("SK").BeginsWith("CONN#")),
	).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		return status(500)
	}

	gateway := apigatewaymanagementapi.NewFromConfig(h.cfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(fmt.Sprintf("https://%s/%s", req.RequestContext.DomainName, req.RequestContext.Stage))
	})

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
			return status(500)
		}
		for _, item := range page.Items {
			sk := item["SK"].(*types.AttributeValueMemberS).Value
			id := strings.TrimPrefix(sk, "CONN#")
			if id == sender {
				continue
			}

			_, err := gateway.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
				ConnectionId: aws.String(id),
				Data:         data,
			})
			var gone *gwtypes.GoneException
			if errors.As(err, &gone) {
				h.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(h.table), Key: key(msg.BoardID, sk)})
			} else if err != nil {
				log.Printf("failed to send to %s: %v", id, err)
			}
		}
	}
	return status(200)
}
