Blackfire Probe for Go
======================

Welcome to the Blackfire probe for go! This document will help you integrate the
Blackfire probe into your go applications.

You can generate as many profiles as you like, but only one profiling session
can be active at a time.

Beta Notes
----------

This is still a beta release of the go probe. As such, none of the APIs should
be considered finalized; they may change in a future release.

Prerequisites
-------------

The probe requires the Blackfire agent to be installed.
Please refer to the [installation docs](https://blackfire.io/docs/up-and-running/installation).

Usage
-----

**Note:** Only one profile may run at a time. Attempting to start another
profile while one is still running will return `ProfilerErrorAlreadyProfiling`.

### Installing the HTTP Handler

```golang
import "github.com/blackfire/go-blackfire/http"

func installProfilerHttpServer() {
	if err := http.StartServer("localhost:6020"); err != nil {
		log.Println(err)
	}
}
```

The following HTTP paths will be available:

| Path                | Description                                           |
| ------------------- | ----------------------------------------------------- |
| `/start`            | Start profiling                                       |
| `/start?duration=x` | Profile for the specified duration in seconds (float) |
| `/stop`             | Stop profiling                                        |

The HTTP paths do not return any data; only code 200 on successful trigger.

### Installing the Signal Handler

```golang
import "github.com/blackfire/go-blackfire/signal"

func installProfilerSignalHandlers() {
	if err := signal.StartOnSignal(syscall.SIGUSR1, 5*time.Second); err != nil {
		log.Println(err)
	}
	if err := signal.StopOnSignal(syscall.SIGUSR2); err != nil {
		log.Println(err)
	}
}
```

### Advanced Profiling

```golang
import "github.com/blackfire/go-blackfire"

func runProfiler() {
	if err := blackfire.StartProfiling(); err != nil {
		log.Println(err)
	}

	doSomeThings()

	if err := blackfire.StopProfiling(); err != nil {
		log.Println(err)
	}
}

func profileFor5Seconds() {
	if err := blackfire.ProfileFor(5 * time.Second); err != nil {
		log.Println(err)
	}
}
```

Advanced API
------------

The Blackfire probe also provides a number of APIs for more advanced needs:

- `SetAgentSocket()`: Sets the socket type and address to use for connecting to
  the agent. Example: `tcp://127.0.0.1:40635`
- `SetBlackfireQuery()`: Sets the Blackfire query string to use when
  communicating with the agent.
- `IsProfiling()`: Returns true if the profiler is currently profiling.
- `SetMaxProfileDuration()`: Sets the maximum duration that a profiling session
  may run. At this point, it will be automatically terminated. Defaults to 30 minutes.
- `ProfileWithCallback()`: Run the profiler, and call a callback on completion.

Running your application with profiling enabled
-----------------------------------------------

The easiest way to enable profiling is to start your application via `blackfire run`:

```
$ blackfire run my_application
2019/12/11 11:44:01 INFO: My application started
```

The `blackfire` tool will automatically set up the environment values that the
probe uses in order to profile and upload the data. If you run your application
directly (i.e. not via `blackfire run`), all profiling will be disabled, and it
will write a log message to this effect:

```
$ my_application
2019/12/11 11:44:44 Profiling is disabled because the required variables are not
set. To enable profiling,run via 'blackfire run' or call SetAgentSocket() and
SetBlackfireQuery() manually.
2019/12/11 11:44:45 INFO: My application started
```

When profiling is disabled, all profiling functions will return
`ProfilerErrorProfilingDisabled`.
