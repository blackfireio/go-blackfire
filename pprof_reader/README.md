PProf Reader
============

Library to read profiles written in go's pprof format.

PProf is sampled data encoded into protobufs, which is then gzipped.
`internal/profile` is copied directly from golang's
`src/runtime/pprof/internal/profile` directory.

This library reads a pprof profile and converts it to an edge based graph
similar to Blackfire.

Usage:

```golang
fr, err := os.Open(filename)
if err != nil {
	return nil, err
}
defer fr.Close()

profile, err := pprof_reader.ReadFromPProf(fr)
if err != nil {
	return nil, err
}

err = pprof_reader.WriteBFFormat(profile, os.Stdout)
...
```
