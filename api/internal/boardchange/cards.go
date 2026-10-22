package boardchange

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/access"
)

// These mirror the key formats the board read (internal/board) expects.
func cardSK(cardID string) string {
	return "CARD#" + cardID
}

func taskSK(cardID, taskID string) string {
	return "TASK#" + cardID + "#" + taskID
}

func taskPrefix(cardID string) string {
	return "TASK#" + cardID + "#"
}

// ordered is the part of a card or task that places it.
type ordered struct {
	Order float64
}

// lastOrder is the order of the last card, or task, whose key starts with
// prefix: 0 when there is none.
func (s *Store) lastOrder(ctx context.Context, boardID, prefix string) (float64, error) {
	keyEx := expression.Key("PK").Equal(expression.Value(boardID)).
		And(expression.Key("SK").BeginsWith(prefix))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return 0, err
	}

	last := 0.0
	pages := dynamodb.NewQueryPaginator(s.db, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		var pageItems []ordered
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
			return 0, err
		}
		for _, item := range pageItems {
			last = max(last, item.Order)
		}
	}
	return last, nil
}

func (s *Store) getOrder(ctx context.Context, boardID, sk string) (float64, bool, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key:       access.Key(boardID, sk),
	})
	if err != nil || out.Item == nil {
		return 0, false, err
	}
	var item ordered
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return 0, false, err
	}
	return item.Order, true, nil
}

func formatOrder(order float64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatFloat(order, 'f', -1, 64)}
}

func (s *Store) createCard(ctx context.Context, c Change) error {
	last, err := s.lastOrder(ctx, c.BoardID, "CARD#")
	if err != nil {
		return err
	}

	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"PK":          &types.AttributeValueMemberS{Value: c.BoardID},
			"SK":          &types.AttributeValueMemberS{Value: cardSK(c.CardID)},
			"ID":          &types.AttributeValueMemberS{Value: c.CardID},
			"Name":        &types.AttributeValueMemberS{Value: c.Name},
			"Description": &types.AttributeValueMemberS{Value: ""},
			"CreatedAt":   &types.AttributeValueMemberS{Value: c.CreatedAt},
			"Order":       formatOrder(last + orderGap),
		},
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if isConditionFailed(err) {
		return refuse(http.StatusConflict, "an item with this id already exists")
	}
	return err
}

func (s *Store) updateCard(ctx context.Context, c Change) error {
	update := expression.Set(expression.Name("Name"), expression.Value(c.Name)).
		Set(expression.Name("Description"), expression.Value(c.Description))
	return s.update(ctx, c.BoardID, cardSK(c.CardID), update, "card not found")
}

// update sets fields of a card or task that must still exist: without the
// condition, updating a deleted one would bring back a broken one.
func (s *Store) update(ctx context.Context, boardID, sk string, update expression.UpdateBuilder, missing string) error {
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return err
	}

	_, err = s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.table),
		Key:                       access.Key(boardID, sk),
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ConditionExpression:       aws.String("attribute_exists(SK)"),
	})
	if isConditionFailed(err) {
		return refuse(http.StatusNotFound, missing)
	}
	return err
}

// between is the order for an item placed after before and ahead of after,
// either of which may be absent.
func (s *Store) between(ctx context.Context, boardID string, before, after *string, sk func(string) string) (float64, error) {
	neighbour := func(id *string) (*float64, error) {
		if id == nil || *id == "" {
			return nil, nil
		}
		order, ok, err := s.getOrder(ctx, boardID, sk(*id))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, refuse(http.StatusNotFound, "neighbour not found")
		}
		return &order, nil
	}
	b, err := neighbour(before)
	if err != nil {
		return 0, err
	}
	a, err := neighbour(after)
	if err != nil {
		return 0, err
	}

	switch {
	case b == nil && a == nil:
		return 0, nil
	case b == nil:
		return *a - orderGap, nil
	case a == nil:
		return *b + orderGap, nil
	default:
		return *b + (*a-*b)/2, nil
	}
}

func (s *Store) moveCard(ctx context.Context, c Change) error {
	if (c.BeforeID == nil || *c.BeforeID == "") && (c.AfterID == nil || *c.AfterID == "") {
		return nil
	}
	order, err := s.between(ctx, c.BoardID, c.BeforeID, c.AfterID, cardSK)
	if err != nil {
		return err
	}
	return s.update(ctx, c.BoardID, cardSK(c.CardID),
		expression.Set(expression.Name("Order"), expression.Value(order)), "card not found")
}

// batchSize is DynamoDB's limit on items in one BatchWriteItem.
const batchSize = 25

// maxBatchAttempts bounds the retries for unprocessed items; the change
// queue retries the whole delete after that.
const maxBatchAttempts = 5

// retryDelay is the wait before the first retry of unprocessed items; it
// doubles with each one, as AWS advises when DynamoDB is throttling.
const retryDelay = 50 * time.Millisecond

// deleteCard deletes the card's tasks in batches, then the card. It isn't
// atomic, which the 100-item transaction limit would make it for small cards
// only: a crash midway leaves a card with some tasks gone, and the retry
// finishes the job. A card that is already gone counts as deleted.
func (s *Store) deleteCard(ctx context.Context, c Change) error {
	keyEx := expression.Key("PK").Equal(expression.Value(c.BoardID)).
		And(expression.Key("SK").BeginsWith(taskPrefix(c.CardID)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return err
	}

	pages := dynamodb.NewQueryPaginator(s.db, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      aws.String("SK"),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}
		for batch := range slices.Chunk(page.Items, batchSize) {
			if err := s.deleteTasks(ctx, c.BoardID, batch); err != nil {
				return err
			}
		}
	}

	_, err = s.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key:       access.Key(c.BoardID, cardSK(c.CardID)),
	})
	return err
}

// deleteTasks deletes the tasks whose keys are given, trying again for those
// DynamoDB leaves unprocessed.
func (s *Store) deleteTasks(ctx context.Context, boardID string, tasks []map[string]types.AttributeValue) error {
	requests := make([]types.WriteRequest, 0, len(tasks))
	for _, task := range tasks {
		requests = append(requests, types.WriteRequest{DeleteRequest: &types.DeleteRequest{
			Key: map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: boardID}, "SK": task["SK"]},
		}})
	}

	for attempt := 0; len(requests) > 0; attempt++ {
		if attempt == maxBatchAttempts {
			return fmt.Errorf("deleting tasks: %d still unprocessed", len(requests))
		}
		if attempt > 0 {
			select {
			case <-time.After(retryDelay << (attempt - 1)):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		out, err := s.db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{s.table: requests},
		})
		if err != nil {
			return err
		}
		requests = out.UnprocessedItems[s.table]
	}
	return nil
}
