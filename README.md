# consul-master-election-tagger
[![GitHub version](https://badge.fury.io/gh/wywygmbh%2Fconsul-master-election-tagger.svg)](https://badge.fury.io/gh/wywygmbh%2Fconsul-master-election-tagger)
[![Build Status](https://travis-ci.org/wywygmbh/consul-master-election-tagger.svg?branch=master)](https://travis-ci.org/wywygmbh/consul-master-election-tagger)


Application to support master/slave election using consul tags.

## Motivation

We've created this helper to support the setup of high availability clusters for tools which don't support auto configuration.

One example is [redis](https://redis.io/). It supports Master/Slave operation, but there's no way for a zero configuration setup.

So we take the powers of [consul](https://www.consul.io/), mix it with some go code and get a running cluster including failover support.

## Details

Creates a consul query, which will return the given service (`-consul-service-name`) with the tag `master`. A query has to be used to support multiple tags, which is not possible with the current consul golang bindings and DNS service.

It uses consul locks to add the tag `master` to only one node, the services on the other nodes are tagged as `slave`. 

It's best to let this run as a health check for an existing service.

## Example

To setup a master/slave replication for sensu we need:

* a consul service for redis
* a health check for redis itself
* an additional check to trigger the election process
* a templated shell script to elect/demote redis instances ([redis-promote.ctmpl.erb](examples/redis-promote.ctmpl.erb))

How it works:

1. two (or more) redis instances are started
    * initially each of them is standalone
2. the ping check marks them as green in sensu
3. the election check is executed
    * the election can't find and instance with tag sensu AND tag master
    * it tries to become master using an atomic lock
      * on success, it proclaims itself as master
      * on failure, it tries again, at which point it will find, that there is now a master and declares itself as slave
    * the change of the tags will cause [consul-template](https://github.com/hashicorp/consul-template) to update the promotion script and execute it
4. success!
    * now you've got a running master/slave setup without any configuration of intervention
    * you can connect on `sensu-redis-master.query.consul`
    * the master will stay master, as long as the health checks are passing
    * if the healths checks are failing, the cluster is completely split apart and reconfigured
5. the whole locking dance is repeated every 10 seconds
6. if the master fails, some slave will eventually get the master tag and the cluster is reconfigured to follow the new master


```puppet
::consul::service { "redis":
  ensure       => present,
  tags         => ["sensu"],
  port         => $port,
  checks       => [
    {
      script   => "/opt/sensu/embedded/bin/ruby /opt/sensu/embedded/bin/check-redis-ping.rb -p ${port}",
      interval => "2s",
      timeout  => "1s",
    }
  ]
}
```
```puppet
consul::check { "redis-zeroconf":
  ensure     => 'present',
  script     => "/usr/local/bin/consul-master-election-tagger --consul-service-name redis --consul-query-name sensu-redis-master --consul-query-tag sensu",
  interval   => "10s",
  service_id => 'redis',
}
```
```puppet
consul_template::watch { "redis-promote.sh":
  template    => 'components/consul/redis-promote.ctmpl.erb',
  destination => '/etc/redis/redis-promote.sh',
  command     => 'chmod +x /etc/redis/redis-promote.sh && /etc/redis/redis-promote.sh',
  require     => Service['redis'],
}
```

## Arguments

`-consul-query-name`

  Name of the consul query, which will be executed to return the current master. This is a workaround for [consul#1781](https://github.com/hashicorp/consul/issues/1781) (see [Prepared Query](#prepared-query) below for details.)

`-consul-query-tag`

  Consul tags, which should be included to the query. Can be used multiple times. (Note: tag 'master' is added by default)

`-consul-service-name`: 

  Name of the service to tag with master/slave.
  
## Prepared query

Consul currently does not support querying multiple tags at once (like give me all services with tag sensu AND tag master). But there's a workaround using prepared queries, this a queries which are predefined and stored within consul, these queries can just be executed by their name.

The creation and updating of these queries is done automatically within the tagger application.

## License

Copyright 2017 wywy GmbH

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

This code is being actively maintained by some fellow engineers at [wywy GmbH](http://wywy.com/).