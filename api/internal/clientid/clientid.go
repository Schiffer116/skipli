// Package clientid lets clients choose the IDs of the boards, cards and
// tasks they create, so the app can refer to something it made offline
// before the server has seen it, and a retried create can't make a
// duplicate (see docs/adr/0003-offline-editing-for-solo-boards-only.md).
package clientid

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var ErrInvalid = errors.New("id must be a lowercase UUID")

// Pick returns the ID the client sent for a new item, or a fresh one when
// it sent none. Only canonical UUIDs are accepted: IDs become DynamoDB
// keys, so anything else could collide with other kinds of item.
func Pick(id string) (string, error) {
	if id == "" {
		return uuid.New().String(), nil
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return "", ErrInvalid
	}
	return id, nil
}

// Taken reports whether a write guarded by attribute_not_exists failed
// because an item with that ID already exists, whether written alone or
// in a transaction.
func Taken(err error) bool {
	var conditional *types.ConditionalCheckFailedException
	if errors.As(err, &conditional) {
		return true
	}
	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) {
		for _, reason := range canceled.CancellationReasons {
			if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
				return true
			}
		}
	}
	return false
}
