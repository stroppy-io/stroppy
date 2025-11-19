package yandex

import (
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/defaults"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/scripting"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"sigs.k8s.io/yaml"
)

type userDataYamlDataSourceEc2 struct {
	StrictID bool `yaml:"strict_id"`
}

type userDataYamlDataSource struct {
	Ec2 userDataYamlDataSourceEc2 `yaml:"Ec2"`
}

type userDataYamlUsers struct {
	Name              string   `yaml:"name"`
	Sudo              string   `yaml:"sudo"`
	Shell             string   `yaml:"shell"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys"`
}

type UserDataYaml struct {
	Datasource userDataYamlDataSource `yaml:"datasource"`
	SSHPwauth  string                 `yaml:"ssh_pwauth"`
	Users      []userDataYamlUsers    `yaml:"users"`
	Runcmd     [][]string             `yaml:"runcmd"`
}

func (u *UserDataYaml) Serialize() ([]byte, error) {
	return yaml.Marshal(u)
}

const (
	defaultUserDataYamlName      = "st-t-postgres"
	defaultUserDataYamlShell     = "/bin/bash"
	defaultUserDataYamlSudo      = "ALL=(ALL) NOPASSWD:ALL"
	defaultUserDataYamlSSHPwauth = "no"
)

func newUserDataYamlUsers(
	username string,
	sshAuthorizedKeys []string,
	runCmd [][]string,
) *UserDataYaml {
	return &UserDataYaml{
		Datasource: userDataYamlDataSource{
			Ec2: userDataYamlDataSourceEc2{
				StrictID: false,
			},
		},
		SSHPwauth: defaultUserDataYamlSSHPwauth,
		Users: []userDataYamlUsers{
			{
				Name:              username,
				Sudo:              defaultUserDataYamlSudo,
				Shell:             defaultUserDataYamlShell,
				SSHAuthorizedKeys: sshAuthorizedKeys,
			},
		},
		Runcmd: runCmd,
	}
}

const defaultScriptName = "stroppy-cloud-script.sh"

func NewUserDataWithScript(
	user *crossplane.SshUser,
	script *crossplane.Deployment_Strategy_Scripting,
) *UserDataYaml {
	username := defaults.StringOrDefault(
		user.GetName(),
		defaultUserDataYamlName,
	)
	scriptBase64, err := scripting.BuildBase64Script(script)
	if err != nil {
		return nil
	}
	cmd := fmt.Sprintf(
		"echo \"%s\" | base64 -d > %[2]s && chmod +x %[2]s && ./%[2]s",
		scriptBase64,
		defaultScriptName,
	)
	return newUserDataYamlUsers(
		username,
		user.GetSshAuthorizedKeys(),
		[][]string{
			{
				"su",
				username,
				"-c",
				cmd,
			},
		},
	)
}

func NewUserDataWithEmptyScript(user *crossplane.SshUser) *UserDataYaml {
	username := defaults.StringOrDefault(
		user.GetName(),
		defaultUserDataYamlName,
	)
	return newUserDataYamlUsers(
		username,
		user.GetSshAuthorizedKeys(),
		nil,
	)
}
