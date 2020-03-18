package helpers

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
)

func GetAwsSession(endpoint string) *session.Session {
	config := aws.Config{
		Region: aws.String("eu-west-1"),
	}
	if endpoint != "" {
		defaultResolver := endpoints.DefaultResolver()
		s3CustResolverFn := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
			if endpoint != "" {
				return endpoints.ResolvedEndpoint{
					URL: endpoint,
				}, nil
			}
			return defaultResolver.EndpointFor(service, region, optFns...)
		}
		config.EndpointResolver = endpoints.ResolverFunc(s3CustResolverFn)
	}
	return session.Must(session.NewSessionWithOptions(session.Options{Config: config}))
}
