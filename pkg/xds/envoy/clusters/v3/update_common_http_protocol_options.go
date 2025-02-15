package clusters

import (
	envoy_cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_upstream_http "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/golang/protobuf/ptypes/any"
	proto2 "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kumahq/kuma/pkg/util/proto"
)

func UpdateCommonHttpProtocolOptions(cluster *envoy_cluster.Cluster, fn func(*envoy_upstream_http.HttpProtocolOptions)) error {
	if cluster.TypedExtensionProtocolOptions == nil {
		cluster.TypedExtensionProtocolOptions = map[string]*any.Any{}
	}
	options := &envoy_upstream_http.HttpProtocolOptions{}
	if any := cluster.TypedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]; any != nil {
		if err := anypb.UnmarshalTo(any, options, proto2.UnmarshalOptions{}); err != nil {
			return err
		}
	}

	fn(options)

	pbst, err := proto.MarshalAnyDeterministic(options)
	if err != nil {
		return err
	}
	cluster.TypedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = pbst
	return nil
}
