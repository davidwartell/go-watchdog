/*
 * Copyright (c) 2026 by David Wartell. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package watchdog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// recordingHeartbeat answers StillRunning with stillRunning and records the fake time of every call.
type recordingHeartbeat struct {
	mu           sync.Mutex
	start        time.Time
	stillRunning func(call int) bool
	calls        []time.Duration
}

func (h *recordingHeartbeat) StillRunning(context.Context) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, time.Since(h.start))
	return h.stillRunning(len(h.calls))
}

func (h *recordingHeartbeat) callTimes() []time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]time.Duration(nil), h.calls...)
}

// TestWatchDog pins when the watchdog cancels its context: at maxRunTime, when a heartbeat reports the task stopped,
// never because of a heartbeat that keeps reporting it running, and not at all once cancelled or its parent ends.
func TestWatchDog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		heartbeat         func(call int) bool // nil: no heartbeat
		heartbeatInterval time.Duration
		cancelAfter       time.Duration // 0: never cancel the instance
		parentAfter       time.Duration // 0: never end the parent
		wantCancelAt      time.Duration
		wantCause         error
		wantCalls         []time.Duration
	}{
		{
			name:         "no heartbeat runs to the max run time",
			wantCancelAt: 10 * time.Second,
			wantCause:    context.Canceled,
		},
		{
			name:              "a heartbeat interval without a heartbeat is ignored",
			heartbeatInterval: time.Second,
			wantCancelAt:      10 * time.Second,
			wantCause:         context.Canceled,
		},
		{
			name:         "a heartbeat with no interval is never called",
			heartbeat:    func(int) bool { return true },
			wantCancelAt: 10 * time.Second,
			wantCause:    context.Canceled,
		},
		{
			name:              "a running heartbeat is called every interval until the max run time",
			heartbeat:         func(int) bool { return true },
			heartbeatInterval: 3 * time.Second,
			wantCancelAt:      10 * time.Second,
			wantCause:         context.Canceled,
			wantCalls:         []time.Duration{3 * time.Second, 6 * time.Second, 9 * time.Second},
		},
		{
			name:              "a heartbeat reporting the task stopped cancels at once",
			heartbeat:         func(call int) bool { return call < 2 },
			heartbeatInterval: 3 * time.Second,
			wantCancelAt:      6 * time.Second,
			wantCause:         context.Canceled,
			wantCalls:         []time.Duration{3 * time.Second, 6 * time.Second},
		},
		{
			name:              "cancelling the instance stops it before the max run time",
			heartbeat:         func(int) bool { return true },
			heartbeatInterval: 3 * time.Second,
			cancelAfter:       4 * time.Second,
			wantCancelAt:      4 * time.Second,
			wantCause:         context.Canceled,
			wantCalls:         []time.Duration{3 * time.Second},
		},
		{
			name:         "the parent ending stops it before the max run time",
			parentAfter:  2 * time.Second,
			wantCancelAt: 2 * time.Second,
			wantCause:    errParentEnded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				parent, endParent := context.WithCancelCause(t.Context())
				defer endParent(nil)

				var heartbeat *recordingHeartbeat
				var dog *Instance
				var ctx context.Context
				if tt.heartbeat != nil {
					heartbeat = &recordingHeartbeat{start: start, stillRunning: tt.heartbeat}
					dog, ctx = NewWatchDogWithHeartbeat(parent, 10*time.Second, heartbeat, tt.heartbeatInterval)
				} else {
					dog, ctx = NewWatchDogWithHeartbeat(parent, 10*time.Second, nil, tt.heartbeatInterval)
				}
				if tt.cancelAfter > 0 {
					time.AfterFunc(tt.cancelAfter, dog.Cancel)
				}
				if tt.parentAfter > 0 {
					time.AfterFunc(tt.parentAfter, func() { endParent(errParentEnded) })
				}

				<-ctx.Done()
				if got := time.Since(start); got != tt.wantCancelAt {
					t.Errorf("cancelled at %v, want %v", got, tt.wantCancelAt)
				}
				if got := context.Cause(ctx); !errors.Is(got, tt.wantCause) {
					t.Errorf("cause %v, want %v", got, tt.wantCause)
				}
				dog.Cancel()
				synctest.Wait()

				var gotCalls []time.Duration
				if heartbeat != nil {
					gotCalls = heartbeat.callTimes()
				}
				if !equalDurations(gotCalls, tt.wantCalls) {
					t.Errorf("heartbeat calls at %v, want %v", gotCalls, tt.wantCalls)
				}
			})
		})
	}
}

// TestNewWatchDog pins that the plain constructor cancels at its max run time and calls no heartbeat.
func TestNewWatchDog(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		dog, ctx := NewWatchDog(t.Context(), 5*time.Second)
		defer dog.Cancel()
		<-ctx.Done()
		if got := time.Since(start); got != 5*time.Second {
			t.Errorf("cancelled at %v, want 5s", got)
		}
	})
}

// errParentEnded is the cause a test ends a watchdog's parent context with.
var errParentEnded = errors.New("parent ended")

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
