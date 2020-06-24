package envoy

import (
	"fmt"

	envoy_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_grpc_credential "github.com/envoyproxy/go-control-plane/envoy/config/grpc_credential/v2alpha"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	"github.com/golang/protobuf/ptypes/wrappers"

	core_xds "github.com/Kong/kuma/pkg/core/xds"
	"github.com/Kong/kuma/pkg/sds/server"
	"github.com/Kong/kuma/pkg/util/proto"
	util_xds "github.com/Kong/kuma/pkg/util/xds"
	xds_context "github.com/Kong/kuma/pkg/xds/context"
)

// CreateDownstreamTlsContext creates DownstreamTlsContext for incoming connections
// It verifies that incoming connection has TLS certificate signed by Mesh CA with URI SAN of prefix spiffe://{mesh_name}/
// It secures inbound listener with certificate of "identity_cert" that will be received from the SDS (it contains URI SANs of all inbounds).
// Access to SDS is secured by TLS certificate (set in config or autogenerated at CP start) and path to dataplane token
func CreateDownstreamTlsContext(ctx xds_context.Context, metadata *core_xds.DataplaneMetadata) (*envoy_auth.DownstreamTlsContext, error) {
	if !ctx.Mesh.Resource.MTLSEnabled() {
		return nil, nil
	}
	validationSANMatcher := MeshSpiffeIDPrefixMatcher(ctx.Mesh.Resource.Meta.GetName())
	commonTlsContext, err := CreateCommonTlsContext(ctx, metadata, validationSANMatcher)
	if err != nil {
		return nil, err
	}
	return &envoy_auth.DownstreamTlsContext{
		CommonTlsContext:         commonTlsContext,
		RequireClientCertificate: &wrappers.BoolValue{Value: true},
	}, nil
}

// CreateUpstreamTlsContext creates UpstreamTlsContext for outgoing connections
// It verifies that the upstream server has TLS certificate signed by Mesh CA with URI SAN of spiffe://{mesh_name}/{upstream_service}
// The downstream client exposes for the upstream server cert with multiple URI SANs, which means that if DP has inbound with services "web" and "web-api" and communicates with "backend"
// the upstream server ("backend") will see that DP with TLS certificate of URIs of "web" and "web-api".
// There is no way to correlate incoming request to "web" or "web-api" with outgoing request to "backend" to expose only one URI SAN.
//
// Pass "*" for upstreamService to validate that upstream service is a service that is part of the mesh (but not specific one)
func CreateUpstreamTlsContext(ctx xds_context.Context, metadata *core_xds.DataplaneMetadata, upstreamService string, sni string) (*envoy_auth.UpstreamTlsContext, error) {
	if !ctx.Mesh.Resource.MTLSEnabled() {
		return nil, nil
	}
	var validationSANMatcher *envoy_type_matcher.StringMatcher
	if upstreamService == "*" {
		validationSANMatcher = MeshSpiffeIDPrefixMatcher(ctx.Mesh.Resource.Meta.GetName())
	} else {
		validationSANMatcher = ServiceSpiffeIDMatcher(ctx.Mesh.Resource.Meta.GetName(), upstreamService)
	}
	commonTlsContext, err := CreateCommonTlsContext(ctx, metadata, validationSANMatcher)
	if err != nil {
		return nil, err
	}
	return &envoy_auth.UpstreamTlsContext{
		CommonTlsContext: commonTlsContext,
		Sni:              sni,
	}, nil
}

func CreateCommonTlsContext(ctx xds_context.Context, metadata *core_xds.DataplaneMetadata, validationSANMatcher *envoy_type_matcher.StringMatcher) (*envoy_auth.CommonTlsContext, error) {
	meshCaSecret, err := sdsSecretConfig(ctx, server.MeshCaResource, metadata)
	if err != nil {
		return nil, err
	}
	identitySecret, err := sdsSecretConfig(ctx, server.IdentityCertResource, metadata)
	if err != nil {
		return nil, err
	}
	return &envoy_auth.CommonTlsContext{
		ValidationContextType: &envoy_auth.CommonTlsContext_CombinedValidationContext{
			CombinedValidationContext: &envoy_auth.CommonTlsContext_CombinedCertificateValidationContext{
				DefaultValidationContext: &envoy_auth.CertificateValidationContext{
					MatchSubjectAltNames: []*envoy_type_matcher.StringMatcher{validationSANMatcher},
				},
				ValidationContextSdsSecretConfig: meshCaSecret,
			},
		},
		TlsCertificateSdsSecretConfigs: []*envoy_auth.SdsSecretConfig{
			identitySecret,
		},
	}, nil
}

