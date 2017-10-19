// Copyright 2016 Google Inc. All rights reserved.
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

package zip

import (
	"fmt"
	"runtime"
)

type RateLimit struct {
	requests    chan request
	completions chan int64

	stop chan struct{}
}

type request struct {
	size     int64
	serviced chan struct{}
}

// NewRateLimit starts a new rate limiter that permits the usage of up to <capacity> at once,
// except when no capacity is in use, in which case the first caller is always permitted
func NewRateLimit(capacity int64) *RateLimit {
	ret := &RateLimit{
		requests:    make(chan request),
		completions: make(chan int64),

		stop: make(chan struct{}),
	}

	go ret.monitorChannels(capacity)

	return ret
}

// RequestExecution blocks until another execution of size <size> can be allowed to run.
func (r *RateLimit) Request(size int64) {
	request := request{
		size:     size,
		serviced: make(chan struct{}, 1),
	}

	// wait for the request to be received
	r.requests <- request

	// wait for the request to be accepted
	<-request.serviced
}

// Finish declares the completion of an execution of size <size>
func (r *RateLimit) Finish(size int64) {
	r.completions <- size
}

// Stop the background goroutine
func (r *RateLimit) Stop() {
	close(r.stop)
}

// monitorChannels processes incoming requests from channels
func (r *RateLimit) monitorChannels(capacity int64) {
	var usedCapacity int64
	var currentRequest *request

	for {
		var requests chan request
		if currentRequest == nil {
			// If we don't already have a queued request, then we should check for a new request
			requests = r.requests
		}

		select {
		case newRequest := <-requests:
			currentRequest = &newRequest
		case amountCompleted := <-r.completions:
			usedCapacity -= amountCompleted

			if usedCapacity < 0 {
				panic(fmt.Sprintf("usedCapacity < 0: %v (decreased by %v)", usedCapacity, amountCompleted))
			}
		case <-r.stop:
			return
		}

		if currentRequest != nil {
			accepted := false
			if usedCapacity == 0 {
				accepted = true
			} else {
				if capacity >= usedCapacity+currentRequest.size {
					accepted = true
				}
			}
			if accepted {
				usedCapacity += currentRequest.size
				currentRequest.serviced <- struct{}{}
				currentRequest = nil
			}
		}
	}
}

// A CPURateLimiter limits the number of active calls based on CPU requirements
type CPURateLimiter struct {
	impl *RateLimit
}

func NewCPURateLimiter(capacity int64) *CPURateLimiter {
	if capacity <= 0 {
		capacity = int64(runtime.NumCPU())
	}
	impl := NewRateLimit(capacity)
	return &CPURateLimiter{impl: impl}
}

func (e CPURateLimiter) Request() {
	e.impl.Request(1)
}

func (e CPURateLimiter) Finish() {
	e.impl.Finish(1)
}

func (e CPURateLimiter) Stop() {
	e.impl.Stop()
}

// A MemoryRateLimiter limits the number of active calls based on Memory requirements
type MemoryRateLimiter struct {
	*RateLimit
}

func NewMemoryRateLimiter(capacity int64) *MemoryRateLimiter {
	if capacity <= 0 {
		capacity = 512 * 1024 * 1024 // 512MB
	}
	impl := NewRateLimit(capacity)
	return &MemoryRateLimiter{RateLimit: impl}
}
