package ingress

import (
	"context"
	"reflect"

	mesh_proto "github.com/Kong/kuma/api/mesh/v1alpha1"
	core_mesh "github.com/Kong/kuma/pkg/core/resources/apis/mesh"
	"github.com/Kong/kuma/pkg/core/resources/manager"
	"github.com/Kong/kuma/pkg/core/resources/store"
)

type serviceSet []*mesh_proto.Dataplane_Networking_Ingress_AvailableService

func (set serviceSet) getBy(tags map[string]string) *mesh_proto.Dataplane_Networking_Ingress_AvailableService {
	for _, in := range set {
		if reflect.DeepEqual(in.Tags, tags) {
			return in
		}
	}
	return nil
}

func UpdateAvailableServices(ctx context.Context, rm manager.ResourceManager, ingress *core_mesh.DataplaneResource, others []*core_mesh.DataplaneResource) error {
	availableServices := GetIngressAvailableServices(others)
	if reflect.DeepEqual(availableServices, ingress.Spec.GetNetworking().GetIngress().GetAvailableServices()) {
		return nil
	}
	ingress.Spec.Networking.Ingress.AvailableServices = availableServices
	if err := rm.Update(ctx, ingress); err != nil {
		return err
	}
	return nil
}

func GetIngressAvailableServices(others []*core_mesh.DataplaneResource) []*mesh_proto.Dataplane_Networking_Ingress_AvailableService {
	availableServices := make([]*mesh_proto.Dataplane_Networking_Ingress_AvailableService, 0, len(others))
	for _, dp := range others {
		if dp.Spec.IsIngress() {
			continue
		}
		for _, dpInbound := range dp.Spec.GetNetworking().GetInbound() {
			if dup := serviceSet(availableServices).getBy(dpInbound.GetTags()); dup != nil {
				continue
			}
			availableServices = append(availableServices, &mesh_proto.Dataplane_Networking_Ingress_AvailableService{
				Tags: dpInbound.Tags,
			})
		}
	}
	return availableServices
}

func GetAllDataplanes(resourceManager manager.ReadOnlyResourceManager) ([]*core_mesh.DataplaneResource, error) {
	ctx := context.Background()
	meshes := &core_mesh.MeshResourceList{}
	if err := resourceManager.List(ctx, meshes); err != nil {
		return nil, err
	}
	dataplanes := make([]*core_mesh.DataplaneResource, 0)
	for _, mesh := range meshes.Items {
		dataplaneList := &core_mesh.DataplaneResourceList{}
		if err := resourceManager.List(ctx, dataplaneList, store.ListByMesh(mesh.Meta.GetName())); err != nil {
			return nil, err
		}
		dataplanes = append(dataplanes, dataplaneList.Items...)
	}
	return dataplanes, nil
}
