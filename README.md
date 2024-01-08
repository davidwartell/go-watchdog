# go-watchdog

Watchdog will call the cancel function associated with the returned context when a max run time has been exceeded.  
Pass the context returned to a long-running tasks that can be interrupted by a cancelled context.

## Usage

Example
```go
// create a new watchdog
watchDog, watchDogCtx := watchdog.NewWatchDog(
    ctx,
    time.Duration(300) * time.Second,
)
defer watchDog.Cancel()

// SomeLongRunningFunction that we want to run no more than 300 seconds.  Function should exit if watchDogCtx is cancelled.
// Use context aware network/database connections in this function.  If you have long running computations in a loop
// periodically check if watchDogCtx.Err() != nil in SomeLongRunningFunction.
SomeLongRunningFunction(watchDogCtx)
```

## Contributing

Happy to accept PRs.

# Author

**davidwartell**

* <http://github.com/davidwartell>
* <http://linkedin.com/in/wartell>
