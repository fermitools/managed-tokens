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
	"iter"
	"slices"
	"time"
)

// WorkerSpecificConfigOption is a type that represents a worker-specific configuration option.
type WorkerSpecificConfigOption uint8

const (
	// NumRetriesOption is a worker-specific configuration option that represents the number of times a worker should retry a task before giving up.
	NumRetriesOption WorkerSpecificConfigOption = iota
	// RetrySleepOption is a worker-specific configuration option that represents the time.Duration a worker should sleep between retries
	RetrySleepOption
	// InteractiveTokenGetterOption is a worker-specific configuration option that represents whether the token getter should be interactive.
	// It is supported by the GetToken WorkerType and StoreAndGetToken WorkerType
	InteractiveTokenGetterOption
	// AlternateTokenGetterOption is a worker-specific configuration option that represents whether to use an alternate token getter. It is // supported by the GetToken WorkerType and StoreAndGetToken , and the value of the AlternateTokenGetterOption must
	// be of a type that implements the TokenGetter interface.
	AlternateTokenGetterOption
	invalidWorkerSpecificConfigOption
)

// Setters

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

func SetNumRetriesOption(w WorkerType, numRetries uint) ConfigOption {
	return SetWorkerSpecificConfigOption(w, NumRetriesOption, numRetries)
}

func SetRetrySleepOption(w WorkerType, retrySleep time.Duration) ConfigOption {
	return SetWorkerSpecificConfigOption(w, RetrySleepOption, retrySleep)
}

func SetInteractiveTokenGetterOption(w WorkerType, interactive bool) ConfigOption {
	if w != GetToken && w != StoreAndGetToken {
		return ConfigOption(func(*Config) error { return nil }) // No-op
	}
	return SetWorkerSpecificConfigOption(w, InteractiveTokenGetterOption, interactive)
}

func SetAlternateTokenGetterOption(w WorkerType, tokenGetter TokenGetter) ConfigOption {
	return SetWorkerSpecificConfigOption(w, AlternateTokenGetterOption, tokenGetter)
}

// Exported utility helpers

// ValidRetryWorkerTypes returns an iterator over the valid WorkerTypes that support retry configuration options
func ValidRetryWorkerTypes() iter.Seq[WorkerType] {
	validWorkerTypes := []WorkerType{
		PushTokens,
	}
	return func(yield func(w WorkerType) bool) {
		for _, wt := range validWorkerTypes {
			if !yield(wt) {
				return
			}
		}
	}
}

// ValidTokenGetterWorkerTypes returns an iterator over the valid WorkerTypes that support token getter configuration options
func ValidTokenGetterWorkerTypes() iter.Seq[WorkerType] {
	validWorkerTypes := []WorkerType{
		GetToken,
		StoreAndGetToken,
		StoreAndGetTokenInteractive,
	}
	return func(yield func(w WorkerType) bool) {
		for _, wt := range validWorkerTypes {
			if !yield(wt) {
				return
			}
		}
	}
}

// Getters
// getWorkerRetryValueFromConfig retrieves the retry value for a specific worker type from the given configuration.
// It returns the retry value as a uint and a non-nil error if the worker type is not found in the configuration or if the value is not of type uint.
func getWorkerNumRetriesValueFromConfig(c Config, w WorkerType) (uint, error) {
	m, err := getWorkerTypeMapFromConfig(c, w, slices.Collect(ValidRetryWorkerTypes()))
	if err != nil {
		if errors.Is(err, errNoWorkerTypeMapInConfig) {
			return 0, fmt.Errorf("no WorkerType %s map found in Config: %w", w, err)
		}
		return 0, err
	}

	val, ok := m[NumRetriesOption]
	if !ok {
		return 0, fmt.Errorf("no NumRetriesOption found for workerType %s in workerSpecificConfig", w)
	}

	valUInt, ok := val.(uint)
	if !ok {
		return 0, fmt.Errorf("value for workerType %s is not of type uint.  Got type %T", w, val)
	}

	return valUInt, nil
}

