[![Go Reference](https://pkg.go.dev/badge/github.com/serega-cpp/batch.svg)](https://pkg.go.dev/github.com/serega-cpp/batch)
[![Lint](https://github.com/serega-cpp/batch/workflows/Lint/badge.svg)](https://github.com/serega-cpp/batch/actions/workflows/lint.yaml)
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

The package flushes the buffer either after reaching a specified size (configured with `Options.BatchFlushSize`) or after its lifetime expires (configured with `Options.BatchFlushInterval`). It does not guarantee that the buffer will always be completely full on flushing. It tries its best, but the items count sent to the database may be larger or smaller than the size. All items from a single `Puts()` call are always sent to the same buffer, so try to keep the number of items reasonable. If you need to work with large sets of items, you can use `PutsMuch()` which splits items into segments and process them one-by-one.

There are two different timeouts to control the execution time of batch operations:
- The request timeout (configured per call in `Put()` / `Puts()`) applies to the operation of adding an item to the buffer. Once the item has been successfully appended to the buffer, it can no longer be cancelled through this timeout.
- The batch processing timeout (configured via `Options.BatchTimeout`) applies to buffer collection and flush operations. It is also passed to the user-defined `FlushFunc` function, which is responsible for handling the timeout through the provided context.

To improve performance in scenarios with a "very small number of clients" an adaptive timeout mode has been introduced. In this mode, the batch flush timeout can be dynamically reduced if the system thinks that all clients have already sent their data and are awaiting results. To enable this mode, set the `Options.BatchMinFlushInterval` parameter. However, do not set it too low, try to keep it between `Options.BatchFlushInterval` and the average flush time.

#### Usage sample:

```
import "github.com/serega-cpp/batch"

const DatabaseConnCount = 4

type Item struct {}

db := database.New(..., DatabaseConnCount)

options := batch.Options[Item]{
  BatchTimeout:       time.Second,
  BatchFlushInterval: 100 * time.Millisecond,
  BatchFlushSize:     10,
  FlushThreadsCount:  DatabaseConnCount,
  FlushFunc: func(ctx context.Context, thread int, items []Item) error {
    return db.Conns[thread].Insert(ctx, items)
  },
}

b := batch.New[Item](options)
defer b.Close()

err := b.Put(ctx, item)
```

#### Installation

```
go get github.com/serega-cpp/batch
```

#### Acknowledgments

Inspired by https://github.com/elgopher/batch
