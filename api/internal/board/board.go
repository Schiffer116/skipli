package board

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/auth"
)

const memberIndexName = "Member"

type Handler struct {
	db     *dynamodb.Client
	table  string
	tokens *auth.TokenVerifier
}

func NewHandler(db *dynamodb.Client, table string, tokens *auth.TokenVerifier) *Handler {
	return &Handler{db: db, table: table, tokens: tokens}
}

func (h *Handler) RegisterRoutes(router *http.ServeMux) {
	router.HandleFunc("GET /boards", h.tokens.RequireAuth(h.list))
	router.HandleFunc("GET /boards/{id}", h.get)
	router.HandleFunc("GET /boards/{id}/members", h.tokens.RequireAuth(h.members))
	router.HandleFunc("POST /boards", h.tokens.RequireAuth(h.create))
	router.HandleFunc("PUT /boards/order", h.tokens.RequireAuth(h.order))
	router.HandleFunc("PUT /boards/{id}", h.tokens.RequireAuth(h.put))
	router.HandleFunc("PUT /boards/{id}/favorite", h.tokens.RequireAuth(h.favorite))
	router.HandleFunc("POST /boards/{id}/invite", h.tokens.RequireAuth(h.invite))
	router.HandleFunc("DELETE /boards/{id}", h.tokens.RequireAuth(h.delete))
}

