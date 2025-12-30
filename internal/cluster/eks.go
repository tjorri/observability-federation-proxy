package cluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/tjorri/observability-federation-proxy/internal/config"
)

// eksRESTConfig wraps rest.Config to implement RESTConfig interface.
type eksRESTConfig struct {
	*rest.Config
}

func (c *eksRESTConfig) Host() string {
	return c.Config.Host
}

func (r *Registry) createEKSCluster(ctx context.Context, cfg config.ClusterConfig) (*Cluster, error) {
	if cfg.EKS == nil {
		return nil, fmt.Errorf("eks config is required for eks cluster type")
	}

	// Load AWS config
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.EKS.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// If role assumption is configured, wrap credentials
	if cfg.EKS.AssumeRole != nil {
		stsClient := sts.NewFromConfig(awsCfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, cfg.EKS.AssumeRole.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			if cfg.EKS.AssumeRole.ExternalID != "" {
				o.ExternalID = aws.String(cfg.EKS.AssumeRole.ExternalID)
			}
			if cfg.EKS.AssumeRole.SessionName != "" {
				o.RoleSessionName = cfg.EKS.AssumeRole.SessionName
			} else {
				o.RoleSessionName = "observability-federation-proxy"
			}
		})
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}

	// Get EKS cluster info
	eksClient := eks.NewFromConfig(awsCfg)
	clusterInfo, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(cfg.EKS.ClusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe EKS cluster: %w", err)
	}

	// Decode CA certificate
	caData, err := base64.StdEncoding.DecodeString(*clusterInfo.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA certificate: %w", err)
	}

	// Create STS client for token generation
	stsClient := sts.NewFromConfig(awsCfg)

	// Create REST config with token-based auth
	restCfg := &rest.Config{
		Host: *clusterInfo.Cluster.Endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &eksTokenTransport{
				base:        rt,
				clusterName: cfg.EKS.ClusterName,
				stsClient:   stsClient,
			}
		},
	}

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Cluster{
		Name:       cfg.Name,
		Config:     cfg,
		Client:     client,
		restConfig: &eksRESTConfig{restCfg},
	}, nil
}

// eksTokenTransport adds EKS token authentication to HTTP requests.
type eksTokenTransport struct {
	base        http.RoundTripper
	clusterName string
	stsClient   *sts.Client
	token       string
	expiry      time.Time
	mu          sync.Mutex
}

func (t *eksTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.getToken()
	if err != nil {
		return nil, err
	}

	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(req)
}

func (t *eksTokenTransport) getToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Return cached token if still valid (with 1 minute buffer)
	if t.token != "" && time.Now().Add(time.Minute).Before(t.expiry) {
		return t.token, nil
	}

	// Generate new token
	token, err := t.generateToken()
	if err != nil {
		return "", err
	}

	t.token = token
	// EKS tokens are valid for 15 minutes
	t.expiry = time.Now().Add(14 * time.Minute)
	return t.token, nil
}

func (t *eksTokenTransport) generateToken() (string, error) {
	presignClient := sts.NewPresignClient(t.stsClient)

	// Create presigned GetCallerIdentity request with cluster name header
	presignedReq, err := presignClient.PresignGetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{}, func(opt *sts.PresignOptions) {
		opt.ClientOptions = append(opt.ClientOptions, func(o *sts.Options) {
			o.APIOptions = append(o.APIOptions, smithyhttp.AddHeaderValue("x-k8s-aws-id", t.clusterName))
		})
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign request: %w", err)
	}

	// EKS expects the token in format: k8s-aws-v1.<base64-encoded-url>
	token := "k8s-aws-v1." + base64.RawURLEncoding.EncodeToString([]byte(presignedReq.URL))
	return token, nil
}
