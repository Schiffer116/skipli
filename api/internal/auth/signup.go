package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// googleProvider is the identity provider's name in the user pool. Cognito
// names its federated users "<provider>_<subject>", in any letter case.
const googleProvider = "Google"

// PreSignUp is the user pool's pre sign-up trigger. Cognito would give the
// first Google sign-in for an address a user of its own, with its own id and
// no boards, so instead this links that Google account to the address's
// email user, creating the email user first if there is none yet. Either way
// the person keeps one id however they sign in, and a later email code for
// the address reaches the same account. Other sign-ups pass through.
func PreSignUp(ctx context.Context, idp *cip.Client, event events.CognitoEventUserPoolsPreSignup) (events.CognitoEventUserPoolsPreSignup, error) {
	if event.TriggerSource != "PreSignUp_ExternalProvider" {
		return event, nil
	}

	provider, subject, _ := strings.Cut(event.UserName, "_")
	if !strings.EqualFold(provider, googleProvider) || subject == "" {
		return event, fmt.Errorf("unsupported identity provider %q", provider)
	}
	// An unverified address could belong to someone else, and linking would
	// hand them that person's boards.
	if event.Request.UserAttributes["email_verified"] != "true" {
		return event, errors.New("Google hasn't verified this email address")
	}
	email := NormalizeEmail(event.Request.UserAttributes["email"])
	if email == "" {
		return event, errors.New("Google didn't share an email address")
	}

	users := NewDirectory(idp, event.UserPoolID)
	if err := users.linkProvider(ctx, email, googleProvider, subject); err != nil {
		return event, fmt.Errorf("link %s account: %w", googleProvider, err)
	}
	return event, nil
}

func (d *Directory) linkProvider(ctx context.Context, email, provider, subject string) error {
	_, username, err := d.ensure(ctx, email)
	if err != nil {
		return err
	}
	_, err = d.idp.AdminLinkProviderForUser(ctx, &cip.AdminLinkProviderForUserInput{
		UserPoolId: aws.String(d.poolID),
		DestinationUser: &types.ProviderUserIdentifierType{
			ProviderName:           aws.String("Cognito"),
			ProviderAttributeValue: aws.String(username),
		},
		SourceUser: &types.ProviderUserIdentifierType{
			ProviderName:           aws.String(provider),
			ProviderAttributeName:  aws.String("Cognito_Subject"),
			ProviderAttributeValue: aws.String(subject),
		},
	})
	return err
}
