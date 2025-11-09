package resource

import (
	"strings"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/protoyaml"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

func ExtRefFromResourceDef(
	ref *crossplane.Ref,
	def *crossplane.ResourceDef,
) *crossplane.ExtRef {
	return &crossplane.ExtRef{
		Ref:        ref,
		ApiVersion: def.GetApiVersion(),
		Kind:       def.GetKind(),
	}
}

func MarshalWithReplaceOneOffs(def *crossplane.ResourceDef) (string, error) {
	// TODO: ITS BIG CRUTCH TO REPLACE FOR_PROVIDER TO ACCEPTABLE FROM K8S
	yaml, err := protoyaml.Marshal(def)
	if err != nil {
		return "", err
	}
	replacedSymbol := "NONE"
	switch def.GetSpec().GetForProvider().(type) {
	case *crossplane.ResourceDef_Spec_YandexCloudVm:
		replacedSymbol = "yandexCloudVm"
	case *crossplane.ResourceDef_Spec_YandexCloudNetwork:
		replacedSymbol = "yandexCloudNetwork"
	case *crossplane.ResourceDef_Spec_YandexCloudSubnet:
		replacedSymbol = "yandexCloudSubnet"
	}
	return strings.ReplaceAll(string(yaml), replacedSymbol, "forProvider"), nil
}
