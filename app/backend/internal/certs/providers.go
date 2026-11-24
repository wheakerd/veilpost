package certs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	alidnsapi "github.com/go-acme/alidns-20150109/v4/client"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	legoroute53 "github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/providers/dns/tencentcloud"
	dnspod "github.com/go-acme/tencentclouddnspod/v20210323"
	tencentcommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

const (
	cloudflareAPI = "https://api.cloudflare.com/client/v4"
	// Propagation is checked against the authoritative name servers.
	propagationTimeout = 5 * time.Minute
	pollingInterval    = 5 * time.Second
	providerTimeout    = 30 * time.Second
)

// dnsProvider builds a lego DNS-01 provider from decrypted values. Each
// configuration is built explicitly, so ambient environment variables and
// cloud instance credentials of the console host are never used.
func (s *Service) dnsProvider(provider string, secrets map[string]string) (challenge.Provider, error) {
	switch provider {
	case "cloudflare":
		return cloudflare.NewDNSProviderConfig(&cloudflare.Config{
			AuthToken: secrets["apiToken"], BaseURL: s.cloudflareURL, TTL: 120,
			PropagationTimeout: propagationTimeout, PollingInterval: pollingInterval,
			HTTPClient: &http.Client{Timeout: providerTimeout},
		})
	case "alidns":
		return alidns.NewDNSProviderConfig(&alidns.Config{
			APIKey: secrets["accessKeyId"], SecretKey: secrets["accessKeySecret"], TTL: 600,
			PropagationTimeout: propagationTimeout, PollingInterval: pollingInterval, HTTPTimeout: providerTimeout,
		})
	case "tencentcloud":
		return tencentcloud.NewDNSProviderConfig(&tencentcloud.Config{
			SecretID: secrets["secretId"], SecretKey: secrets["secretKey"], TTL: 600,
			PropagationTimeout: propagationTimeout, PollingInterval: pollingInterval, HTTPTimeout: providerTimeout,
		})
	case "route53":
		return legoroute53.NewDNSProviderConfig(&legoroute53.Config{
			Client:       route53Client(secrets),
			HostedZoneID: strings.TrimPrefix(secrets["hostedZoneId"], "/hostedzone/"),
			MaxRetries:   5, TTL: 60, WaitForRecordSetsChanged: true,
			PropagationTimeout: propagationTimeout, PollingInterval: pollingInterval,
		})
	}
	return nil, fmt.Errorf("unsupported DNS provider")
}

func route53Client(secrets map[string]string) *route53.Client {
	return route53.New(route53.Options{
		Region:           secrets["region"],
		Credentials:      aws.NewCredentialsCache(awscredentials.NewStaticCredentialsProvider(secrets["accessKeyId"], secrets["secretAccessKey"], "")),
		HTTPClient:       &http.Client{Timeout: providerTimeout},
		RetryMaxAttempts: 2,
	})
}

func alidnsClient(secrets map[string]string) (*alidnsapi.Client, error) {
	config := new(openapi.Config).SetRegionId("cn-hangzhou").
		SetAccessKeyId(secrets["accessKeyId"]).SetAccessKeySecret(secrets["accessKeySecret"]).
		SetReadTimeout(int(verifyTimeout.Milliseconds())).SetConnectTimeout(int(verifyTimeout.Milliseconds()))
	return alidnsapi.NewClient(config)
}

// verify calls one read-only API of the provider: it lists at most one zone
// (or reads the configured Route 53 hosted zone) and changes nothing.
func (s *Service) verify(ctx context.Context, provider string, secrets map[string]string) error {
	switch provider {
	case "cloudflare":
		return verifyCloudflare(ctx, s.cloudflareURL, secrets["apiToken"])
	case "alidns":
		client, err := alidnsClient(secrets)
		if err != nil {
			return err
		}
		request := new(alidnsapi.DescribeDomainsRequest).SetPageNumber(1).SetPageSize(1)
		_, err = alidnsapi.DescribeDomainsWithContext(ctx, client, request, &dara.RuntimeOptions{})
		return err
	case "tencentcloud":
		cpf := profile.NewClientProfile()
		cpf.HttpProfile.Endpoint = "dnspod.tencentcloudapi.com"
		cpf.HttpProfile.ReqTimeout = int(verifyTimeout.Seconds())
		client, err := dnspod.NewClient(tencentcommon.NewCredential(secrets["secretId"], secrets["secretKey"]), "", cpf)
		if err != nil {
			return err
		}
		request := dnspod.NewDescribeDomainListRequest()
		request.Limit = tencentcommon.Int64Ptr(1)
		_, err = dnspod.DescribeDomainListWithContext(ctx, client, request)
		return err
	case "route53":
		client := route53Client(secrets)
		if zone := strings.TrimPrefix(secrets["hostedZoneId"], "/hostedzone/"); zone != "" {
			_, err := client.GetHostedZone(ctx, &route53.GetHostedZoneInput{Id: aws.String(zone)})
			return err
		}
		_, err := client.ListHostedZones(ctx, &route53.ListHostedZonesInput{MaxItems: aws.Int32(1)})
		return err
	}
	return fmt.Errorf("unsupported DNS provider")
}

// verifyCloudflare lists at most one zone, which needs the Zone:Read
// permission that DNS-01 issuance also uses.
func verifyCloudflare(ctx context.Context, base, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/zones?per_page=1", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := (&http.Client{Timeout: verifyTimeout}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var body struct {
		Success bool `json:"success"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body)
	if response.StatusCode >= 500 {
		return &net.OpError{Op: "read", Err: errors.New("provider unavailable")}
	}
	if response.StatusCode != http.StatusOK || decodeErr != nil || !body.Success {
		return errors.New("credentials rejected")
	}
	return nil
}

// unreachable reports network failures and timeouts, as opposed to answers
// in which the provider rejected the credentials.
func unreachable(err error) bool {
	var netErr net.Error
	var urlErr *url.Error
	var opErr *net.OpError
	var dnsErr *net.DNSError
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.As(err, &netErr) || errors.As(err, &urlErr) || errors.As(err, &opErr) || errors.As(err, &dnsErr)
}
