# Esercizio GO 2019/2020

### Prerequisites

Go must be installed in your system

## Running

To execute code you must first run one or more instances of rpc_consumer.go specifying the listening port

```
go run rpc_consumer.go 4040 

go run rpc_consumer.go 4050
```

Then execute one instance of rpc_server.go specifying the listening port and the consumer's port

```
go run rpc_server.go 1234 4040 4050
```

Finally one or more instances of rpc_producer.go specifying server's port, payload of the message, desired semantic and timeout

```
go run rpc_producer.go 1234 message 1 1
```



