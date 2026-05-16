[![Go Reference](https://pkg.go.dev/badge/github.com/serega-cpp/batch.svg)](https://pkg.go.dev/github.com/serega-cpp/batch)
[![Go Report Card](https://goreportcard.com/badge/github.com/serega-cpp/batch)](https://goreportcard.com/report/github.com/serega-cpp/batch)
[![Go Build](https://github.com/serega-cpp/batch/actions/workflows/build.yaml/badge.svg)](https://github.com/serega-cpp/batch/actions/workflows/build.yaml)
[![codecov](https://codecov.io/gh/serega-cpp/batch/branch/master/graph/badge.svg)](https://codecov.io/gh/serega-cpp/batch)

### Batch

This package is designed for services that need to process incoming requests in batches rather than individually. A typical use case is inserting records into a database.

A straightforward approach is to buffer incoming records, immediately return an OK response to the client, and write the buffer to the database once it is full (or when a timeout occurs). However, this approach is unreliable. A more robust, but significantly more complex solution, is to return an asynchronous request identifier to the client, which can later be used to retrieve the result of the operation. In addition to complicating client-side logic, this approach effectively doubles the server load and may not be suitable for high-load systems.

The solution implemented in this package takes a different approach. Incoming requests are blocked until the buffer is actually written to the database, allowing the service to return the final operation result directly to the client.

The obvious disadvantages of this approach are:
- when using a single connection, request latency is typically equal to the buffer timeout;
- the host system must be able to maintain a large number of open connections in order to fill the buffer efficiently.

At the same time, it is:
- easy to use and allows to build reliable services;
- has good throughput under high loads.

About timeouts and cancellation (make sure that timeout is greater than `Options.BatchFlushInterval`):
- The global timeout (configured via `Options.TotalTimeout`) applies to the entire process, including put, collect, and flush operations. It is passed to the custom `FlushFunc`, which is responsible for handling the timeout through the provided context.
- The request timeout (configured per call in `Put()` / `Puts()`) applies only to the operation of adding an item to the buffer. Once the item has been successfully appended to the buffer, it can no longer be cancelled through this timeout.

#### Usage sample:

```
import "github.com/serega-cpp/batch"

const (
	DatabaseBatchSize = 10
	DatabaseConnCount = 4
)

type Item struct {}

db := database.New(..., DatabaseConnCount)

options := batch.Options[Item]{
  TotalTimeout:       time.Second,
  BatchFlushInterval: 100 * time.Millisecond,
  BatchSize:          DatabaseBatchSize, // default 1000
  FlushThreadsCount:  DatabaseConnCount, // default 1
  FlushFunc: func(thread int, ctx context.Context, items []Item) error {
    return db.Conns[thread].Insert(ctx, items)
  },
}

b := batch.New[Item](options)
defer b.Close()

// and use it in a request handler
err := b.Put(ctx, item)
```

#### Installation

```
go get github.com/serega-cpp/batch
```

#### Acknowledgments

Inspired by https://github.com/elgopher/batch
