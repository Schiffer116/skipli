package jev

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// FromEnv is the judge the API runs with:
//   - TYPESAFE_API_KEY set: Jev with that key, to try the real thing locally;
//   - JEV_KEY_PARAMETER set: Jev with the key in that SSM SecureString, as
//     deployed;
//   - neither, on Lambda: none, so sorting answers 503 rather than faking;
//   - neither, locally: Fake.
//
// JEV_BASE_URL and JEV_MODEL point it elsewhere, such as at OpenRouter.
func FromEnv(cfg aws.Config) Judge {
	client := func(key func(context.Context) (string, error)) Judge {
		return &Client{
			BaseURL: or(os.Getenv("JEV_BASE_URL"), DefaultBaseURL),
			Model:   or(os.Getenv("JEV_MODEL"), DefaultModel),
			Key:     key,
			HTTP:    http.DefaultClient,
		}
	}
	if key := os.Getenv("TYPESAFE_API_KEY"); key != "" {
		return client(func(context.Context) (string, error) { return key, nil })
	}
	if name := os.Getenv("JEV_KEY_PARAMETER"); name != "" {
		return client(SSMKey(ssm.NewFromConfig(cfg), name))
	}
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		return unconfigured{}
	}
	log.Print("no Jev key: sorting with the fake judge")
	return Fake{}
}

type unconfigured struct{}

func (unconfigured) Judge(context.Context, string, []Task) (Verdict, error) {
	return Verdict{}, ErrNotConfigured
}

// SSMKey reads the key from an SSM SecureString the first time it's needed,
// and keeps it once found; until the parameter is set, each sort looks
// again.
func SSMKey(client *ssm.Client, name string) func(context.Context) (string, error) {
	var mu sync.Mutex
	var key string
	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if key != "" {
			return key, nil
		}
		out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
			Name:           aws.String(name),
			WithDecryption: aws.Bool(true),
		})
		var missing *types.ParameterNotFound
		if errors.As(err, &missing) {
			return "", ErrNotConfigured
		}
		if err != nil {
			return "", fmt.Errorf("reading the Jev key: %w", err)
		}
		key = aws.ToString(out.Parameter.Value)
		return key, nil
	}
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
