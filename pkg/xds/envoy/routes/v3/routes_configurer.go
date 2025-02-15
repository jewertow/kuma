package v3

import (
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_filters_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/any"

	"github.com/kumahq/kuma/pkg/util/proto"

	envoy_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	envoy_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"

	mesh_proto "github.com/kumahq/kuma/api/mesh/v1alpha1"
	envoy_common "github.com/kumahq/kuma/pkg/xds/envoy"
)

type RoutesConfigurer struct {
	Routes envoy_common.Routes
}

func (c RoutesConfigurer) Configure(virtualHost *envoy_route.VirtualHost) error {
	for _, route := range c.Routes {
		envoyRoute := &envoy_route.Route{
			Match: c.routeMatch(route.Match),
			Action: &envoy_route.Route_Route{
				Route: c.routeAction(route.Clusters, route.Modify),
			},
		}

		typedPerFilterConfig, err := c.typedPerFilterConfig(&route)
		if err != nil {
			return err
		}
		envoyRoute.TypedPerFilterConfig = typedPerFilterConfig

		c.setHeadersModifications(envoyRoute, route.Modify)

		virtualHost.Routes = append(virtualHost.Routes, envoyRoute)
	}
	return nil
}

func (c RoutesConfigurer) setHeadersModifications(route *envoy_route.Route, modify *mesh_proto.TrafficRoute_Http_Modify) {
	for _, add := range modify.GetRequestHeaders().GetAdd() {
		route.RequestHeadersToAdd = append(route.RequestHeadersToAdd, &envoy_core.HeaderValueOption{
			Header: &envoy_core.HeaderValue{
				Key:   add.Name,
				Value: add.Value,
			},
			Append: &wrappers.BoolValue{
				Value: add.Append,
			},
		})
	}
	for _, remove := range modify.GetRequestHeaders().GetRemove() {
		route.RequestHeadersToRemove = append(route.RequestHeadersToRemove, remove.Name)
	}

	for _, add := range modify.GetResponseHeaders().GetAdd() {
		route.ResponseHeadersToAdd = append(route.ResponseHeadersToAdd, &envoy_core.HeaderValueOption{
			Header: &envoy_core.HeaderValue{
				Key:   add.Name,
				Value: add.Value,
			},
			Append: &wrappers.BoolValue{
				Value: add.Append,
			},
		})
	}
	for _, remove := range modify.GetResponseHeaders().GetRemove() {
		route.ResponseHeadersToRemove = append(route.ResponseHeadersToRemove, remove.Name)
	}
}

func (c RoutesConfigurer) routeMatch(match *mesh_proto.TrafficRoute_Http_Match) *envoy_route.RouteMatch {
	envoyMatch := &envoy_route.RouteMatch{}

	if match.GetPath() != nil {
		c.setPathMatcher(match.GetPath(), envoyMatch)
	} else {
		// Path match is required on Envoy config so if there is only matching by header in TrafficRoute, we need to place
		// the default route match anyways.
		envoyMatch.PathSpecifier = &envoy_route.RouteMatch_Prefix{
			Prefix: "/",
		}
	}

	var headers []string
	for headerName := range match.GetHeaders() {
		headers = append(headers, headerName)
	}
	sort.Strings(headers) // sort for stability of Envoy config
	for _, headerName := range headers {
		envoyMatch.Headers = append(envoyMatch.Headers, c.headerMatcher(headerName, match.Headers[headerName]))
	}
	if match.GetMethod() != nil {
		envoyMatch.Headers = append(envoyMatch.Headers, c.headerMatcher(":method", match.GetMethod()))
	}

	return envoyMatch
}

func (c RoutesConfigurer) headerMatcher(name string, matcher *mesh_proto.TrafficRoute_Http_Match_StringMatcher) *envoy_route.HeaderMatcher {
	headerMatcher := &envoy_route.HeaderMatcher{
		Name: name,
	}
	switch matcher.MatcherType.(type) {
	case *mesh_proto.TrafficRoute_Http_Match_StringMatcher_Prefix:
		headerMatcher.HeaderMatchSpecifier = &envoy_route.HeaderMatcher_PrefixMatch{
			PrefixMatch: matcher.GetPrefix(),
		}
	case *mesh_proto.TrafficRoute_Http_Match_StringMatcher_Exact:
		headerMatcher.HeaderMatchSpecifier = &envoy_route.HeaderMatcher_ExactMatch{
			ExactMatch: matcher.GetExact(),
		}
	case *mesh_proto.TrafficRoute_Http_Match_StringMatcher_Regex:
		headerMatcher.HeaderMatchSpecifier = &envoy_route.HeaderMatcher_SafeRegexMatch{
			SafeRegexMatch: &envoy_type_matcher.RegexMatcher{
				EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{},
				},
				Regex: matcher.GetRegex(),
			},
		}
	}
	return headerMatcher
}

func (c RoutesConfigurer) setPathMatcher(
	matcher *mesh_proto.TrafficRoute_Http_Match_StringMatcher,
	routeMatch *envoy_route.RouteMatch,
) {
	switch matcher.MatcherType.(type) {
	case *mesh_proto.TrafficRoute_Http_Match_StringMatcher_Prefix:
		routeMatch.PathSpecifier = &envoy_route.RouteMatch_Prefix{
			Prefix: matcher.GetPrefix(),
		}
	case *mesh_proto.TrafficRoute_Http_Match_StringMatcher_Exact:
		routeMatch.PathSpecifier = &envoy_route.RouteMatch_Path{
			Path: matcher.GetExact(),
		}
	case *mesh_proto.TrafficRoute_Http_Match_StringMatcher_Regex:
		routeMatch.PathSpecifier = &envoy_route.RouteMatch_SafeRegex{
			SafeRegex: &envoy_type_matcher.RegexMatcher{
				EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{},
				},
				Regex: matcher.GetRegex(),
			},
		}
	}
}

