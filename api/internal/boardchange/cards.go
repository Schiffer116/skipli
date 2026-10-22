package boardchange

import (
	"cmp"
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

// sibling is a card, or a task, as far as placing things among its
// siblings goes.
type sibling struct {
	SK          string
	ID          string
	CardID      string
	Order       float64
	Name        string
	Description string
	Status      string
}

// siblings are the cards, or tasks, whose keys start with prefix, in the
// order the board read shows them: by order, ties in key order.
func (s *Store) siblings(ctx context.Context, boardID, prefix string) ([]sibling, error) {
	keyEx := expression.Key("PK").Equal(expression.Value(boardID)).
		And(expression.Key("SK").BeginsWith(prefix))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, err
	}

	var items []sibling
	pages := dynamodb.NewQueryPaginator(s.db, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		var pageItems []sibling
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
	}
	slices.SortStableFunc(items, func(a, b sibling) int { return cmp.Compare(a.Order, b.Order) })
	return items, nil
}

// place is the order for id among its siblings: right after the one before
// it, else right before the one after it, else at the end. A neighbour that
// was moved away or deleted is never a reason to refuse a move; apply.ts
// places things the same way.
func place(siblings []sibling, id string, beforeID, afterID *string) float64 {
	others := slices.DeleteFunc(slices.Clone(siblings), func(other sibling) bool { return other.ID == id })
	at := func(neighbour *string) int {
		if neighbour == nil {
			return -1
		}
		return slices.IndexFunc(others, func(other sibling) bool { return other.ID == *neighbour })
	}

	if i := at(beforeID); i != -1 {
		if i+1 < len(others) {
			return others[i].Order + (others[i+1].Order-others[i].Order)/2
		}
		return others[i].Order + orderGap
	}
	if i := at(afterID); i != -1 {
		if i > 0 {
			return others[i-1].Order + (others[i].Order-others[i-1].Order)/2
		}
		return others[i].Order - orderGap
	}
	if len(others) == 0 {
		return orderGap
	}
	return others[len(others)-1].Order + orderGap
}

func formatOrder(order float64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatFloat(order, 'f', -1, 64)}
}

func (s *Store) createCard(ctx context.Context, c Change) error {
	cards, err := s.siblings(ctx, c.BoardID, "CARD#")
	if err != nil {
		return err
	}

	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"PK":          &types.AttributeValueMemberS{Value: c.BoardID},
			"SK":          &types.AttributeValueMemberS{Value: cardSK(c.CardID)},
			"ID":          &types.AttributeValueMemberS{Value: c.CardID},
			"Name":        &types.AttributeValueMemberS{Value: value(c.Name)},
			"Description": &types.AttributeValueMemberS{Value: ""},
			"CreatedAt":   &types.AttributeValueMemberS{Value: c.CreatedAt},
			"Order":       formatOrder(place(cards, c.CardID, nil, nil)),
		},
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if isConditionFailed(err) {
		return refuse(http.StatusConflict, "an item with this id already exists")
	}
	return err
}

func (s *Store) updateCard(ctx context.Context, c Change) error {
	return s.update(ctx, c.BoardID, cardSK(c.CardID), fields(c), "card not found")
}

// fields sets the fields an update changed, and only those.
func fields(c Change) expression.UpdateBuilder {
	var update expression.UpdateBuilder
	for name, value := range map[string]*string{"Name": c.Name, "Description": c.Description, "Status": c.Status} {
		if value != nil {
			update = update.Set(expression.Name(name), expression.Value(*value))
		}
	}
	return update
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

func (s *Store) moveCard(ctx context.Context, c Change) error {
	cards, err := s.siblings(ctx, c.BoardID, "CARD#")
	if err != nil {
		return err
	}
	order := place(cards, c.CardID, c.BeforeID, c.AfterID)
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