type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) create(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	owner, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}
	log.Println("owner's email:", owner)

	var r CreateRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	log.Printf("got: %+v", r)

	id := uuid.New().String()
	_, err := h.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName: aws.String(h.table),
					Item: map[string]types.AttributeValue{
						"PK":          &types.AttributeValueMemberS{Value: id},
						"SK":          &types.AttributeValueMemberS{Value: "META"},
						"Owner":       &types.AttributeValueMemberS{Value: owner},
						"ID":          &types.AttributeValueMemberS{Value: id},
						"Name":        &types.AttributeValueMemberS{Value: r.Name},
						"Description": &types.AttributeValueMemberS{Value: r.Description},
					},
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(h.table),
					Item: map[string]types.AttributeValue{
						"PK":     &types.AttributeValueMemberS{Value: id},
						"SK":     &types.AttributeValueMemberS{Value: "MEMBER#" + owner},
						"Member": &types.AttributeValueMemberS{Value: owner},
						"Order":  &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().UnixMilli(), 10)},
					},
				},
			},
		},
	})

	if err != nil {
		log.Printf("failed to create board: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(r)
}

func (h *Handler) get(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	id := req.PathValue("id")

	response, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: id},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		log.Printf("failed to get item: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if response.Item == nil {
		http.NotFound(w, req)
		return
	}

	var board Board
	if err := attributevalue.UnmarshalMap(response.Item, &board); err != nil {
		log.Printf("failed to unmarshal item: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(board)
}

func (h *Handler) members(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := req.PathValue("id")

	response, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: id},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		log.Printf("failed to get board: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if response.Item == nil {
		http.NotFound(w, req)
		return
	}

	keyEx := expression.Key("PK").Equal(expression.Value(id)).
		And(expression.Key("SK").BeginsWith("MEMBER#"))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	queryOutput := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})

	members := []string{}
	for queryOutput.HasMorePages() {
		page, err := queryOutput.NextPage(ctx)
		if err != nil {
			log.Printf("failed to list members: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var pageMembers []struct{ Member string }
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageMembers); err != nil {
			log.Printf("failed to unmarshal members: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, m := range pageMembers {
			members = append(members, m.Member)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(members)
}

type Board struct {
	ID          string `json:"id"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Favorite    bool   `json:"favorite" dynamodbav:"-"`
}

type boardItem struct {
	Board
	PK         string
	SK         string
	Order      float64
	IsFavorite bool `dynamodbav:"Favorite"`
}

type ListResponse struct {
	Boards     []Board `json:"boards"`
	TeamBoards []Board `json:"teamBoards"`
}

func (h *Handler) list(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	email, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}

	keyEx := expression.Key("Member").Equal(expression.Value(email))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	queryOutput := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		IndexName:                 aws.String(memberIndexName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      aws.String("PK"),
	})

	var keys []map[string]types.AttributeValue
	for queryOutput.HasMorePages() {
		page, err := queryOutput.NextPage(ctx)
		if err != nil {
			log.Printf("failed to get next page for boards: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, item := range page.Items {
			keys = append(keys,
				map[string]types.AttributeValue{
					"PK": item["PK"],
					"SK": &types.AttributeValueMemberS{Value: "META"},
				},
				map[string]types.AttributeValue{
					"PK": item["PK"],
					"SK": &types.AttributeValueMemberS{Value: "MEMBER#" + email},
				},
			)
		}
	}

	// The Member index is KEYS_ONLY, so it only gives back which boards this
	// member belongs to — fetch each board's META item for the full data, and
	// the caller's own MEMBER# item for their Order. BatchGetItem caps at 100
	// keys per call (so 50 boards, at two keys each) and can return
	// UnprocessedKeys under throttling; neither is handled here yet.
	var items []boardItem
	if len(keys) > 0 {
		batchOutput, err := h.db.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				h.table: {Keys: keys},
			},
		})
		if err != nil {
			log.Printf("failed to batch get boards: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := attributevalue.UnmarshalListOfMaps(batchOutput.Responses[h.table], &items); err != nil {
			log.Printf("failed to unmarshal boards: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	memberBoards := []Board{}
	orders := map[string]float64{}
	favorites := map[string]bool{}
	for _, item := range items {
		if item.SK == "META" {
			memberBoards = append(memberBoards, item.Board)
		} else {
			orders[item.PK] = item.Order
			favorites[item.PK] = item.IsFavorite
		}
	}
	for i := range memberBoards {
		memberBoards[i].Favorite = favorites[memberBoards[i].ID]
	}

	// Memberships from before Order existed read as 0 and sort first; name
	// breaks ties so they at least come back in a stable order.
	sort.Slice(memberBoards, func(i, j int) bool {
		a, b := memberBoards[i], memberBoards[j]
		if orders[a.ID] != orders[b.ID] {
			return orders[a.ID] < orders[b.ID]
		}
		return a.Name < b.Name
	})

	// Split into non-overlapping sets: boards the caller owns, and boards
	// they're just a member of. Unlike the old API's two overlapping
	// Firestore queries, an owned board appears only in boards here.
	boards := []Board{}
	teamBoards := []Board{}
	for _, b := range memberBoards {
		if b.Owner == email {
			boards = append(boards, b)
		} else {
			teamBoards = append(teamBoards, b)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ListResponse{Boards: boards, TeamBoards: teamBoards})
}

type OrderRequest struct {
	BoardIDs []string `json:"boardIds"`
}

// order saves the caller's own ordering of their boards, as the full list of
// ids for one section of the workspace page (owned or team). Each id's
// position becomes the Order on the caller's MEMBER# item for that board, so
// every member keeps their own order. Rewriting the whole section rather
// than a single before/after midpoint (like cards) keeps it correct for
// memberships that predate Order, which would all tie at 0.
func (h *Handler) order(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	email, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}

	var r OrderRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	if len(r.BoardIDs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// TransactWriteItems' cap; same unhandled-at-scale caveat as list.
	if len(r.BoardIDs) > 100 {
		http.Error(w, "too many boards", http.StatusBadRequest)
		return
	}

	// A transaction can't touch the same item twice.
	// This is questionable design.
	seen := map[string]bool{}
	for _, id := range r.BoardIDs {
		if seen[id] {
			http.Error(w, "duplicate board id", http.StatusBadRequest)
			return
		}
		seen[id] = true
	}

	transactItems := make([]types.TransactWriteItem, 0, len(r.BoardIDs))
	for i, id := range r.BoardIDs {
		// The condition matters: UpdateItem on a missing key would create
		// the item, i.e. quietly make the caller a member of any board id
		// they send.
		transactItems = append(transactItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName: aws.String(h.table),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: id},
					"SK": &types.AttributeValueMemberS{Value: "MEMBER#" + email},
				},
				UpdateExpression:    aws.String("SET #order = :order"),
				ConditionExpression: aws.String("attribute_exists(PK)"),
				ExpressionAttributeNames: map[string]string{
					"#order": "Order",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":order": &types.AttributeValueMemberN{Value: strconv.Itoa(i)},
				},
			},
		})
	}

	_, err := h.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			for _, reason := range canceled.CancellationReasons {
				if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
					http.Error(w, "not a member of every board", http.StatusForbidden)
					return
				}
			}
		}
		log.Printf("failed to reorder boards: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type FavoriteRequest struct {
	Favorite bool `json:"favorite"`
}

func (h *Handler) favorite(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	email, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}

	var r FavoriteRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	_, err := h.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: req.PathValue("id")},
			"SK": &types.AttributeValueMemberS{Value: "MEMBER#" + email},
		},
		UpdateExpression:    aws.String("SET #favorite = :favorite"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeNames: map[string]string{
			"#favorite": "Favorite",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":favorite": &types.AttributeValueMemberBOOL{Value: r.Favorite},
		},
	})
	var notMember *types.ConditionalCheckFailedException
	if errors.As(err, &notMember) {
		http.Error(w, "not a member of this board", http.StatusForbidden)
		return
	}
	if err != nil {
		log.Printf("failed to set favorite: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) isMember(ctx context.Context, boardID, email string) (bool, error) {
	out, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: boardID},
			"SK": &types.AttributeValueMemberS{Value: "MEMBER#" + email},
		},
		ProjectionExpression: aws.String("PK"),
	})
	if err != nil {
		return false, err
	}
	return out.Item != nil, nil
}

type InviteRequest struct {
	Email string `json:"email"`
}

func (h *Handler) invite(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := req.PathValue("id")

	email, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}

	var r InviteRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	invitee := strings.TrimSpace(r.Email)
	if invitee == "" {
		http.Error(w, "missing email", http.StatusBadRequest)
		return
	}

	member, err := h.isMember(ctx, id, email)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !member {
		http.Error(w, "not a member of this board", http.StatusForbidden)
		return
	}

	_, err = h.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(h.table),
		Item: map[string]types.AttributeValue{
			"PK":     &types.AttributeValueMemberS{Value: id},
			"SK":     &types.AttributeValueMemberS{Value: "MEMBER#" + invitee},
			"Member": &types.AttributeValueMemberS{Value: invitee},
			"Order":  &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().UnixMilli(), 10)},
		},
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	var alreadyMember *types.ConditionalCheckFailedException
	if err != nil && !errors.As(err, &alreadyMember) {
		log.Printf("failed to invite member: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) delete(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := req.PathValue("id")

	email, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}

	meta, err := h.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: id},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		log.Printf("failed to get board: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if meta.Item == nil {
		http.NotFound(w, req)
		return
	}
	var board Board
	if err := attributevalue.UnmarshalMap(meta.Item, &board); err != nil {
		log.Printf("failed to unmarshal board: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if board.Owner != email {
		http.Error(w, "only the owner can delete this board", http.StatusForbidden)
		return
	}

	keyEx := expression.Key("PK").Equal(expression.Value(id))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("failed to build query expression: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pages := dynamodb.NewQueryPaginator(h.db, &dynamodb.QueryInput{
		TableName:                 aws.String(h.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      aws.String("PK, SK"),
	})

	var deletes []types.WriteRequest
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			log.Printf("failed to list board items: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, item := range page.Items {
			deletes = append(deletes, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{Key: item},
			})
		}
	}

	for len(deletes) > 0 {
		batch := deletes[:min(25, len(deletes))]
		deletes = deletes[len(batch):]

		out, err := h.db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{h.table: batch},
		})
		if err != nil {
			log.Printf("failed to delete board items: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		deletes = append(deletes, out.UnprocessedItems[h.table]...)
	}

	w.WriteHeader(http.StatusNoContent)
}

type PutBoardRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) put(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	id := req.PathValue("id")

	email, ok := auth.EmailFromContext(ctx)
	if !ok {
		http.Error(w, "missing email in context", http.StatusInternalServerError)
		return
	}

	var r PutBoardRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		log.Printf("failed to decode request body: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	member, err := h.isMember(ctx, id, email)
	if err != nil {
		log.Printf("failed to check membership: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !member {
		http.Error(w, "not a member of this board", http.StatusForbidden)
		return
	}

	_, err = h.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(h.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: id},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
		UpdateExpression: aws.String("SET #name = :name, #description = :description"),
		ExpressionAttributeNames: map[string]string{
			"#name":        "Name",
			"#description": "Description",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":name":        &types.AttributeValueMemberS{Value: r.Name},
			":description": &types.AttributeValueMemberS{Value: r.Description},
		},
	})
	if err != nil {
		log.Printf("failed to update board: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
