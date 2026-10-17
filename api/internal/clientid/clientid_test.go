package clientid

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestPickKeepsAClientUUID(t *testing.T) {
	const id = "0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69"
	got, err := Pick(id)
	if err != nil || got != id {
		t.Fatalf("Pick(%q) = %q, %v", id, got, err)
	}
}

func TestPickMakesAnIDWhenNoneIsSent(t *testing.T) {
	a, errA := Pick("")
	b, errB := Pick("")
	if errA != nil || errB != nil || a == "" || a == b {
		t.Fatalf("Pick(\"\") = %q, %q (%v, %v)", a, b, errA, errB)
	}
}

func TestPickRejectsAnythingButALowercaseUUID(t *testing.T) {
	for _, id := range []string{
		"META",
		"USER#u1",
		"0B6F8F5E-2C3A-4F7E-9A51-1F2D3C4B5A69",
		"{0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69}",
		"urn:uuid:0b6f8f5e-2c3a-4f7e-9a51-1f2d3c4b5a69",
	} {
		if _, err := Pick(id); !errors.Is(err, ErrInvalid) {
			t.Errorf("Pick(%q) err = %v, want ErrInvalid", id, err)
		}
	}
}

func TestTaken(t *testing.T) {
	conditional := &types.ConditionalCheckFailedException{}
	inTransaction := &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{
		{Code: aws.String("None")},
		{Code: aws.String("ConditionalCheckFailed")},
	}}
	otherTransaction := &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{
		{Code: aws.String("ThrottlingError")},
	}}

	for err, want := range map[error]bool{
		conditional:                        true,
		fmt.Errorf("put: %w", conditional): true,
		inTransaction:                      true,
		otherTransaction:                   false,
		errors.New("network down"):         false,
	} {
		if got := Taken(err); got != want {
			t.Errorf("Taken(%v) = %v, want %v", err, got, want)
		}
	}
}
