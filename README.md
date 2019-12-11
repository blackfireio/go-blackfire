Blackfire Probe for Go
======================

Welcome to the Blackfire probe for go! This document will help you integrate the Blackfire probe into your go applications.


Prerequisites
-------------

Blackfire integration will only run if you have the Blackfire agent installed. Please refer to the [installation docs](https://blackfire.io/docs/up-and-running/installation).



Adding the probe to your application
------------------------------------

First, import the Blackfire module:

```golang
import "github.com/blackfire/go-blackfire"
```

There are multiple ways to trigger profiling:

1. Trigger whenever a signal is received:

```golang
blackfire.TriggerOnSignal(syscall.SIGUSR1, 5 * time.Second)
```

With this approach, your application will profile for 5 seconds whenever it receives a `SIGUSR1` signal.


2. Trigger profiling manually

```golang
err := blackfire.ProfileFor(2 * time.Second)
```

With this approach, you can set up your own triggers (such as receiving a specific HTTP request, for example).


**Note:** Only one profile may run at a time. If a second profile trigger arrives while a profile is currently being run, it will be ignored (and output a log message).



Running your application with profiling enabled
-----------------------------------------------

The easiest way to enable profiling is to run your application via `blackfire run`:

```
$ blackfire run my_application
2019/12/11 11:44:01 INFO: My application started
```

The `blackfire` tool will automatically set up the values that the application needs in order to profile and upload the data. Running your application by itself (not via `blackfire run`) will disable all profiling functionality (it will emit a message to the log when your application attempts to set up profiling):

```
$ my_application
2019/12/11 11:44:44 Profiling is disabled: Blackfire agent socket not set. Run via 'blackfire run' or call SetAgentSocket()
2019/12/11 11:44:45 INFO: My application started
```

For more complex scenarios, you can set the agent socket and Blackfire query manually from within your application:

```golang
blackfire.SetAgentSocket(agentAndSocket)
blackfire.SetBlackfireQuery(blackfireQuery)
```
