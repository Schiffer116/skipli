package auth

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// SendCooldown is how long an address must wait between sign-in emails.
const SendCooldown = time.Minute

// Cooldown records when each address was last sent a sign-in email, as a
// DynamoDB item that the table's TTL eventually removes.
type Cooldown struct {
	db     *dynamodb.Client
	table  string
	window time.Duration
}

func NewCooldown(db *dynamodb.Client, table string) *Cooldown {
	return &Cooldown{db: db, table: table, window: SendCooldown}
}

func cooldownKey(email string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "EMAIL#" + email},
		"SK": &types.AttributeValueMemberS{Value: "SEND_COOLDOWN"},
	}
}

// Start begins a cooldown for email. If one is already running it returns
// false and how long remains. TTL deletion lags, so an expired item that still
// exists is treated as absent.
func (c *Cooldown) Start(ctx context.Context, email string) (bool, time.Duration, error) {
	now := time.Now()
	item := cooldownKey(email)
	item["ExpiresAt"] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(now.Add(c.window).Unix(), 10),
	}

	_, err := c.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(c.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) OR ExpiresAt <= :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Unix(), 10)},
		},
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	var running *types.ConditionalCheckFailedException
	if errors.As(err, &running) {
		remaining := c.window
		if v, ok := running.Item["ExpiresAt"].(*types.AttributeValueMemberN); ok {
			if expires, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
				remaining = time.Duration(expires-now.Unix()) * time.Second
			}
		}
		return false, max(remaining, time.Second), nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, 0, nil
}

// Cancel ends a cooldown early, for when the email it guarded was never sent.
func (c *Cooldown) Cancel(ctx context.Context, email string) error {
	_, err := c.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(c.table),
		Key:       cooldownKey(email),
	})
	return err
}
