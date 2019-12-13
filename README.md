Blackfire Probe for Go
======================

Welcome to the Blackfire probe for go! This document will help you integrate the Blackfire probe into your go applications.

You can start profiling as many times as you like, but only one profiling session can be active at a time.


Prerequisites
-------------

Blackfire integration will only run if you have the Blackfire agent installed. Please refer to the [installation docs](https://blackfire.io/docs/up-and-running/installation).



Adding the probe to your application
------------------------------------

First, import the Blackfire module:

```golang
import "github.com/blackfire/go-blackfire"
```

There are multiple ways to profile:


### Trigger from a POSIX signal

With this approach, your application will profile for a set period of time whenever it receives a POSIX signal.

For example, to profile for 5 seconds each time it receives `SIGUSR1`:

```golang
if err := blackfire.TriggerOnSignal(syscall.SIGUSR1, 5 * time.Second); err != nil {
	
}
```

You can then send your process a signal like so:

Example: If your server PID is 18283:

```
$ kill -SIGUSR1 18283
```

**Remember:** Only one profile may run at a time. If a second profile trigger arrives while a profile is currently being run, it will be ignored (and output a log message).


### Trigger profiling manually

With this approach, you trigger via an API call, which you can then hook up to anything you want (for example, receiving a specific magic HTTP request).

```golang
err := blackfire.ProfileFor(2 * time.Second)
```

With this approach, you can set up your own triggers 


### Start and stop manually

This approach offers the most control, but **you must be sure to stop profiling when you're done**, or else it will continue filling the profiler buffer forever!

```golang
err := blackfire.StartProfiling()
```

```golang
err := blackfire.StopProfiling()
```



Running your application with profiling enabled
-----------------------------------------------

The easiest way to enable profiling is to use the [POSIX signal approach](#trigger-from-a-posix-signal) and run your application via `blackfire run`:

```
$ blackfire run my_application
2019/12/11 11:44:01 INFO: My application started
```

The `blackfire` tool will automatically set up the values that the application needs in order to profile and upload the data. If you run your application directly (i.e. not via `blackfire run`), all profiling will be disabled (it will write a log message to this effect):

```
$ my_application
2019/12/11 11:44:44 Profiling is disabled because the required variables are not set. To enable profiling, run via 'blackfire run' or call SetAgentSocket() and SetBlackfireQuery() manually.
2019/12/11 11:44:45 INFO: My application started
```

The other approaches require you to monitor the [error type](#error-types) returned by `ProfileFor()` or `StartProfiling()`. When your application is not launched via `blackfire run`, the returned error will have `ErrorType` = `ProfilerErrorProfilingDisabled`.

### Error Types

The following special errors may be returned when attempting to profile:

 * `ProfilerErrorAlreadyProfiling`: The profiler is already running. You must wait for it to finish.
 * `ProfilerErrorProfilingDisabled`: Profiling is disabled because your app wasn't started via `blackfire run`, and you haven't set the agent socket or Blackfire query.

These error types allow you to intelligently ignore certain classes of errors (for example, allowing your app to run without profiling by launching it without `blackfire run`).



Advanced Usage
--------------

The profiler provides a number of APIs for more advanced usage:

 * `IsRunningViaBlackfire()`: Returns true if your application was launched using `blackfire run`.

 * `SetAgentSocket()`: Sets the socket type and address to use for connecting to the agent. Example: `tcp://127.0.0.1:40635`

 * `SetBlackfireQuery()`: Sets the Blackfire query string to use when communicating with the agent. 

 * `IsProfiling()`: Returns true if the profiler is currently profiling.
