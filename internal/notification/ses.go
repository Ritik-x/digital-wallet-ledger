package notification

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type SESClient struct {
	client *sesv2.Client
	from   string
}

func NewSESClient(ctx context.Context) (*SESClient, error) {
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	from := strings.TrimSpace(os.Getenv("SES_FROM_EMAIL"))

	if region == "" {
		return nil, fmt.Errorf("AWS_REGION is not configured")
	}

	if from == "" {
		return nil, fmt.Errorf("SES_FROM_EMAIL is not configured")
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	return &SESClient{
		client: sesv2.NewFromConfig(cfg),
		from:   from,
	}, nil
}

func (s *SESClient) SendTransferEmail(
	ctx context.Context,
	to string,
	transferID int64,
	amountMinor int64,
	currency string,
) error {
	to = strings.TrimSpace(to)
	currency = strings.TrimSpace(currency)

	if to == "" {
		return fmt.Errorf("recipient email is required")
	}
	if transferID <= 0 {
		return fmt.Errorf("invalid transfer ID")
	}
	if amountMinor <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if len(currency) != 3 {
		return fmt.Errorf("currency must be a three-letter code")
	}

	amount := fmt.Sprintf("%s %.2f", currency, float64(amountMinor)/100)

	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.from),
		Destination: &types.Destination{
			ToAddresses: []string{to},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data: aws.String("LedgerX transfer notification"),
				},
				Body: &types.Body{
					Text: &types.Content{
						Data: aws.String(fmt.Sprintf(
							"Your LedgerX transfer #%d has been processed.\nAmount: %s\n",
							transferID,
							amount,
						)),
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("send transfer email: %w", err)
	}

	return nil
}
