package e2e_test

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AWS Credentials Check", func() {
	var (
		awsProfile string
		awsRegion  string
	)

	BeforeEach(func() {
		awsProfile = os.Getenv("AWS_PROFILE")
		awsRegion = os.Getenv("AWS_REGION")
		if awsRegion == "" {
			awsRegion = "us-east-2" // default region
		}
	})

	It("should have AWS credentials configured", func() {

		By("loading AWS configuration from environment or credentials file")
		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(awsRegion),
		)
		Expect(err).To(BeNil(), "Failed to load AWS configuration")
		Expect(cfg).ToNot(BeNil())

		By("verifying AWS credentials work with STS GetCallerIdentity")
		stsClient := sts.NewFromConfig(cfg)
		identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

		Expect(err).To(BeNil(), "Failed to get caller identity - check AWS credentials")
		Expect(identity).ToNot(BeNil())
		Expect(identity.Account).ToNot(BeNil(), "AWS Account ID should not be nil")
		Expect(identity.Arn).ToNot(BeNil(), "AWS ARN should not be nil")

		By("displaying AWS identity information")
		GinkgoWriter.Printf("✓ AWS Credentials verified successfully\n")
		GinkgoWriter.Printf("  Account: %s\n", *identity.Account)
		GinkgoWriter.Printf("  ARN: %s\n", *identity.Arn)
		GinkgoWriter.Printf("  UserId: %s\n", *identity.UserId)
		if awsProfile != "" {
			GinkgoWriter.Printf("  Profile: %s\n", awsProfile)
		}
		GinkgoWriter.Printf("  Region: %s\n", awsRegion)
	})

	It("should report AWS environment variables if set", func() {
		envVars := map[string]string{
			"AWS_PROFILE":           os.Getenv("AWS_PROFILE"),
			"AWS_REGION":            os.Getenv("AWS_REGION"),
			"AWS_ACCESS_KEY_ID":     getMasked(os.Getenv("AWS_ACCESS_KEY_ID")),
			"AWS_SECRET_ACCESS_KEY": getMasked(os.Getenv("AWS_SECRET_ACCESS_KEY")),
			"AWS_SESSION_TOKEN":     getMasked(os.Getenv("AWS_SESSION_TOKEN")),
			"AWS_SDK_LOAD_CONFIG":   os.Getenv("AWS_SDK_LOAD_CONFIG"),
		}

		GinkgoWriter.Println("\nAWS Environment Variables:")
		for key, value := range envVars {
			if value != "" {
				GinkgoWriter.Printf("  %s: %s\n", key, value)
			}
		}

		// At least one auth method should be present
		hasCredentials := os.Getenv("AWS_ACCESS_KEY_ID") != "" ||
			os.Getenv("AWS_PROFILE") != "" ||
			fileExists("/root/.aws/credentials") ||
			fileExists(os.Getenv("HOME")+"/.aws/credentials")

		Expect(hasCredentials).To(BeTrue(), "No AWS credentials found in environment or credentials file")
	})
})

// getMasked returns a masked version of sensitive values
func getMasked(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	// Show first 4 and last 4 characters
	return value[:4] + "***" + value[len(value)-4:]
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
