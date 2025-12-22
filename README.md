Batch
=====

This package is designed for servers that want to process incoming requests in batches rather than individually. A typical example is adding records to a database.

The obvious solution is to buffer the record, return an OK response to the request, and then write the buffer to the database when the buffer is full (or a timeout occurs). However, this is very unreliable. Another, more complex solution would be to return an asynchronous request identifier to the client, which can then be used to retrieve the operation result. This solution, in addition to complicating the client code, doubles the server load and may not be suitable for high load situations.

The solution implemented in this package offers a different approach. In it, we hold incoming requests until the buffer is actually written to the database. This allows us directly return the result to the client.

The main disadvantages of this solution include:
- for each connection, the request time cannot be less than the buffer timeout;
- the server should be able to maintain a number of open connections at least equal to the buffer size.

#### Usage sample:

```
const (
	BatchSize         = 10
	BatchTimeout      = 100 * time.Millisecond
	BatchFlushTimeout = 1000 * time.Millisecond
)

// Initialization
batchOptions := batch.Options[database.Item]{
  MaxSize:     BatchSize,    // default 1000
  MaxLifetime: BatchTimeout, // default 100ms
  FlushFunc:   func(items []database.Item) error {
    return db.InsertBatch(items)
  },
}

itemsBatch := batch.New[database.Item](batchOptions)

// In request handler
err := itemsBatch.Add(item)

// On exit
itemsBatch.Close()
```

#### Installation

```
go get github.com/serega-cpp/batch
```

#### Acknowledgments

Inspired by https://github.com/elgopher/batch