// getWorkerRetrySleepValueFromConfig retrieves the retrySleepValue for a specific worker type from the given configuration.
func getWorkerRetrySleepValueFromConfig(c Config, w WorkerType) (time.Duration, error) {
	m, err := getWorkerTypeMapFromConfig(c, w, slices.Collect(ValidRetryWorkerTypes()))
	if err != nil {
		if errors.Is(err, errNoWorkerTypeMapInConfig) {
			return 0, fmt.Errorf("no WorkerType %s map found in Config: %w", w, err)
		}
		return 0, err
	}

	val, ok := m[RetrySleepOption]
	if !ok {
		return 0, fmt.Errorf("no RetrySleepOption found for workerType %s in workerSpecificConfig", w)
	}

	valTime, ok := val.(time.Duration)
	if !ok {
		return 0, fmt.Errorf("value for workerType %s is not of type time.Duration.  Got type %T", w, val)
	}

	return valTime, nil
}

// getInteractiveTokenGetterOptionFromConfig retrieves the interactiveTokenGetterOption for a specific worker type from the given configuration.
// If the worker type is not supported or invalid, an error is returned.
func getInteractiveTokenGetterOptionFromConfig(c Config, w WorkerType) (bool, error) {
	m, err := getWorkerTypeMapFromConfig(c, w, slices.Collect(ValidTokenGetterWorkerTypes()))
	if err != nil {
		if errors.Is(err, errNoWorkerTypeMapInConfig) {
			return false, errors.New("no token getter configuration found for the given worker type")
		}
		return false, err
	}

	// Does that map have the InteractiveTokenGetterOption key set?
	val, ok := m[InteractiveTokenGetterOption]
	if !ok {
		return false, nil // This is not an error case - just that the option is not set
	}

	// Type-check the value
	valBool, ok := val.(bool)
	if !ok {
		return false, fmt.Errorf("value for workerType %s is not of type bool.  Got type %T", w, val)
	}

	return valBool, nil
}

// getAlternateTokenGetterOptionFromConfig retrieves the interactiveTokenGetterOption for a specific worker type from the given configuration.
// If the worker type is not supported or invalid, an error is returned.
func getAlternateTokenGetterOptionFromConfig(c Config, w WorkerType) (TokenGetter, error) {
	m, err := getWorkerTypeMapFromConfig(c, w, slices.Collect(ValidTokenGetterWorkerTypes()))
	if err != nil {
		if errors.Is(err, errNoWorkerTypeMapInConfig) {
			return nil, errors.New("no token getter configuration found for the given worker type")
		}
		return nil, err
	}

	val, ok := m[AlternateTokenGetterOption]
	if !ok {
		return nil, fmt.Errorf("no AlternateTokenGetterOption found for workerType %s in workerSpecificConfig", w)
	}

	valInterface, ok := val.(TokenGetter)
	if !ok {
		return nil, fmt.Errorf("value for workerType %s is not of type TokenGetter.  Got type %T", w, val)
	}

	return valInterface, nil
}

func isValidWorkerSpecificConfigOption(option WorkerSpecificConfigOption) bool {
	return option < invalidWorkerSpecificConfigOption
}

// getWorkerTypeMapFromConfig retrieves the worker-specific configuration map for a given worker type from the provided Config.
// It first checks if the worker type is valid and, if the slice validWorkerTypes is not nil, ensures the worker type is included in that list.
// Returns the configuration map associated with the worker type, or an error if the worker type is invalid, not in the valid list, or not present in the configuration.
func getWorkerTypeMapFromConfig(c Config, w WorkerType, validWorkerTypes []WorkerType) (map[WorkerSpecificConfigOption]any, error) {
	if !isValidWorkerType(w) {
		return nil, errors.New("invalid worker type")
	}

	if validWorkerTypes != nil && !slices.Contains(validWorkerTypes, w) {
		return nil, fmt.Errorf("workerType %s is not in the list of valid worker types %v", w, validWorkerTypes)
	}

	m, ok := c.workerSpecificConfig[w]
	if !ok {
		return nil, errNoWorkerTypeMapInConfig
	}

	return m, nil
}

var errNoWorkerTypeMapInConfig error = errors.New("given worker type not found in worker Config")
