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

import (
	"errors"
	"fmt"
	"time"
)

// WorkerSpecificConfigOption is a type that represents a worker-specific configuration option.
type WorkerSpecificConfigOption uint8

const (
	// NumRetriesOption is a worker-specific configuration option that represents the number of times a worker should retry a task before giving up.
	NumRetriesOption WorkerSpecificConfigOption = iota
	// RetrySleepOption is a worker-specific configuration option that represents the time.Duration a worker should sleep between retries
	RetrySleepOption
	invalidWorkerSpecificConfigOption
)

// SetWorkerSpecificConfigOption returns a ConfigOption that sets a worker-specific configuration option
// to the given value.
func SetWorkerSpecificConfigOption(w WorkerType, option WorkerSpecificConfigOption, value any) ConfigOption {
	return ConfigOption(func(c *Config) error {
		if !isValidWorkerType(w) {
			return errors.New("invalid worker type")
		}

		if !isValidWorkerSpecificConfigOption(option) {
			return errors.New("invalid worker-specific configuration option")
		}

		if c.workerSpecificConfig == nil {
			c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
		}

		if c.workerSpecificConfig[w] == nil {
			c.workerSpecificConfig[w] = make(map[WorkerSpecificConfigOption]any)
		}

		c.workerSpecificConfig[w][option] = value
		return nil
	})
}

// getWorkerRetryValueFromConfig retrieves the retry value for a specific worker type from the given configuration.
// It returns the retry value as a uint and a non-nil error if the worker type is not found in the configuration or if the value is not of type uint.
func getWorkerNumRetriesValueFromConfig(c Config, w WorkerType) (uint, error) {
	if !isValidWorkerType(w) {
		return 0, errors.New("invalid worker type")
	}

	val, ok := c.workerSpecificConfig[w]
	if !ok {
		return 0, fmt.Errorf("workerType %s not found in workerSpecificConfig", w)
	}

	valUInt, ok := val[NumRetriesOption].(uint)
	if !ok {
		return 0, fmt.Errorf("value for workerType %s is not of type uint.  Got type %T", w, val)
	}

	return valUInt, nil
}

// getWorkerRetrySleepValueFromConfig retrieves the retrySleepValue for a specific worker type from the given configuration.
func getWorkerRetrySleepValueFromConfig(c Config, w WorkerType) (time.Duration, error) {
	if !isValidWorkerType(w) {
		return 0, errors.New("invalid worker type")
	}

	val, ok := c.workerSpecificConfig[w]
	if !ok {
		return 0, fmt.Errorf("workerType %s not found in workerSpecificConfig", w)
	}

	valTime, ok := val[RetrySleepOption].(time.Duration)
	if !ok {
		return 0, fmt.Errorf("value for workerType %s is not of type time.Duration.  Got type %T", w, val)
	}

	return valTime, nil
}

func isValidWorkerSpecificConfigOption(option WorkerSpecificConfigOption) bool {
	return option < invalidWorkerSpecificConfigOption
}
