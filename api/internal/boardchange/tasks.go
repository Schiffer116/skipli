package boardchange

import (
	"context"
	"net/http"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Schiffer116/skipli/api/internal/access"
)

// findTask finds a task by its id, wherever it is now: a task's key names its
// card, and a teammate may have moved it to another since the change was
// made. cardID is where the change saw it, tried first. ok is false when the
// task is gone.
func (s *Store) findTask(ctx context.Context, boardID, cardID, taskID string) (t sibling, ok bool, err error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key:       access.Key(boardID, taskSK(cardID, taskID)),
	})
	if err != nil {
		return sibling{}, false, err
	}
	if out.Item != nil {
		err = attributevalue.UnmarshalMap(out.Item, &t)
		return t, err == nil, err
	}

	expr, err := expression.NewBuilder().
		WithKeyCondition(expression.Key("PK").Equal(expression.Value(boardID)).
			And(expression.Key("SK").BeginsWith("TASK#"))).
		WithFilter(expression.Name("ID").Equal(expression.Value(taskID))).
		Build()
	if err != nil {
		return sibling{}, false, err
	}
	pages := dynamodb.NewQueryPaginator(s.db, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return sibling{}, false, err
		}
		if len(page.Items) > 0 {
			err = attributevalue.UnmarshalMap(page.Items[0], &t)
			return t, err == nil, err
		}
	}
	return sibling{}, false, nil
}

func (s *Store) createTask(ctx context.Context, c Change) error {
	// A retried create whose task a teammate has since moved to another
	// card is already saved, not a second task.
	if _, ok, err := s.findTask(ctx, c.BoardID, c.CardID, c.TaskID); err != nil || ok {
		if ok {
			return refuse(http.StatusConflict, "a task with this id already exists")
		}
		return err
	}
	tasks, err := s.siblings(ctx, c.BoardID, taskPrefix(c.CardID))
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
						"Name":        &types.AttributeValueMemberS{Value: value(c.Name)},
						"Description": &types.AttributeValueMemberS{Value: ""},
						"Status":      &types.AttributeValueMemberS{Value: "pending"},
						"Order":       formatOrder(place(tasks, c.TaskID, nil, nil)),
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
	t, ok, err := s.findTask(ctx, c.BoardID, c.CardID, c.TaskID)
	if err != nil {
		return err
	}
	if !ok {
		return refuse(http.StatusNotFound, "task not found")
	}
	return s.update(ctx, c.BoardID, t.SK, fields(c), "task not found")
}

// moveTask places a task among the tasks of a card, its own or another,
// from whichever card it is in now.
func (s *Store) moveTask(ctx context.Context, c Change) error {
	t, ok, err := s.findTask(ctx, c.BoardID, c.CardID, c.TaskID)
	if err != nil {
		return err
	}
	if !ok {
		return refuse(http.StatusNotFound, "task not found")
	}
	tasks, err := s.siblings(ctx, c.BoardID, taskPrefix(c.NewCardID))
	if err != nil {
		return err
	}
	order := place(tasks, c.TaskID, c.BeforeID, c.AfterID)

	if c.NewCardID == t.CardID {
		// Same card: TransactWriteItems refuses two operations on one key,
		// so the delete and put below can't be used when the key (which
		// embeds the card) isn't changing.
		return s.update(ctx, c.BoardID, t.SK,
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
					Key:                 access.Key(c.BoardID, t.SK),
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

// deleteTask deletes a task from whichever card it is in now. One already
// gone counts as deleted.
func (s *Store) deleteTask(ctx context.Context, c Change) error {
	t, ok, err := s.findTask(ctx, c.BoardID, c.CardID, c.TaskID)
	if err != nil || !ok {
		return err
	}
	_, err = s.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key:       access.Key(c.BoardID, t.SK),
	})
	return err
}

// cleanUp deletes the tasks that were done on screen when it was made,
// wherever they are now, and no others: not one a teammate marked done
// meanwhile.
func (s *Store) cleanUp(ctx context.Context, c Change) error {
	tasks, err := s.siblings(ctx, c.BoardID, "TASK#")
	if err != nil {
		return err
	}

	var deletes []types.WriteRequest
	for _, t := range tasks {
		if slices.Contains(c.TaskIDs, t.ID) {
			deletes = append(deletes, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{Key: access.Key(c.BoardID, t.SK)},
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
