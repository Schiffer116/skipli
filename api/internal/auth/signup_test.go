package auth

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/Schiffer116/skipli/api/internal/awsfake"
)

// pool fakes a user pool holding the email users in existing, keyed by
// address with their usernames as values.
func pool(t *testing.T, existing map[string]string) *awsfake.Server {
	return awsfake.New(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "AdminCreateUser":
			if _, ok := existing[body["Username"].(string)]; ok {
				return 400, map[string]any{"__type": "UsernameExistsException", "message": "exists"}
			}
			return 200, map[string]any{"User": map[string]any{"Username": "new-id", "Attributes": []any{
				map[string]any{"Name": "sub", "Value": "new-id"},
			}}}
		case "AdminGetUser":
			username := existing[body["Username"].(string)]
			return 200, map[string]any{"Username": username, "UserAttributes": []any{
				map[string]any{"Name": "sub", "Value": username},
			}}
		}
		return 200, map[string]any{}
	})
}

func googleSignUp(email, verified string) events.CognitoEventUserPoolsPreSignup {
	var event events.CognitoEventUserPoolsPreSignup
	event.TriggerSource = "PreSignUp_ExternalProvider"
	event.UserPoolID = "pool"
	event.UserName = "google_1234"
	event.Request.UserAttributes = map[string]string{"email": email, "email_verified": verified}
	return event
}

func linked(t *testing.T, fake *awsfake.Server) (destination, subject string) {
	t.Helper()
	links := fake.Calls("AdminLinkProviderForUser")
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	dest := links[0].Body["DestinationUser"].(map[string]any)
	src := links[0].Body["SourceUser"].(map[string]any)
	if dest["ProviderName"] != "Cognito" || src["ProviderName"] != "Google" || src["ProviderAttributeName"] != "Cognito_Subject" {
		t.Errorf("link %v, want Google subject onto a Cognito user", links[0].Body)
	}
	return dest["ProviderAttributeValue"].(string), src["ProviderAttributeValue"].(string)
}

func TestGoogleSignUpLinksTheExistingEmailUser(t *testing.T) {
	fake := pool(t, map[string]string{"alice@example.com": "alice-id"})

	if _, err := PreSignUp(context.Background(), fake.Cognito(), googleSignUp("Alice@Example.com", "true")); err != nil {
		t.Fatal(err)
	}

	if dest, subject := linked(t, fake); dest != "alice-id" || subject != "1234" {
		t.Errorf("linked %s to %s, want 1234 to alice-id", subject, dest)
	}
}

// Google first, email code later: the email user made now is the one the
// email code signs in to, so both reach the same account.
func TestGoogleSignUpWithoutAnAccountCreatesTheEmailUserToLinkTo(t *testing.T) {
	fake := pool(t, map[string]string{})

	if _, err := PreSignUp(context.Background(), fake.Cognito(), googleSignUp("bob@example.com", "true")); err != nil {
		t.Fatal(err)
	}

	creates := fake.Calls("AdminCreateUser")
	if len(creates) != 1 || creates[0].Body["Username"] != "bob@example.com" {
		t.Fatalf("created %v, want bob@example.com", creates)
	}
	if dest, _ := linked(t, fake); dest != "new-id" {
		t.Errorf("linked to %s, want the new user new-id", dest)
	}
}

func TestGoogleSignUpWithAnUnverifiedEmailIsRefused(t *testing.T) {
	fake := pool(t, map[string]string{"alice@example.com": "alice-id"})

	if _, err := PreSignUp(context.Background(), fake.Cognito(), googleSignUp("alice@example.com", "false")); err == nil {
		t.Fatal("got no error, want the sign-up refused")
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("made %d calls, want none", len(calls))
	}
}

func TestEmailSignUpPassesThrough(t *testing.T) {
	fake := pool(t, map[string]string{})
	var event events.CognitoEventUserPoolsPreSignup
	event.TriggerSource = "PreSignUp_AdminCreateUser"

	if _, err := PreSignUp(context.Background(), fake.Cognito(), event); err != nil {
		t.Fatal(err)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("made %d calls, want none", len(calls))
	}
}
