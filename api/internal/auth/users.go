package auth

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

type User struct {
	ID    string
	Email string
}

type Directory struct {
	idp    *cip.Client
	poolID string
}

func NewDirectory(idp *cip.Client, poolID string) *Directory {
	return &Directory{idp: idp, poolID: poolID}
}

func (d *Directory) Ensure(ctx context.Context, email string) (User, error) {
	created, err := d.idp.AdminCreateUser(ctx, &cip.AdminCreateUserInput{
		UserPoolId:    aws.String(d.poolID),
		Username:      aws.String(email),
		MessageAction: types.MessageActionTypeSuppress,
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(email)},
			{Name: aws.String("email_verified"), Value: aws.String("true")},
		},
	})
	var exists *types.UsernameExistsException
	if errors.As(err, &exists) {
		got, err := d.idp.AdminGetUser(ctx, &cip.AdminGetUserInput{
			UserPoolId: aws.String(d.poolID),
			Username:   aws.String(email),
		})
		if err != nil {
			return User{}, err
		}
		return userFrom(got.UserAttributes), nil
	}
	if err != nil {
		return User{}, err
	}

	_, err = d.idp.AdminSetUserPassword(ctx, &cip.AdminSetUserPasswordInput{
		UserPoolId: aws.String(d.poolID),
		Username:   aws.String(email),
		Password:   aws.String(rand.Text() + "Aa1!"),
		Permanent:  true,
	})
	if err != nil {
		return User{}, err
	}
	return userFrom(created.User.Attributes), nil
}

func userFrom(attributes []types.AttributeType) User {
	var u User
	for _, a := range attributes {
		switch aws.ToString(a.Name) {
		case "sub":
			u.ID = aws.ToString(a.Value)
		case "email":
			u.Email = aws.ToString(a.Value)
		}
	}
	return u
}
