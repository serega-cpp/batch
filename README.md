[![Go Report Card](https://goreportcard.com/badge/github.com/serega-cpp/batch)](https://goreportcard.com/report/github.com/serega-cpp/batch)
[![codecov](https://codecov.io/gh/serega-cpp/batch/branch/master/graph/badge.svg)](https://codecov.io/gh/serega-cpp/batch)

### Batch

This package is designed for servers that want to process incoming requests in batches rather than individually. A typical example is adding records to a database.

The obvious solution is to buffer the record, return an OK response to the request, and then write the buffer to the database when the buffer is full (or a timeout occurs). However, this is very unreliable. Another, more complex solution would be to return an asynchronous request identifier to the client, which can then be used to retrieve the operation result. This solution, in addition to complicating the client code, doubles the server load and may not be suitable for high load situations.

The solution implemented in this package offers a different approach. In it, we hold incoming requests until the buffer is actually written to the database. This allows us directly return the result to the client.

The main disadvantages of this solution include:
- for each connection, the request time cannot be less than the buffer timeout;
- the server should be able to maintain a number of open connections at least equal to the buffer size.

#### Usage sample:

```
import "github.com/serega-cpp/batch"

const (
	BatchTimeout      = 100 * time.Millisecond
	DatabaseBatchSize = 10
	DatabaseConnCount = 4
)

type Record struct {}

// Initialization
batchOptions := batch.Options[Record]{
  MaxLifetime:  BatchTimeout,      // default 100ms
  MaxSize:      DatabaseBatchSize, // default 1000
  FlushThreads: DatabaseConnCount, // default 1
  FlushFunc:    func(thread int, records []Record) error {
    return db.InsertBatch(records)
  },
}

b := batch.New[Record](batchOptions)

// In request handler
err := b.AddOne(record)

// On exit
b.Close()
```

#### Installation

```
go get github.com/serega-cpp/batch
```

#### Acknowledgments

Inspired by https://github.com/elgopher/batch
