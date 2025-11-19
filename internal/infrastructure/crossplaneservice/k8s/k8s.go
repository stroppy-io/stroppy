package k8s

import (
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

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

var ErrResourceNotFound = fmt.Errorf("resource not found")

type Client struct {
	dynamicK8S         *dynamic.DynamicClient
	discovery          *discovery.DiscoveryClient
	restMapper         *restmapper.DeferredDiscoveryRESTMapper
	decodingSerializer runtime.Serializer
}

func NewClient(
	k8sConfigPath string,
) (*Client, error) {
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
	return &Client{
		dynamicK8S:         dynClient,
		discovery:          discoveryClient,
		restMapper:         restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient)),
		decodingSerializer: yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme),
	}, nil
}

func (c *Client) CreateResource(
	ctx context.Context,
	resource *crossplane.Resource,
) error {
	obj := &unstructured.Unstructured{}
	_, gvk, err := c.decodingSerializer.Decode([]byte(resource.GetResourceYaml()), nil, obj)
	if err != nil {
		return fmt.Errorf("failed to decode yaml: %w", err)
	}
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to find REST mapping: %w", err)
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
		return fmt.Errorf("failed to marshal object to JSON: %w", err)
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
		return fmt.Errorf("failed to apply resource: %w", err)
	}

	return nil
}

func (c *Client) UpdateResourceFromRemote(
	ctx context.Context,
	resource *crossplane.Resource,
) (*crossplane.Resource, error) {
	// 3. Определяем GVR
	gvk := schema.FromAPIVersionAndKind(resource.GetRef().GetApiVersion(), resource.GetRef().GetKind())
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find REST mapping: %w", err)
	}
	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if resource.GetRef().GetRef().GetNamespace() == "" {
			return nil, fmt.Errorf("namespace must be specified for namespaced resource (%s)", resource.GetRef().GetKind())
		}
		ri = c.dynamicK8S.Resource(mapping.Resource).Namespace(resource.GetRef().GetRef().GetNamespace())
	} else {
		ri = c.dynamicK8S.Resource(mapping.Resource)
	}
	obj, err := ri.Get(ctx, resource.GetRef().GetRef().GetName(), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceNotFound
		}
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}
	synced := getCondition(obj, syncedCondition)
	ready := getCondition(obj, readyCondition)
	externalID, _, _ := unstructured.NestedString(obj.Object,
		statusField, atProviderField, idField)

	resource.Synced = synced == trueString
	resource.Ready = ready == trueString
	resource.ExternalId = externalID
	return resource, nil
}

func (c *Client) DeleteResource(
	ctx context.Context,
	ref *crossplane.ExtRef,
) error {
	gvk := schema.FromAPIVersionAndKind(ref.GetApiVersion(), ref.GetKind())
	mapping, err := c.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to find REST mapping: %w", err)
	}
	var ri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if ref.GetRef().GetNamespace() == "" {
			return fmt.Errorf(
				"namespace must be specified for namespaced resource (%s)",
				ref.GetKind(),
			)
		}
		ri = c.dynamicK8S.Resource(mapping.Resource).Namespace(ref.GetRef().GetNamespace())
	} else {
		ri = c.dynamicK8S.Resource(mapping.Resource)
	}
	if err := ri.Delete(ctx, ref.GetRef().GetName(), metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource already deleted or doesn't exist - consider it success

		}
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	return nil
}
