package main

import (
	"github.com/hashicorp/consul/api"
	"github.com/garyburd/redigo/redis"
	"log"
	"fmt"
	"os"
)

func main() {

	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		panic(err)
	}
	kv := client.KV()
	session := client.Session()


	preparedQuery := client.PreparedQuery()
	preparedQueries, _, err := preparedQuery.List(&api.QueryOptions{})
	if err != nil {
		panic(err)
	}

	var masterQuery api.PreparedQueryDefinition
	for _, query := range preparedQueries {
		if query.Name == "sensu-master" {
			log.Printf("found query: %+v", query)
			masterQuery = *query
			break
		}
	}
	if masterQuery.ID == "" {
		log.Println("query not found, creating")

		masterQueryDefinition := api.PreparedQueryDefinition{Name: "sensu-master", Service: api.ServiceQuery{Service: "redis", OnlyPassing: true, Tags: []string{"sensu", "master"}}}
		newMasterQueryId, _, err := preparedQuery.Create(&masterQueryDefinition, &api.WriteOptions{})
		if err != nil {
			panic(err)
		}
		masterQueryDefinition.ID = newMasterQueryId
		masterQuery = masterQueryDefinition
	}

	queryResponse, _, err := preparedQuery.Execute(masterQuery.ID, &api.QueryOptions{})
	if err != nil {
		panic(err)
	}

	log.Printf("%+v", queryResponse.Nodes)
	os.Exit(0)


	lock, err := client.LockOpts(&api.LockOptions{Key: "sentinel", LockTryOnce: true})
	if err != nil {
		panic(err)
	}

	lockChan, err := lock.Lock(nil)
	if err != nil {
		panic(err)
	}
	lockHeld := false
	if lockChan == nil {
		log.Println("lock aquisition failed")

		kvData, _, err := kv.Get("sentinel", &api.QueryOptions{})
		if err != nil {
			panic(err)
		}

		sessionData, _, err := session.Info(kvData.Session, &api.QueryOptions{})
		log.Printf("%+v", sessionData)

		redis, err := redis.DialURL("redis://localhost/0")
		if err != nil {
			panic(err)
		}
		resp, err := redis.Do("slaveof", sessionData.Node, 6379)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%+v", resp)


/*type KVPair struct {
    Key         string
    CreateIndex uint64
    ModifyIndex uint64
    LockIndex   uint64
    Flags       uint64
    Value       []byte
    Session     string
}*/

	} else {
		log.Println("got lock")
		lockHeld = true
	}
	log.Println(lockHeld)

	for {
		log.Println(<-lockChan)
	}

}
