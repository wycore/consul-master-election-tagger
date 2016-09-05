# consul-master-election-tagger
Application to support master/slave election using consul tags.

Creates a consul query, which will return the given service (`-consul-service-name`) with the tag `master`.
Query is used to support multiple tags, which is not possible with the current consul golang bindings and DNS service.

It uses consul locks to add the tag `master` to only one node, the services on the other nodes are tagged as `slave`. 

It's best to let this run as a health check for an existing service.

## Arguments
`-consul-query-name`:
Name of the consul query, which will be executed to return the current master.

`-consul-query-tag`:
Consul tags, which should be included to the query. Can be used multiple times. (Note: tag 'master' is set by default)

`-consul-service-name`: 
Name of the service to tag with master/slave.