func (c RoutesConfigurer) hasExternal(clusters []envoy_common.Cluster) bool {
	for _, cluster := range clusters {
		if cluster.IsExternalService() {
			return true
		}
	}
	return false
}

func (c RoutesConfigurer) routeAction(clusters []envoy_common.Cluster, modify *mesh_proto.TrafficRoute_Http_Modify) *envoy_route.RouteAction {
	routeAction := &envoy_route.RouteAction{}
	if len(clusters) != 0 {
		routeAction.Timeout = ptypes.DurationProto(clusters[0].Timeout().GetHttp().GetRequestTimeout().AsDuration())
	}
	if len(clusters) == 1 {
		routeAction.ClusterSpecifier = &envoy_route.RouteAction_Cluster{
			Cluster: clusters[0].Name(),
		}
	} else {
		var weightedClusters []*envoy_route.WeightedCluster_ClusterWeight
		var totalWeight uint32
		for _, cluster := range clusters {
			weightedClusters = append(weightedClusters, &envoy_route.WeightedCluster_ClusterWeight{
				Name:   cluster.Name(),
				Weight: &wrappers.UInt32Value{Value: cluster.Weight()},
			})
			totalWeight += cluster.Weight()
		}
		routeAction.ClusterSpecifier = &envoy_route.RouteAction_WeightedClusters{
			WeightedClusters: &envoy_route.WeightedCluster{
				Clusters:    weightedClusters,
				TotalWeight: &wrappers.UInt32Value{Value: totalWeight},
			},
		}
	}
	if c.hasExternal(clusters) {
		routeAction.HostRewriteSpecifier = &envoy_route.RouteAction_AutoHostRewrite{
			AutoHostRewrite: &wrappers.BoolValue{Value: true},
		}
	}
	c.setModifications(routeAction, modify)
	return routeAction
}

func (c RoutesConfigurer) setModifications(routeAction *envoy_route.RouteAction, modify *mesh_proto.TrafficRoute_Http_Modify) {
	if modify.GetPath() != nil {
		switch modify.GetPath().Type.(type) {
		case *mesh_proto.TrafficRoute_Http_Modify_Path_RewritePrefix:
			routeAction.PrefixRewrite = modify.GetPath().GetRewritePrefix()
		case *mesh_proto.TrafficRoute_Http_Modify_Path_Regex:
			routeAction.RegexRewrite = &envoy_type_matcher.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher.RegexMatcher{
					EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
						GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{},
					},
					Regex: modify.GetPath().GetRegex().GetPattern(),
				},
				Substitution: modify.GetPath().GetRegex().GetSubstitution(),
			}
		}
	}

	if modify.GetHost() != nil {
		switch modify.GetHost().Type.(type) {
		case *mesh_proto.TrafficRoute_Http_Modify_Host_Value:
			routeAction.HostRewriteSpecifier = &envoy_route.RouteAction_HostRewriteLiteral{
				HostRewriteLiteral: modify.GetHost().GetValue(),
			}
		case *mesh_proto.TrafficRoute_Http_Modify_Host_FromPath:
			routeAction.HostRewriteSpecifier = &envoy_route.RouteAction_HostRewritePathRegex{
				HostRewritePathRegex: &envoy_type_matcher.RegexMatchAndSubstitute{
					Pattern: &envoy_type_matcher.RegexMatcher{
						EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{},
						},
						Regex: modify.GetHost().GetFromPath().GetPattern(),
					},
					Substitution: modify.GetHost().GetFromPath().GetSubstitution(),
				},
			}
		}
	}
}

func (c *RoutesConfigurer) typedPerFilterConfig(route *envoy_common.Route) (map[string]*any.Any, error) {
	typedPerFilterConfig := map[string]*any.Any{}

	if route.RateLimit != nil {
		rateLimit, err := c.createRateLimit(route.RateLimit.GetConf().GetHttp())
		if err != nil {
			return nil, err
		}
		typedPerFilterConfig["envoy.filters.http.local_ratelimit"] = rateLimit
	}

	return typedPerFilterConfig, nil
}

func (c *RoutesConfigurer) createRateLimit(rlHttp *mesh_proto.RateLimit_Conf_Http) (*any.Any, error) {
	var status *envoy_type_v3.HttpStatus
	var responseHeaders []*envoy_config_core_v3.HeaderValueOption
	if rlHttp.GetOnRateLimit() != nil {
		status = &envoy_type_v3.HttpStatus{
			Code: envoy_type_v3.StatusCode(rlHttp.GetOnRateLimit().GetStatus().GetValue()),
		}
		responseHeaders = []*envoy_config_core_v3.HeaderValueOption{}
		for _, h := range rlHttp.GetOnRateLimit().GetHeaders() {
			responseHeaders = append(responseHeaders, &envoy_config_core_v3.HeaderValueOption{
				Header: &envoy_config_core_v3.HeaderValue{
					Key:   h.GetKey(),
					Value: h.GetValue(),
				},
				Append: h.GetAppend(),
			})
		}
	}

	config := &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix: "rate_limit",
		Status:     status,
		TokenBucket: &envoy_type_v3.TokenBucket{
			MaxTokens: rlHttp.GetRequests(),
			TokensPerFill: &wrappers.UInt32Value{
				Value: rlHttp.GetRequests(),
			},
			FillInterval: rlHttp.GetInterval(),
		},
		FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
			RuntimeKey: "local_rate_limit_enabled",
		},
		FilterEnforced: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
			RuntimeKey: "local_rate_limit_enforced",
		},
		ResponseHeadersToAdd: responseHeaders,
	}

	return proto.MarshalAnyDeterministic(config)
}
