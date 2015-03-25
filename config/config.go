// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"android/soong/common"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

var _ common.Config = (*Config)(nil)

// The configuration file name
const ConfigFileName = "soong.config"

// A FileConfigurableOptions contains options which can be configured by the
// config file. These will be included in the config struct.
type FileConfigurableOptions struct {
}

func NewFileConfigurableOptions() FileConfigurableOptions {
	f := FileConfigurableOptions{}
	return f
}

// A Config object represents the entire build configuration for Blue.
type Config struct {
	FileConfigurableOptions

	srcDir  string // the path of the root source directory
	envDeps map[string]string
}

// loads configuration options from a JSON file in the cwd.
func loadFromConfigFile(config *Config) error {
	// Make a proxy config
	var configProxy FileConfigurableOptions

	// Try to open the file
	configFileReader, err := os.Open(ConfigFileName)
	defer configFileReader.Close()
	if os.IsNotExist(err) {
		// Need to create a file, so that blueprint & ninja don't get in
		// a dependency tracking loop.
		// Make a file-configurable-options with defaults, write it out using
		// a json writer.
		configProxy = NewFileConfigurableOptions()
		err = saveToConfigFile(configProxy)
		if err != nil {
			return err
		}
	} else {
		// Make a decoder for it
		jsonDecoder := json.NewDecoder(configFileReader)
		err = jsonDecoder.Decode(&configProxy)
		if err != nil {
			return fmt.Errorf("config file: %s did not parse correctly: "+err.Error(), ConfigFileName)
		}
	}

	// Copy the configurable options out of the config_proxy into the config,
	// and we're done!
	config.FileConfigurableOptions = configProxy

	// No error
	return nil
}

func saveToConfigFile(config FileConfigurableOptions) error {
	data, err := json.MarshalIndent(&config, "", "    ")
	if err != nil {
		return fmt.Errorf("cannot marshal config data: %s", err.Error())
	}

	configFileWriter, err := os.Create(ConfigFileName)
	if err != nil {
		return fmt.Errorf("cannot create empty config file %s: %s\n", ConfigFileName, err.Error())
	}
	defer configFileWriter.Close()

	_, err = configFileWriter.Write(data)
	if err != nil {
		return fmt.Errorf("default config file: %s could not be written: %s", ConfigFileName, err.Error())
	}

	return nil
}

// New creates a new Config object.  The srcDir argument specifies the path to
// the root source directory. It also loads the config file, if found.
func New(srcDir string) (*Config, error) {
	// Make a config with default options
	config := &Config{
		srcDir:  srcDir,
		envDeps: make(map[string]string),
	}

	// Load any configurable options from the configuration file
	err := loadFromConfigFile(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) SrcDir() string {
	return c.srcDir
}

// HostGoOS returns the OS of the system that the Go toolchain is being run on.
func (c *Config) HostGoOS() string {
	return runtime.GOOS
}

// PrebuiltOS returns the name of the host OS used in prebuilts directories
func (c *Config) PrebuiltOS() string {
	switch runtime.GOOS {
	case "linux":
		return "linux-x86"
	case "darwin":
		return "darwin-x86"
	default:
		panic("Unknown GOOS")
	}
}

// GoRoot returns the path to the root directory of the Go toolchain.
func (c *Config) GoRoot() string {
	return fmt.Sprintf("%s/prebuilts/go/%s", c.srcDir, c.PrebuiltOS())
}

func (c *Config) CpPreserveSymlinksFlags() string {
	switch c.HostGoOS() {
	case "darwin":
		return "-R"
	case "linux":
		return "-d"
	default:
		return ""
	}
}

func (c *Config) Getenv(key string) string {
	var val string
	var exists bool
	if val, exists = c.envDeps[key]; !exists {
		val = os.Getenv(key)
		c.envDeps[key] = val
	}
	return val
}

func (c *Config) EnvDeps() map[string]string {
	return c.envDeps
}
