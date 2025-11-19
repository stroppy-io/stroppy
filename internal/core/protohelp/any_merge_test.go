package protohelp

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestMergeAnyMessages(t *testing.T) {
	src := &panel.WorkflowTask_DeployDatabase_Output{
		DatabaseDeployment: &crossplane.Deployment{
			Id:             "test-deployment-id",
			SupportedCloud: crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX,
		},
	}
	dst := &panel.WorkflowTask_DeployStroppy_Input{}
	srcAny, err := anypb.New(src)
	require.NoError(t, err)
	dstAny, err := anypb.New(dst)
	require.NoError(t, err)
	merged, err := MergeAnyMessages(srcAny, dstAny)
	require.NoError(t, err)

	newdst := &panel.WorkflowTask_DeployStroppy_Input{}
	require.NoError(t, anypb.UnmarshalTo(merged, newdst, proto.UnmarshalOptions{}))
	require.NotNil(t, newdst.GetDatabaseDeployment())
	require.Equal(t, "test-deployment-id", newdst.GetDatabaseDeployment().GetId())
	require.Equal(t, crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX, newdst.GetDatabaseDeployment().GetSupportedCloud())
}
