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

import "iter"

// WorkerType is a type that represents the kind of worker being referenced.  Its main use is to set configuration values that are
// worker-specific, like retry counts, timeouts, etc.
type WorkerType uint8

const (
	GetKerberosTickets WorkerType = iota
	GetToken
	StoreAndGetToken
	PingAggregator
	PushTokens
	invalid
)

// validWorkerTypes returns an iterator of all valid WorkerType values.
func validWorkerTypes() iter.Seq[WorkerType] {
	return func(yield func(WorkerType) bool) {
		for w := range invalid {
			if !yield(WorkerType(w)) {
				return
			}
		}
	}
}

func (wt WorkerType) String() string {
	switch wt {
	case GetKerberosTickets:
		return "GetKerberosTickets"
	case GetToken:
		return "GetToken"
	case StoreAndGetToken:
		return "StoreAndGetToken"
	case PingAggregator:
		return "PingAggregator"
	case PushTokens:
		return "PushTokens"
	default:
		return "Unknown"
	}
}

// Worker returns the Worker function associated with the WorkerType.
func (w WorkerType) Worker() Worker {
	switch w {
	case GetKerberosTickets:
		return getKerberosTicketsWorker
	case GetToken:
		return getTokenWorker
	case StoreAndGetToken:
		return storeAndGetTokenWorker
	case PingAggregator:
		return pingAggregatorWorker
	case PushTokens:
		return pushTokensWorker
	default:
		return nil
	}
}

func isValidWorkerType(w WorkerType) bool {
	return w < invalid
}
