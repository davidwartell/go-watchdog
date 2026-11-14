/*
 * Copyright (c) 2022-2026 by David Wartell. All Rights Reserved.
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
	"sync"
	"time"
)

type Instance struct {
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	maxRunTime        time.Duration
	heartbeat         Heartbeat
	heartbeatInterval time.Duration
}

type Heartbeat interface {
	StillRunning(ctx context.Context) (ok bool)
}

// NewWatchDog creates a new watchdog Instance. Watchdog will call the cancel function associated with the returned
// context when maxRunTime has been exceeded or Heartbeat.StillRunning return false.  Pass the context returned to a
// long-running tasks that can be interrupted by a cancelled context.
//
// Cancel() should be called on the returned Instance when you are done, or it will not be garbage collected until
// maxRunTime expires.
//
// I'm using watchdog to time limit background tasks running in golang on an IoT sensor.  The Heartbeat callback is used
// to reset (in)visibility of the task in a queue, so it is not dispatched to another sensor.  Could also be used for
// long-running tasks driven by an AWS SQS queue for example.
//
//goland:noinspection GoUnusedExportedFunction
func NewWatchDog(ctx context.Context, maxRunTime time.Duration) (*Instance, context.Context) {
	return NewWatchDogWithHeartbeat(ctx, maxRunTime, nil, 0)
}

// NewWatchDogWithHeartbeat creates a new watch dog same NewWatchDog except in addition a callback is called every
// heartbeatInterval. Cancel() should be called when you are done with the instance, or it will run until maxRunTime
// expires.
func NewWatchDogWithHeartbeat(
	ctx context.Context,
	maxRunTime time.Duration,
	heartbeat Heartbeat,
	heartbeatInterval time.Duration,
) (*Instance, context.Context) {
	wdCtx, wdCancel := context.WithCancel(ctx)
	dog := &Instance{
		ctx:               wdCtx,
		cancel:            wdCancel,
		maxRunTime:        maxRunTime,
		heartbeat:         heartbeat,
		heartbeatInterval: heartbeatInterval,
	}

	dog.wg.Go(dog.run)
	return dog, wdCtx
}

// Cancel cancels the watchdog's context and waits for the watchdog to stop.
func (i *Instance) Cancel() {
	i.cancel()
	i.wg.Wait()
}

// run waits until maxRunTime expires, a heartbeat reports the task stopped, or the context ends.
func (i *Instance) run() {
	maxRunTimeTimer := time.NewTimer(i.maxRunTime)
	defer maxRunTimeTimer.Stop()

	for i.waitOnce(maxRunTimeTimer.C) {
	}
}

// waitOnce waits for the next heartbeat, maxRunTime, or the end of the context, and reports whether to keep watching.
//
// Each wait gets its own heartbeat timer, so no timer is ever reset: the result does not depend on the Go 1.23 timer
// semantics a consumer's go.mod may not select.
func (i *Instance) waitOnce(maxRunTime <-chan time.Time) bool {
	// a nil channel is never ready, so without a heartbeat only maxRunTime or the context end the wait
	var heartbeatChan <-chan time.Time
	if i.heartbeat != nil && i.heartbeatInterval > 0 {
		heartbeatTimer := time.NewTimer(i.heartbeatInterval)
		defer heartbeatTimer.Stop()
		heartbeatChan = heartbeatTimer.C
	}

	select {
	case <-heartbeatChan:
		if i.heartbeat.StillRunning(i.ctx) {
			return true
		}
	case <-maxRunTime:
	case <-i.ctx.Done():
		return false
	}
	i.cancel()
	return false
}