func sdsSecretConfig(context xds_context.Context, name string, metadata *core_xds.DataplaneMetadata) (*envoy_auth.SdsSecretConfig, error) {
	withCallCredentials := func(grpc *envoy_core.GrpcService_GoogleGrpc) (*envoy_core.GrpcService_GoogleGrpc, error) {
		if metadata.GetDataplaneTokenPath() == "" {
			return grpc, nil
		}

		config := &envoy_grpc_credential.FileBasedMetadataConfig{
			SecretData: &envoy_core.DataSource{
				Specifier: &envoy_core.DataSource_Filename{
					Filename: metadata.GetDataplaneTokenPath(),
				},
			},
		}
		typedConfig, err := proto.MarshalAnyDeterministic(config)
		if err != nil {
			return nil, err
		}

		grpc.CallCredentials = append(grpc.CallCredentials, &envoy_core.GrpcService_GoogleGrpc_CallCredentials{
			CredentialSpecifier: &envoy_core.GrpcService_GoogleGrpc_CallCredentials_FromPlugin{
				FromPlugin: &envoy_core.GrpcService_GoogleGrpc_CallCredentials_MetadataCredentialsFromPlugin{
					Name: "envoy.grpc_credentials.file_based_metadata",
					ConfigType: &envoy_core.GrpcService_GoogleGrpc_CallCredentials_MetadataCredentialsFromPlugin_TypedConfig{
						TypedConfig: typedConfig,
					},
				},
			},
		})
		grpc.CredentialsFactoryName = "envoy.grpc_credentials.file_based_metadata"

		return grpc, nil
	}
	googleGrpc, err := withCallCredentials(&envoy_core.GrpcService_GoogleGrpc{
		TargetUri:  context.ControlPlane.SdsLocation,
		StatPrefix: util_xds.SanitizeMetric("sds_" + name),
		ChannelCredentials: &envoy_core.GrpcService_GoogleGrpc_ChannelCredentials{
			CredentialSpecifier: &envoy_core.GrpcService_GoogleGrpc_ChannelCredentials_SslCredentials{
				SslCredentials: &envoy_core.GrpcService_GoogleGrpc_SslCredentials{
					RootCerts: &envoy_core.DataSource{
						Specifier: &envoy_core.DataSource_InlineBytes{
							InlineBytes: context.ControlPlane.SdsTlsCert,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &envoy_auth.SdsSecretConfig{
		Name: name,
		SdsConfig: &envoy_core.ConfigSource{
			ConfigSourceSpecifier: &envoy_core.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoy_core.ApiConfigSource{
					ApiType: envoy_core.ApiConfigSource_GRPC,
					GrpcServices: []*envoy_core.GrpcService{
						{
							TargetSpecifier: &envoy_core.GrpcService_GoogleGrpc_{
								GoogleGrpc: googleGrpc,
							},
						},
					},
				},
			},
		},
	}, nil
}

func MeshSpiffeIDPrefix(mesh string) string {
	return fmt.Sprintf("spiffe://%s/", mesh)
}

func MeshSpiffeIDPrefixMatcher(mesh string) *envoy_type_matcher.StringMatcher {
	return &envoy_type_matcher.StringMatcher{
		MatchPattern: &envoy_type_matcher.StringMatcher_Prefix{
			Prefix: MeshSpiffeIDPrefix(mesh),
		},
	}
}

func ServiceSpiffeID(mesh string, service string) string {
	return fmt.Sprintf("spiffe://%s/%s", mesh, service)
}

func ServiceSpiffeIDMatcher(mesh string, service string) *envoy_type_matcher.StringMatcher {
	return &envoy_type_matcher.StringMatcher{
		MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
			Exact: ServiceSpiffeID(mesh, service),
		},
	}
}
