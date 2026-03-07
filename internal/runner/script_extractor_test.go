package runner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_spyProxyObject(t *testing.T) {
	vm := createVM()
	accessedProps := []string{}
	proxy := spyProxyObject(vm, vm.NewObject(), &accessedProps)

	require.NoError(t, vm.Set("__ENV", proxy))

	_, err := vm.RunString(`
__ENV.some;
__ENV.other;
__ENV.__some_secret || "secret";
`)
	require.NoError(t, err)
	require.Equal(t, []string{"some", "other", "__some_secret"}, accessedProps)
}

func Test_stepSpy(t *testing.T) {
	vm := createVM()
	steps := []string{}
	require.NoError(t, vm.Set("Step", stepSpy(vm, &steps)))
	v, err := vm.RunString(`
Step("other step", undefined);
Step("my great step", ()=>{ return "wow" });
`)
	require.NoError(t, err)
	require.Equal(t, "wow", v.ToString().String())
	require.Equal(t, []string{"other step", "my great step"}, steps)
}

func Test_parseGroupsSpy(t *testing.T) {
	vm := createVM()
	accessedProps := []SQLSection{}

	require.NoError(t, vm.Set("parse_sql_with_groups", parseSectionsSpy(&accessedProps)))

	_, err := vm.RunString(`
const groups = parse_sql_with_groups("", null);
groups("some group directly");
const group_name = "my dynamic name"
groups(group_name);
`)
	require.NoError(t, err)
	require.Equal(
		t,
		[]SQLSection{
			{Name: "some group directly"},
			{Name: "my dynamic name"},
		},
		accessedProps,
	)
}
