package fleetdb

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/client-go/rest"
)

const (
	tokenPrefix     = "k8s-aws-v1."
	tokenExpiry     = 14 * time.Minute
	clusterIDHeader = "x-k8s-aws-id"
)

// newRESTConfig returns a rest.Config that authenticates to the given EKS
// cluster using IAM credentials from the ambient environment (Pod Identity,
// IRSA, instance profile, etc.).
func newRESTConfig(ctx context.Context, awsCfg aws.Config, clusterName string) (*rest.Config, error) {
	eksClient := eks.NewFromConfig(awsCfg)
	out, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: &clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("describe cluster %s: %w", clusterName, err)
	}

	ca, err := base64.StdEncoding.DecodeString(*out.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("decode CA: %w", err)
	}

	provider := &tokenProvider{
		sts:         sts.NewFromConfig(awsCfg),
		clusterName: clusterName,
	}

	return &rest.Config{
		Host: *out.Cluster.Endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &tokenRoundTripper{delegate: rt, provider: provider}
		},
	}, nil
}

type tokenProvider struct {
	sts         *sts.Client
	clusterName string

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (p *tokenProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token != "" && time.Now().Before(p.expiry.Add(-1*time.Minute)) {
		return p.token, nil
	}

	presignClient := sts.NewPresignClient(p.sts)
	presigned, err := presignClient.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(o *sts.PresignOptions) {
		o.ClientOptions = []func(*sts.Options){
			sts.WithAPIOptions(
				smithyhttp.AddHeaderValue(clusterIDHeader, p.clusterName),
				smithyhttp.AddHeaderValue("X-Amz-Expires", "60"),
			),
		}
	})
	if err != nil {
		return "", fmt.Errorf("presign GetCallerIdentity: %w", err)
	}

	p.token = tokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(presigned.URL))
	p.expiry = time.Now().Add(tokenExpiry)
	return p.token, nil
}

type tokenRoundTripper struct {
	delegate http.RoundTripper
	provider *tokenProvider
}

func (t *tokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.provider.Token(req.Context())
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	return t.delegate.RoundTrip(clone)
}
