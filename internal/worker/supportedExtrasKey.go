// COPYRIGHT 2024 FERMI NATIONAL ACCELERATOR LABORATORY
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
//
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package worker

// supportedExtrasKey is an enumerated key for the Config.Extras map.  Callers wishing to store values
// in the Config.Extras map should use a SupportedExtrasKey as the key
type supportedExtrasKey int

const (
	// DefaultRoleFileTemplate is a key to store the value of the default role file template in the Config.Extras map
	DefaultRoleFileDestinationTemplate supportedExtrasKey = iota
	// FileCopierOptions allows the user to specify options for the file copier
	FileCopierOptions
	// PingOptions allows the user to specify options for the PingAggregatorWorker to use
	PingOptions
	// SSHOptions allows the user to specify options for the PushTokensWorker to use when pushing files to the destination nodes
	SSHOptions
)

func (s supportedExtrasKey) String() string {
	switch s {
	case DefaultRoleFileDestinationTemplate:
		return "DefaultRoleFileDestinationTemplate"
	case FileCopierOptions:
		return "FileCopierOptions"
	case PingOptions:
		return "PingOptions"
	case SSHOptions:
		return "SSHOptions"
	default:
		return "unsupported extras key"
	}
}

// SetSupportedExtrasKeyValue returns a func(*Config) that sets the value for the given supportedExtraskey in the Extras map
func SetSupportedExtrasKeyValue(key supportedExtrasKey, value any) func(*Config) error {
	return func(c *Config) error {
		c.Extras[key] = value
		return nil
	}
}

// GetDefaultRoleFileTemplateValueFromExtras retrieves the default role file template value from the worker.Config,
// and asserts that it is a string.  Callers should check the bool return value to make sure the type assertion
// passes, for example:
//
//	c := worker.NewConfig( // various options )
//	// set the default role file template in here
//	tmplString, ok := GetDefaultRoleFileTemplateValueFromExtras(c)
//	if !ok { // handle missing or incorrect value }
func GetDefaultRoleFileDestinationTemplateValueFromExtras(c *Config) (string, bool) {
	defaultRoleFileDestinationTemplateString, ok := c.Extras[DefaultRoleFileDestinationTemplate].(string)
	return defaultRoleFileDestinationTemplateString, ok
}

// defaultFileCopierOpts assumes that the FileCopier will implement rsync, and thus the default options will render the
// destination file with permissions 0o400
var defaultFileCopierOpts []string = []string{"--perms", "--chmod=u=r,go="}

// GetFileCopierOptionsFromExtras retrieves the file copier options value from the worker.Config,
// and asserts that it is a string.  Callers should check the bool return value to make sure the type assertion
// passes, for example:
//
//	c := worker.NewConfig( // various options )
//	// set the default role file template in here
//	opts, ok := GetFileCopierOptionsFromExtras(c)
//	if !ok { // handle missing or incorrect value }
func GetFileCopierOptionsFromExtras(c *Config) ([]string, bool) {
	_fileCopierOpts, ok := c.Extras[FileCopierOptions]
	if !ok {
		return defaultFileCopierOpts, true
	}
	fileCopierOpts, ok := _fileCopierOpts.([]string)
	return fileCopierOpts, ok
}

// GetPingOptionsFromExtras retrieves the ping options slice from the worker.Config, and asserts
// that it is a []string.  Callers should check the bool return value to make sure that the
// type assertion passes.
func GetPingOptionsFromExtras(c *Config) ([]string, bool) {
	emptyOpts := make([]string, 0)
	_pingOpts, ok := c.Extras[PingOptions]
	if !ok {
		return emptyOpts, true
	}
	pingOpts, ok := _pingOpts.([]string)
	return pingOpts, ok
}

// GetSSHOptionsFromExtras retrieves the SSH options slice from the worker.Config, and asserts
// that it is a []string.  Callers should check the bool return value to make sure that the
// type assertion passes.
func GetSSHOptionsFromExtras(c *Config) ([]string, bool) {
	emptyOpts := make([]string, 0)
	_sshOpts, ok := c.Extras[SSHOptions]
	if !ok {
		return emptyOpts, true
	}
	SSHOpts, ok := _sshOpts.([]string)
	return SSHOpts, ok
}
