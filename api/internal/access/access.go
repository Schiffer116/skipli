// Package access decides whether someone may open a board, for both the
// HTTP API and the realtime API.
package access

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Check answers 200 for a member of the board, 403 for anyone else, and 404
// when the board doesn't exist.
func Check(ctx context.Context, db *dynamodb.Client, table, boardID, userID string) (int, error) {
	member, err := Exists(ctx, db, table, boardID, "MEMBER#"+userID)
	if err != nil {
		return 0, err
	}
	if member {
		return http.StatusOK, nil
	}
	return Missing(ctx, db, table, boardID)
}

// Missing is the answer for someone who isn't a member: 403, or 404 when the
// board doesn't exist. Members are deleted along with their board, so this
// read only happens for callers who are about to be turned away.
func Missing(ctx context.Context, db *dynamodb.Client, table, boardID string) (int, error) {
	exists, err := Exists(ctx, db, table, boardID, "META")
	if err != nil {
		return 0, err
	}
	if !exists {
		return http.StatusNotFound, nil
	}
	return http.StatusForbidden, nil
}

func Exists(ctx context.Context, db *dynamodb.Client, table, pk, sk string) (bool, error) {
	out, err := db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:            aws.String(table),
		Key:                  Key(pk, sk),
		ProjectionExpression: aws.String("PK"),
	})
	if err != nil {
		return false, err
	}
	return out.Item != nil, nil
}

func Key(pk, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: sk},
	}
}
