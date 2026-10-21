package boardchange

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/access"
)

func (s *Store) createTask(ctx context.Context, c Change) error {
	last, err := s.lastOrder(ctx, c.BoardID, taskPrefix(c.CardID))
	if err != nil {
		return err
	}

	// The card must still exist, or a create sent after the card was
	// deleted (such as an offline change sent late) would leave a task
	// that no card shows.
	_, err = s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				ConditionCheck: &types.ConditionCheck{
					TableName:           aws.String(s.table),
					Key:                 access.Key(c.BoardID, cardSK(c.CardID)),
					ConditionExpression: aws.String("attribute_exists(SK)"),
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(s.table),
					Item: map[string]types.AttributeValue{
						"PK":          &types.AttributeValueMemberS{Value: c.BoardID},
						"SK":          &types.AttributeValueMemberS{Value: taskSK(c.CardID, c.TaskID)},
						"ID":          &types.AttributeValueMemberS{Value: c.TaskID},
						"CardID":      &types.AttributeValueMemberS{Value: c.CardID},
						"Name":        &types.AttributeValueMemberS{Value: c.Name},
						"Description": &types.AttributeValueMemberS{Value: ""},
						"Status":      &types.AttributeValueMemberS{Value: "pending"},
						"Order":       formatOrder(last + orderGap),
					},
					ConditionExpression: aws.String("attribute_not_exists(SK)"),
				},
			},
		},
	})
	if canceledBy(err, 0) {
		return refuse(http.StatusNotFound, "card not found")
	}
	if canceledBy(err, 1) {
		return refuse(http.StatusConflict, "a task with this id already exists")
	}
	return err
}

func (s *Store) updateTask(ctx context.Context, c Change) error {
	update := expression.Set(expression.Name("Name"), expression.Value(c.Name)).
		Set(expression.Name("Description"), expression.Value(c.Description)).
		Set(expression.Name("Status"), expression.Value(c.Status))
	return s.update(ctx, c.BoardID, taskSK(c.CardID, c.TaskID), update, "task not found")
}

// task is what a move carries over to the task's new key.
type task struct {
	Name        string
	Description string
	Status      string
}

// moveTask places a task among the tasks of a card, its own or another. With
// neither neighbour given it lands at order 0.
func (s *Store) moveTask(ctx context.Context, c Change) error {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key:       access.Key(c.BoardID, taskSK(c.CardID, c.TaskID)),
	})
	if err != nil {
		return err
	}
	if out.Item == nil {
		return refuse(http.StatusNotFound, "task not found")
	}
	var t task
	if err := attributevalue.UnmarshalMap(out.Item, &t); err != nil {
		return err
	}

	order, err := s.between(ctx, c.BoardID, c.BeforeID, c.AfterID, func(id string) string {
		return taskSK(c.NewCardID, id)
	})
	if err != nil {
		return err
	}

	if c.NewCardID == c.CardID {
		// Same card: TransactWriteItems refuses two operations on one key,
		// so the delete and put below can't be used when the key (which
		// embeds the card) isn't changing.
		return s.update(ctx, c.BoardID, taskSK(c.CardID, c.TaskID),
			expression.Set(expression.Name("Order"), expression.Value(order)), "task not found")
	}

	// Another card: the key embeds the card, so moving means delete + put,
	// in one transaction so a task is never dropped if the second step
	// fails. Both the task and the card it goes to must still be there, or
	// the move would bring back a deleted task or leave it in a deleted card.
	_, err = s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Delete: &types.Delete{
					TableName:           aws.String(s.table),
					Key:                 access.Key(c.BoardID, taskSK(c.CardID, c.TaskID)),
					ConditionExpression: aws.String("attribute_exists(SK)"),
				},
			},
			{
				ConditionCheck: &types.ConditionCheck{
					TableName:           aws.String(s.table),
					Key:                 access.Key(c.BoardID, cardSK(c.NewCardID)),
					ConditionExpression: aws.String("attribute_exists(SK)"),
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(s.table),
					Item: map[string]types.AttributeValue{
						"PK":          &types.AttributeValueMemberS{Value: c.BoardID},
						"SK":          &types.AttributeValueMemberS{Value: taskSK(c.NewCardID, c.TaskID)},
						"ID":          &types.AttributeValueMemberS{Value: c.TaskID},
						"CardID":      &types.AttributeValueMemberS{Value: c.NewCardID},
						"Name":        &types.AttributeValueMemberS{Value: t.Name},
						"Description": &types.AttributeValueMemberS{Value: t.Description},
						"Status":      &types.AttributeValueMemberS{Value: t.Status},
						"Order":       formatOrder(order),
					},
				},
			},
		},
	})
	if canceledBy(err, 0) {
		return refuse(http.StatusNotFound, "task not found")
	}
	if canceledBy(err, 1) {
		return refuse(http.StatusNotFound, "card not found")
	}
	return err
}

func (s *Store) deleteTask(ctx context.Context, c Change) error {
	_, err := s.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key:       access.Key(c.BoardID, taskSK(c.CardID, c.TaskID)),
	})
	return err
}

// cleanUp deletes every done task on the board.
func (s *Store) cleanUp(ctx context.Context, c Change) error {
	expr, err := expression.NewBuilder().
		WithKeyCondition(expression.Key("PK").Equal(expression.Value(c.BoardID)).
			And(expression.Key("SK").BeginsWith("TASK#"))).
		WithFilter(expression.Name("Status").Equal(expression.Value("done"))).
		WithProjection(expression.NamesList(expression.Name("PK"), expression.Name("SK"))).
		Build()
	if err != nil {
		return err
	}
	pages := dynamodb.NewQueryPaginator(s.db, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ProjectionExpression:      expr.Projection(),
	})

	var deletes []types.WriteRequest
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			deletes = append(deletes, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{Key: map[string]types.AttributeValue{"PK": item["PK"], "SK": item["SK"]}},
			})
		}
	}

	for len(deletes) > 0 {
		batch := deletes[:min(25, len(deletes))]
		deletes = deletes[len(batch):]

		out, err := s.db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{s.table: batch},
		})
		if err != nil {
			return err
		}
		deletes = append(deletes, out.UnprocessedItems[s.table]...)
	}
	return nil
}
