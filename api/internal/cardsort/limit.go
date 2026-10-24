package cardsort

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	// gap is the least time between one person's sorts.
	gap = 3 * time.Second
	// perHour is how many sorts one person may ask for in a clock hour.
	perHour = 30
)

// Limit counts each person's sorts, in a DynamoDB item per clock hour that
// the table's TTL removes, so one account can't run up the Jev bill.
type Limit struct {
	db    *dynamodb.Client
	table string
}

func NewLimit(db *dynamodb.Client, table string) *Limit {
	return &Limit{db: db, table: table}
}

// Take counts a sort for userID. When the person is over the limit it
// returns false and how long until they aren't.
func (l *Limit) Take(ctx context.Context, userID string, now time.Time) (bool, time.Duration, error) {
	hour := now.Truncate(time.Hour)
	unix := func(t time.Time) types.AttributeValue {
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}
	}
	_, err := l.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(l.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "SORTS#" + strconv.FormatInt(hour.Unix(), 10)},
		},
		UpdateExpression: aws.String("SET LastAt = :now, ExpiresAt = :expires ADD Sorts :one"),
		ConditionExpression: aws.String("(attribute_not_exists(Sorts) OR Sorts < :max) AND " +
			"(attribute_not_exists(LastAt) OR LastAt <= :gapAgo)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now":     unix(now),
			":expires": unix(hour.Add(2 * time.Hour)),
			":one":     &types.AttributeValueMemberN{Value: "1"},
			":max":     &types.AttributeValueMemberN{Value: strconv.Itoa(perHour)},
			":gapAgo":  unix(now.Add(-gap)),
		},
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	var limited *types.ConditionalCheckFailedException
	if errors.As(err, &limited) {
		wait := hour.Add(time.Hour).Sub(now)
		if sorts, ok := number(limited.Item["Sorts"]); !ok || sorts < perHour {
			last, _ := number(limited.Item["LastAt"])
			wait = time.Unix(last, 0).Add(gap).Sub(now)
		}
		return false, max(wait, time.Second), nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, 0, nil
}

func number(v types.AttributeValue) (int64, bool) {
	n, ok := v.(*types.AttributeValueMemberN)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(n.Value, 10, 64)
	return i, err == nil
}
