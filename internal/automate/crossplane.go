package automate

import (
	"connectrpc.com/connect"
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

type CrossplaneApi struct {
	*crossplane.UnimplementedCrossplaneServer

	dynamicK8S         *dynamic.DynamicClient
	discovery          *discovery.DiscoveryClient
	restMapper         *restmapper.DeferredDiscoveryRESTMapper
	decodingSerializer runtime.Serializer
}

func NewCrossplaneApi(
	k8sConfigPath string,
) (*CrossplaneApi, error) {
	config, err := clientcmd.BuildConfigFromFlags("", k8sConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from %s: %v", k8sConfigPath, err)
	}
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}
	return &CrossplaneApi{
		UnimplementedCrossplaneServer: &crossplane.UnimplementedCrossplaneServer{},
		dynamicK8S:                    dynClient,
		discovery:                     discoveryClient,
		restMapper:                    restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient)),
		decodingSerializer:            yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme),
	}, nil
}

func (c *CrossplaneApi) CreateResource(
	ctx context.Context,
	request *crossplane.CreateResourceRequest,
) (*crossplane.ResourceWithStatus, error) {
	yamlStr, err := resource.MarshalWithReplaceOneOffs(request.GetResource())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource def: %w", err)
	}
	obj := &unstructured.Unstructured{}
	_, gvk, err := c.decodingSerializer.Decode([]byte(yamlStr), nil, obj)
	if err != nil {
		return nil, fmt.Errorf("failed to decode yaml: %w", err)
	}
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find REST mapping: %w", err)
	}
	var dr dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if obj.GetNamespace() == "" {
			obj.SetNamespace("default")
		}
		dr = c.dynamicK8S.Resource(mapping.Resource).Namespace(obj.GetNamespace())
	} else {
		dr = c.dynamicK8S.Resource(mapping.Resource)
	}
	obj.SetManagedFields(nil) // optional cleanup
	data, err := obj.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}
	_, err = dr.Patch(
		ctx,
		obj.GetName(),
		types.ApplyPatchType,
		data,
		metav1.PatchOptions{
			FieldManager: fieldManager,
			Force:        pointer(true),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to apply resource: %w", err)
	}

	return &crossplane.ResourceWithStatus{
		Ref:          request.GetRef(),
		ResourceDef:  request.GetResource(),
		ResourceYaml: yamlStr,
		Synced:       false,
		Ready:        false,
		ExternalId:   "",
	}, nil
}

func (c *CrossplaneApi) CreateResourcesMany(
	ctx context.Context,
	request *crossplane.CreateResourcesManyRequest,
) (*crossplane.CreateResourcesManyResponse, error) {
	resWithStatus := make([]*crossplane.ResourceWithStatus, 0)
	for _, res := range request.GetResources() {
		re, err := c.CreateResource(ctx, &crossplane.CreateResourceRequest{
			Resource: res,
		})
		if err != nil {
			return nil, err
		}
		resWithStatus = append(resWithStatus, re)
	}
	return &crossplane.CreateResourcesManyResponse{
		Responses: resWithStatus,
	}, nil
}

func (c *CrossplaneApi) GetResourceStatus(
	ctx context.Context,
	request *crossplane.GetResourceStatusRequest,
) (*crossplane.GetResourceStatusResponse, error) {
	// 3. Определяем GVR
	gvk := schema.FromAPIVersionAndKind(request.GetRef().GetApiVersion(), request.GetRef().GetKind())
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find REST mapping: %w", err)
	}
	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if request.GetRef().GetRef().GetNamespace() == "" {
			return nil, fmt.Errorf("namespace must be specified for namespaced resource (%s)", request.GetRef().GetKind())
		}
		ri = c.dynamicK8S.Resource(mapping.Resource).Namespace(request.GetRef().GetRef().GetNamespace())
	} else {
		ri = c.dynamicK8S.Resource(mapping.Resource)
	}
	obj, err := ri.Get(ctx, request.GetRef().GetRef().GetName(), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("resource not found: %w", err))
		}
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}
	synced := getCondition(obj, syncedCondition)
	ready := getCondition(obj, readyCondition)
	externalID, _, _ := unstructured.NestedString(obj.Object,
		statusField, atProviderField, idField)

	return &crossplane.GetResourceStatusResponse{
		Synced:     synced == trueString,
		Ready:      ready == trueString,
		ExternalId: externalID,
	}, nil
}

func (c *CrossplaneApi) DeleteResource(
	ctx context.Context,
	request *crossplane.DeleteResourceRequest,
) (*crossplane.DeleteResourceResponse, error) {
	gvk := schema.FromAPIVersionAndKind(request.GetRef().GetApiVersion(), request.GetRef().GetKind())
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find REST mapping: %w", err)
	}
	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if request.GetRef().GetRef().GetNamespace() == "" {
			return nil, fmt.Errorf(
				"namespace must be specified for namespaced resource (%s)",
				request.GetRef().GetKind(),
			)
		}
		ri = c.dynamicK8S.Resource(mapping.Resource).Namespace(request.GetRef().GetRef().GetNamespace())
	} else {
		ri = c.dynamicK8S.Resource(mapping.Resource)
	}
	if err := ri.Delete(ctx, request.GetRef().GetRef().GetName(), metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource already deleted or doesn't exist - consider it success
			return &crossplane.DeleteResourceResponse{
				Synced: false,
			}, nil
		}
		return nil, fmt.Errorf("failed to delete resource: %w", err)
	}

	return &crossplane.DeleteResourceResponse{
		Synced: false,
	}, nil
}

func (c *CrossplaneApi) DeleteResourcesMany(
	ctx context.Context,
	request *crossplane.DeleteResourcesManyRequest,
) (*crossplane.DeleteResourcesManyResponse, error) {
	responses := make([]*crossplane.DeleteResourceResponse, 0)
	for _, ref := range request.GetRefs() {
		re, err := c.DeleteResource(ctx, &crossplane.DeleteResourceRequest{
			Ref: ref,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete resource: %w", err)
		}
		responses = append(responses, re)
	}
	return &crossplane.DeleteResourcesManyResponse{
		Responses: responses,
	}, nil
}
